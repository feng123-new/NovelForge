package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/voocel/ainovel-cli/internal/server/eventstore"
)

const (
	sseReplayBatch     = 500
	sseReplayPerStream = 10_000
)

// Event is the stable envelope used by the local SSE stream.
type Event struct {
	ID      uint64    `json:"id,omitempty"`
	Type    string    `json:"type"`
	Time    time.Time `json:"time"`
	Project string    `json:"project,omitempty"`
	Data    any       `json:"data,omitempty"`
}

type eventSubscriber struct {
	channel chan Event
	project string
}

// EventBroker persists before fan-out. A full subscriber buffer is closed so
// EventSource reconnects with Last-Event-ID instead of blocking producers.
type EventBroker struct {
	mu          sync.Mutex
	nextSubID   uint64
	repository  eventstore.Repository
	subscribers map[uint64]eventSubscriber
}

func newEventBroker(repository eventstore.Repository) *EventBroker {
	return &EventBroker{
		repository:  repository,
		subscribers: make(map[uint64]eventSubscriber),
	}
}

// Publish keeps the original convenience signature. API and job code that must
// observe persistence errors should call PublishContext.
func (b *EventBroker) Publish(eventType, project string, data any) Event {
	event, _ := b.PublishContext(context.Background(), eventType, project, data)
	return event
}

// PublishContext appends durably before non-blocking fan-out.
func (b *EventBroker) PublishContext(
	ctx context.Context,
	eventType string,
	project string,
	data any,
) (Event, error) {
	record, err := b.repository.Append(ctx, eventType, project, data)
	if err != nil {
		return Event{}, err
	}
	event := eventFromRecord(record)

	b.mu.Lock()
	for id, subscriber := range b.subscribers {
		if subscriber.project != "" && subscriber.project != event.Project {
			continue
		}
		select {
		case subscriber.channel <- event:
		default:
			delete(b.subscribers, id)
			close(subscriber.channel)
		}
	}
	b.mu.Unlock()
	return event, nil
}

func (b *EventBroker) replay(
	ctx context.Context,
	afterID uint64,
	project string,
	limit int,
) ([]Event, error) {
	records, err := b.repository.Replay(ctx, afterID, project, limit)
	if err != nil {
		return nil, err
	}
	events := make([]Event, 0, len(records))
	for _, record := range records {
		events = append(events, eventFromRecord(record))
	}
	return events, nil
}

func (b *EventBroker) subscribe(project string) (<-chan Event, func()) {
	channel := make(chan Event, 64)
	b.mu.Lock()
	b.nextSubID++
	id := b.nextSubID
	b.subscribers[id] = eventSubscriber{channel: channel, project: project}
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			subscriber, ok := b.subscribers[id]
			if ok {
				delete(b.subscribers, id)
				close(subscriber.channel)
			}
			b.mu.Unlock()
		})
	}
	return channel, cancel
}

func eventFromRecord(record eventstore.Record) Event {
	return Event{
		ID:      record.ID,
		Type:    record.Type,
		Time:    record.Time.UTC(),
		Project: record.Project,
		Data:    json.RawMessage(record.Data),
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAPIError(
			w,
			r,
			http.StatusInternalServerError,
			"SSE_UNAVAILABLE",
			"streaming is unavailable",
		)
		return
	}
	afterID, failure := parseLastEventID(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	projectFilter := strings.TrimSpace(r.URL.Query().Get("project"))
	if projectFilter != "" && !validProjectFilter(projectFilter) {
		writeAPIError(
			w,
			r,
			http.StatusBadRequest,
			"PROJECT_FILTER_INVALID",
			"project filter is invalid",
		)
		return
	}

	events, unsubscribe := s.events.subscribe(projectFilter)
	defer unsubscribe()

	firstReplay, err := s.events.replay(r.Context(), afterID, projectFilter, sseReplayBatch)
	if err != nil {
		writeFailure(w, r, internalFailure())
		return
	}

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")

	connected := Event{
		Type: "connected",
		Time: time.Now().UTC(),
		Data: map[string]any{
			"product":        productName,
			"version":        s.cfg.Version,
			"workspace":      s.workspaceLabel,
			"project_filter": projectFilter,
		},
	}
	if err := writeSSE(w, connected); err != nil {
		return
	}
	flusher.Flush()

	lastSent := afterID
	replayed := 0
	batch := firstReplay
	for {
		for _, event := range batch {
			if event.ID <= lastSent {
				continue
			}
			if err := writeSSE(w, event); err != nil {
				return
			}
			lastSent = event.ID
			replayed++
		}
		flusher.Flush()
		if len(batch) < sseReplayBatch {
			break
		}
		if replayed >= sseReplayPerStream {
			_ = writeSSE(w, Event{
				Type: "replay.truncated",
				Time: time.Now().UTC(),
				Data: map[string]any{
					"last_event_id": lastSent,
					"reconnect":     true,
				},
			})
			flusher.Flush()
			return
		}
		batch, err = s.events.replay(r.Context(), lastSent, projectFilter, sseReplayBatch)
		if err != nil {
			_ = writeSSE(w, Event{
				Type: "stream.error",
				Time: time.Now().UTC(),
				Data: map[string]any{
					"code":      "EVENT_REPLAY_FAILED",
					"retryable": true,
				},
			})
			flusher.Flush()
			return
		}
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				// A slow client is disconnected intentionally. EventSource will
				// reconnect and replay from the last delivered durable ID.
				return
			}
			if event.ID <= lastSent {
				continue
			}
			if err := writeSSE(w, event); err != nil {
				return
			}
			lastSent = event.ID
			flusher.Flush()
		case <-heartbeat.C:
			if err := writeSSE(w, Event{
				Type: "heartbeat",
				Time: time.Now().UTC(),
				Data: map[string]any{"last_event_id": lastSent},
			}); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func parseLastEventID(r *http.Request) (uint64, *apiFailure) {
	value := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if value == "" {
		value = strings.TrimSpace(r.URL.Query().Get("last_event_id"))
	}
	if value == "" {
		return 0, nil
	}
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, &apiFailure{
			Status:  http.StatusBadRequest,
			Code:    "LAST_EVENT_ID_INVALID",
			Message: "Last-Event-ID must be an unsigned integer",
		}
	}
	return id, nil
}

func validProjectFilter(value string) bool {
	if len(value) > 128 {
		return false
	}
	for _, runeValue := range value {
		if (runeValue >= 'a' && runeValue <= 'z') ||
			(runeValue >= 'A' && runeValue <= 'Z') ||
			(runeValue >= '0' && runeValue <= '9') ||
			runeValue == '-' ||
			runeValue == '_' {
			continue
		}
		return false
	}
	return value != ""
}

func writeSSE(w http.ResponseWriter, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	eventType := strings.Map(func(runeValue rune) rune {
		if runeValue == '\n' || runeValue == '\r' {
			return -1
		}
		return runeValue
	}, event.Type)
	if eventType == "" {
		eventType = "message"
	}
	if event.ID > 0 {
		if _, err := fmt.Fprintf(w, "id: %d\n", event.ID); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
	return err
}
