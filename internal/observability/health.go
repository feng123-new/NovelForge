package observability

import (
	"context"
	"encoding/json"
	"sort"
	"time"
)

type Health struct {
	Provider            string     `json:"provider"`
	State               string     `json:"state"`
	LastError           string     `json:"last_error"`
	LastAttemptAt       *time.Time `json:"last_attempt_at"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	CooldownUntil       *time.Time `json:"cooldown_until"`
	ManuallyPaused      bool       `json:"manually_paused"`
}

func (s *Store) Health(ctx context.Context, p Policy) ([]Health, error) {
	providers := map[string]bool{}
	for _, r := range p.Prices {
		providers[r.Provider] = true
	}
	for _, v := range p.PausedProviders {
		providers[v] = true
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT provider FROM observation_attempts WHERE project_id=? GROUP BY provider ORDER BY max(seq) DESC LIMIT 100`, s.ProjectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v string
		if err = rows.Scan(&v); err != nil {
			break
		}
		providers[v] = true
	}
	readErr := rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if readErr != nil {
		return nil, readErr
	}
	names := []string{}
	for v := range providers {
		names = append(names, v)
	}
	sort.Strings(names)
	if len(names) > 200 {
		names = names[:200]
	}
	out := []Health{}
	for _, provider := range names {
		h := Health{Provider: provider, State: "unknown"}
		for _, v := range p.PausedProviders {
			h.ManuallyPaused = h.ManuallyPaused || v == provider
		}
		rows, err = s.DB.QueryContext(ctx, `SELECT payload_json FROM observation_attempts WHERE project_id=? AND provider=? ORDER BY seq DESC LIMIT ?`, s.ProjectID, provider, p.FailureThreshold)
		if err != nil {
			return nil, err
		}
		var latestEnd *time.Time
		first := true
		for rows.Next() {
			var raw string
			var a Attempt
			if err = rows.Scan(&raw); err != nil {
				break
			}
			if err = json.Unmarshal([]byte(raw), &a); err != nil {
				break
			}
			if first {
				h.LastAttemptAt = &a.StartedAt
				h.LastError = a.ErrorCode
				if a.State == "completed" {
					h.State = "last_attempt_succeeded"
				} else if a.State == "pending" {
					h.State = "in_flight_or_interrupted"
				} else {
					h.State = "last_attempt_failed_or_unknown"
				}
				latestEnd = a.EndedAt
				first = false
			}
			if a.State != "failed" {
				break
			}
			h.ConsecutiveFailures++
		}
		readErr = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if readErr != nil {
			return nil, readErr
		}
		if h.ConsecutiveFailures >= p.FailureThreshold && latestEnd != nil {
			until := latestEnd.Add(time.Duration(p.CooldownSeconds) * time.Second)
			if s.now().Before(until) {
				h.CooldownUntil = &until
				h.State = "cooldown"
			}
		}
		if h.ManuallyPaused {
			h.State = "paused"
		}
		out = append(out, h)
	}
	return out, nil
}
