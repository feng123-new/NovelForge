package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Event is the stable envelope used by the local SSE stream.
type Event struct {
	ID      uint64    `json:"id"`
	Type    string    `json:"type"`
	Time    time.Time `json:"time"`
	Project string    `json:"project,omitempty"`
	Data    any       `json:"data,omitempty"`
}

// EventBroker is an in-process fan-out bus. Durable job events will be replayed
// from the job store in a later phase; this broker deliberately owns no truth.
type EventBroker struct {
	mu          sync.RWMutex
	nextID      uint64
	subscribers map[chan Event]struct{}
}

func newEventBroker() *EventBroker {
	return &EventBroker{subscribers: make(map[chan Event]struct{})}
}

// Publish sends an event to currently connected clients without allowing a
// slow browser to block engine or API work.
func (b *EventBroker) Publish(eventType, project string, data any) Event {
	b.mu.Lock()
	b.nextID++
	event := Event{
		ID:      b.nextID,
		Type:    eventType,
		Time:    time.Now().UTC(),
		Project: project,
		Data:    data,
	}
	for subscriber := range b.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
	b.mu.Unlock()
	return event
}

func (b *EventBroker) subscribe() (<-chan Event, func()) {
	channel := make(chan Event, 32)
	b.mu.Lock()
	b.subscribers[channel] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subscribers, channel)
			close(channel)
			b.mu.Unlock()
		})
	}
	return channel, cancel
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "streaming is unavailable")
		return
	}

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")

	events, unsubscribe := s.events.subscribe()
	defer unsubscribe()

	connected := Event{
		ID:   0,
		Type: "connected",
		Time: time.Now().UTC(),
		Data: map[string]any{
			"product":   productName,
			"version":   s.cfg.Version,
			"workspace": s.workspaceLabel,
		},
	}
	if err := writeSSE(w, connected); err != nil {
		return
	}
	flusher.Flush()

	keepAlive := time.NewTicker(20 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := writeSSE(w, event); err != nil {
				return
			}
			flusher.Flush()
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	eventType := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, event.Type)
	if eventType == "" {
		eventType = "message"
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, eventType, data)
	return err
}
