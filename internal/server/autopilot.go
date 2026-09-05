package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/voocel/ainovel-cli/internal/autopilot"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
)

type jobStartInput struct {
	StartChapter  int  `json:"start_chapter"`
	TargetChapter int  `json:"target_chapter"`
	ReviewEvery   *int `json:"review_every,omitempty"`
	MaxRewrites   *int `json:"max_rewrites,omitempty"`
	MaxRetries    *int `json:"max_retries,omitempty"`
}

func (s *Server) registerAutopilotRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/projects/{id}/autopilot", s.handleAutopilot)
	mux.HandleFunc("/api/projects/{id}/autopilot/{job}", s.handleAutopilotJob)
	mux.HandleFunc("/api/projects/{id}/autopilot/{job}/pause", s.handleAutopilotControl)
	mux.HandleFunc("/api/projects/{id}/autopilot/{job}/stop", s.handleAutopilotControl)
	mux.HandleFunc("/api/projects/{id}/autopilot/{job}/resume", s.handleAutopilotControl)
}
func (s *Server) autopilotReady() bool { return s.runner != nil && s.runner.Ready() }
func (s *Server) jobView(j autopilot.Job) map[string]any {
	v := j.View()
	v["actions"] = map[string]bool{"pause": s.autopilotReady() && (j.State == autopilot.Pending || j.State == autopilot.Running || j.State == autopilot.Retrying) && j.Control == "", "stop": !j.Terminal() && j.Control != "stop", "resume": s.autopilotReady() && (j.State == autopilot.Paused || j.State == autopilot.Failed)}
	return v
}
func jobFailure(err error) *apiFailure {
	if errors.Is(err, autopilot.ErrNotFound) {
		return &apiFailure{Status: 404, Code: "JOB_NOT_FOUND", Message: "autopilot job not found"}
	}
	if errors.Is(err, autopilot.ErrConflict) {
		return &apiFailure{Status: 409, Code: "JOB_STATE_CONFLICT", Message: "job state conflicts; refresh the task before retrying"}
	}
	var f *autopilot.Failure
	if errors.As(err, &f) {
		return &apiFailure{Status: 503, Code: f.Code, Message: "autopilot dependencies or model profile are unavailable"}
	}
	return &apiFailure{Status: 500, Code: "AUTOPILOT_STORAGE_ERROR", Message: "autopilot operation could not be persisted"}
}
func (s *Server) handleAutopilot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.projects.Get(r.Context(), id)
	if err != nil {
		writeFailure(w, r, *projectFailure(err))
		return
	}
	switch r.Method {
	case http.MethodGet:
		limit, offset, failure := parseBoundedPage(r, 50, 100)
		if failure != nil {
			writeFailure(w, r, *failure)
			return
		}
		jobs, err := s.jobs.List(r.Context(), id, limit, offset)
		if err != nil {
			writeFailure(w, r, *jobFailure(err))
			return
		}
		views := []map[string]any{}
		for _, j := range jobs {
			views = append(views, s.jobView(j))
		}
		nextChapter, nextErr := s.projects.AutopilotNextChapter(r.Context(), id)
		if nextErr != nil {
			writeFailure(w, r, *projectFailure(nextErr))
			return
		}
		writeJSON(w, 200, map[string]any{"next_chapter": nextChapter, "jobs": views, "worker_available": s.autopilotReady(), "model_available": s.qualityConfigured(id), "limit": limit, "offset": offset})
		return
	case http.MethodPost:
		if !s.autopilotReady() {
			writeFailure(w, r, apiFailure{Status: 503, Code: "AUTOPILOT_UNAVAILABLE", Message: "autopilot worker is not running"})
			return
		}
		if p.Archived {
			writeFailure(w, r, apiFailure{Status: 409, Code: "PROJECT_ARCHIVED", Message: "restore the project before starting a job"})
			return
		}
		s.executeIdempotent(w, r, "autopilot.start", id, func(body []byte) (int, any, *apiFailure) {
			var options jobStartInput
			if failure := decodeJSONBody(body, &options, false); failure != nil {
				return failure.Status, nil, failure
			}
			foundation, err := s.projects.GetFoundationRequest(r.Context(), id)
			if err != nil {
				f := projectFailure(err)
				return f.Status, nil, f
			}
			if options.StartChapter == 0 {
				options.StartChapter, err = s.projects.AutopilotNextChapter(r.Context(), id)
				if err != nil {
					f := projectFailure(err)
					return f.Status, nil, f
				}
			}
			if options.TargetChapter == 0 {
				options.TargetChapter = p.TotalChapters
			}
			if options.TargetChapter == 0 {
				options.TargetChapter = options.StartChapter
			}
			every := 0
			switch foundation.Automation.ReviewPolicy {
			case "every_chapter":
				every = 1
			case "every_n":
				every = foundation.Automation.ReviewEveryN
			}
			if foundation.Automation.Mode == "copilot" {
				every = 1
			}
			in := autopilot.Input{BookTargetChapter: max(p.TotalChapters, options.TargetChapter), FoundationID: foundation.ID, Idea: foundation.Idea, Style: foundation.Style, Language: p.Language, WordsPerChapter: p.WordsPerChapter, StartChapter: options.StartChapter, TargetChapter: options.TargetChapter, ReviewEvery: every, MaxRewrites: foundation.Automation.MaxRewrites, MaxRetries: 2, ModelProfile: foundation.ModelProfile}
			if options.ReviewEvery != nil {
				in.ReviewEvery = *options.ReviewEvery
			}
			if options.MaxRewrites != nil {
				in.MaxRewrites = *options.MaxRewrites
			}
			if options.MaxRetries != nil {
				in.MaxRetries = *options.MaxRetries
			}
			if in.Validate() != nil {
				return 400, nil, &apiFailure{Status: 400, Code: "JOB_INPUT_INVALID", Message: "chapter range and policy limits are invalid"}
			}
			if _, err = s.autopilotModel(r.Context(), id, in.ModelProfile); err != nil {
				f := jobFailure(err)
				return f.Status, nil, f
			}
			lease, err := s.projects.AcquireExecution(r.Context(), id)
			if err != nil {
				return 409, nil, &apiFailure{Status: 409, Code: "PROJECT_BUSY", Message: "project is being written by another operation"}
			}
			defer lease.Close()
			job, err := s.jobs.Enqueue(r.Context(), id, r.Header.Get("Idempotency-Key"), in)
			if err != nil {
				f := jobFailure(err)
				return f.Status, nil, f
			}
			s.runner.Wake()
			return 202, map[string]any{"job": s.jobView(job)}, nil
		})
	default:
		writeMethodNotAllowed(w, r, http.MethodGet, http.MethodPost)
	}
}
func (s *Server) ownJob(r *http.Request) (autopilot.Job, *apiFailure) {
	j, err := s.jobs.Get(r.Context(), r.PathValue("job"))
	if err != nil {
		return j, jobFailure(err)
	}
	if j.ProjectID != r.PathValue("id") {
		return autopilot.Job{}, jobFailure(autopilot.ErrNotFound)
	}
	if _, err = s.projects.Get(r.Context(), j.ProjectID); err != nil {
		return j, projectFailure(err)
	}
	return j, nil
}
func (s *Server) handleAutopilotJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	j, failure := s.ownJob(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	text := ""
	displayedCandidateID := ""
	qs, err := s.projects.OpenQualityStore(r.Context(), j.ProjectID)
	if err != nil {
		writeFailure(w, r, *projectFailure(err))
		return
	}
	defer qs.Close()
	chapter := j.Chapter
	if j.State == autopilot.Completed {
		chapter = j.CompletedThrough
	}
	snap, err := qs.Snapshot(r.Context(), j.ProjectID, chapter)
	if err != nil && !errors.Is(err, qualitygate.ErrNotFound) {
		writeFailure(w, r, *qualityFailure(err))
		return
	}
	if err == nil {
		candidateID := snap.Transaction.FinalCandidateID
		if candidateID == "" && len(snap.Candidates) > 0 {
			candidateID = snap.Candidates[len(snap.Candidates)-1].ID
		}
		if candidateID != "" {
			c, err := qs.Candidate(r.Context(), candidateID)
			if err != nil {
				writeFailure(w, r, *qualityFailure(err))
				return
			}
			text = c.Text
			displayedCandidateID = c.ID
		}
	}
	writeJSON(w, 200, map[string]any{"job": s.jobView(j), "foundation": j.Foundation, "chapter_plan": j.Plan, "candidate_text": text, "candidate_id": displayedCandidateID, "quality": snap})
}
func (s *Server) handleAutopilotControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	j, failure := s.ownJob(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	action := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	if action == "resume" && !s.autopilotReady() {
		writeFailure(w, r, apiFailure{Status: 503, Code: "AUTOPILOT_UNAVAILABLE", Message: "autopilot worker is not running"})
		return
	}
	s.executeIdempotent(w, r, "autopilot."+action, j.ProjectID, func(body []byte) (int, any, *apiFailure) {
		var approval autopilot.Approval
		if f := decodeJSONBody(body, &approval, true); f != nil {
			return f.Status, nil, f
		}
		if action == "resume" {
			projectState, err := s.projects.Get(r.Context(), j.ProjectID)
			if err != nil {
				f := projectFailure(err)
				return f.Status, nil, f
			}
			if projectState.Archived {
				return 409, nil, &apiFailure{Status: 409, Code: "PROJECT_ARCHIVED", Message: "restore the project before resuming"}
			}
			lease, err := s.projects.AcquireExecution(r.Context(), j.ProjectID)
			if err != nil {
				return 409, nil, &apiFailure{Status: 409, Code: "PROJECT_BUSY", Message: "project is being edited"}
			}
			defer lease.Close()
		}
		next, err := s.jobs.ControlApproved(r.Context(), j.ID, action, approval)
		if err != nil {
			f := jobFailure(err)
			return f.Status, nil, f
		}
		if s.runner != nil {
			s.runner.Wake()
		}
		return 202, map[string]any{"job": s.jobView(next)}, nil
	})
}

// Job events were committed with their state. Fan-out must not append a
// duplicate durable event, and slow browsers must never block the worker.
func (s *Server) publishJobEvent(e autopilot.Event) {
	event := Event{ID: e.ID, Type: "autopilot.changed", Time: e.Time, Project: e.Project, Data: e.Data}
	b := s.events
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, sub := range b.subscribers {
		if sub.project != "" && sub.project != e.Project {
			continue
		}
		select {
		case sub.channel <- event:
		default:
			delete(b.subscribers, id)
			close(sub.channel)
		}
	}
}

// Central transport guard covers versions, quality, ledger, metadata and
// project deletion. Workers call services directly while holding the same
// book lease; they never loop back through HTTP handlers.
func (s *Server) autopilotWriteGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		id := ""
		if len(parts) >= 3 && parts[0] == "api" && parts[1] == "projects" {
			id = parts[2]
			if len(parts) >= 4 && parts[3] == "autopilot" {
				next.ServeHTTP(w, r)
				return
			}
		}
		if r.URL.Path == "/api/truth/events" || r.URL.Path == "/api/truth/rebuild" {
			body, failure := readRequestBody(r)
			if failure != nil {
				writeFailure(w, r, *failure)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			var identity struct {
				ProjectID string `json:"project_id"`
			}
			if json.Unmarshal(body, &identity) == nil {
				id = identity.ProjectID
			}
		}
		if id == "" {
			next.ServeHTTP(w, r)
			return
		}
		if _, err := s.projects.Get(r.Context(), id); err != nil {
			writeFailure(w, r, *projectFailure(err))
			return
		}
		destructive := r.Method == http.MethodDelete || strings.HasSuffix(r.URL.Path, "/archive")
		if destructive {
			unfinished, err := s.jobs.Unfinished(r.Context(), id)
			if err != nil {
				writeFailure(w, r, *jobFailure(err))
				return
			}
			if unfinished {
				writeFailure(w, r, apiFailure{Status: 409, Code: "PROJECT_JOB_UNFINISHED", Message: "stop the unfinished job before archiving or deleting this project"})
				return
			}
		}
		active, err := s.jobs.Active(r.Context(), id)
		if err != nil {
			writeFailure(w, r, *jobFailure(err))
			return
		}
		if active {
			writeFailure(w, r, apiFailure{Status: 409, Code: "PROJECT_AUTOPILOT_BUSY", Message: "pause or stop the project job before editing"})
			return
		}
		lease, err := s.projects.AcquireExecution(r.Context(), id)
		if err != nil {
			writeFailure(w, r, apiFailure{Status: 409, Code: "PROJECT_BUSY", Message: "project is being written by another operation"})
			return
		}
		defer lease.Close()
		active, err = s.jobs.Active(r.Context(), id)
		if destructive {
			active, err = s.jobs.Unfinished(r.Context(), id)
		}
		if err != nil {
			writeFailure(w, r, *jobFailure(err))
			return
		}
		if active {
			writeFailure(w, r, apiFailure{Status: 409, Code: "PROJECT_AUTOPILOT_BUSY", Message: "pause or stop the project job before editing"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
