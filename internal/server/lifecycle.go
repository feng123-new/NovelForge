package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/voocel/ainovel-cli/internal/lifecycle"
	"github.com/voocel/ainovel-cli/internal/project"
)

func (s *Server) registerLifecycleRoutes(m *http.ServeMux) {
	m.HandleFunc("/api/projects/{id}/lifecycle/imports", s.handleManuscriptImports)
	m.HandleFunc("/api/projects/{id}/lifecycle/imports/{import}", s.handleManuscriptImport)
	m.HandleFunc("/api/projects/{id}/lifecycle/imports/{import}/step", s.handleManuscriptStep)
	m.HandleFunc("/api/projects/{id}/lifecycle/export", s.handleManuscriptExport)
	m.HandleFunc("/api/projects/{id}/lifecycle/backup", s.handleLifecycleBackup)
	m.HandleFunc("/api/projects/{id}/lifecycle/backups/{backup}", s.handleStoredLifecycleBackup)
	m.HandleFunc("/api/projects/{id}/lifecycle/migrate", s.handleLifecycleMigrate)
	m.HandleFunc("/api/lifecycle/restore", s.handleLifecycleRestore)
}
func lifecycleFailure(err error) *apiFailure {
	switch {
	case errors.Is(err, lifecycle.ErrConflict), errors.Is(err, project.ErrConflict):
		return &apiFailure{Status: 409, Code: "LIFECYCLE_CONFLICT", Message: "resolve existing content, unfinished Final commits or project conflicts before retrying"}
	case errors.Is(err, lifecycle.ErrLimit):
		return &apiFailure{Status: 413, Code: "LIFECYCLE_LIMIT", Message: "manuscript or archive exceeds the documented limits"}
	case errors.Is(err, lifecycle.ErrInvalid), errors.Is(err, project.ErrValidation), errors.Is(err, project.ErrUnsafePath):
		return &apiFailure{Status: 400, Code: "LIFECYCLE_INVALID", Message: "unsupported format, unsafe archive or invalid lifecycle request"}
	case errors.Is(err, lifecycle.ErrNotFound), errors.Is(err, project.ErrNotFound), errors.Is(err, os.ErrNotExist):
		return &apiFailure{Status: 404, Code: "LIFECYCLE_NOT_FOUND", Message: "project or lifecycle item was not found"}
	default:
		f := internalFailure()
		return &f
	}
}
func lifecyclePage(r *http.Request) (int, int, error) {
	limit, offset := 50, 0
	var err error
	if x := r.URL.Query().Get("limit"); x != "" {
		limit, err = strconv.Atoi(x)
		if err != nil {
			return 0, 0, lifecycle.ErrInvalid
		}
	}
	if x := r.URL.Query().Get("offset"); x != "" {
		offset, err = strconv.Atoi(x)
		if err != nil {
			return 0, 0, lifecycle.ErrInvalid
		}
	}
	if limit < 1 || limit > 100 || offset < 0 {
		return 0, 0, lifecycle.ErrInvalid
	}
	return limit, offset, nil
}
func (s *Server) handleManuscriptImports(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if r.Method == http.MethodPost {
		s.uploadManuscript(w, r, id)
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET", "POST")
		return
	}
	limit, offset, err := lifecyclePage(r)
	if err != nil {
		writeFailure(w, r, *lifecycleFailure(err))
		return
	}
	store, err := s.projects.OpenLifecycle(r.Context(), id)
	if err != nil {
		writeFailure(w, r, *lifecycleFailure(err))
		return
	}
	defer store.DB.Close()
	imports, err := store.List(r.Context(), limit, offset)
	if err != nil {
		writeFailure(w, r, *lifecycleFailure(err))
		return
	}
	writeJSON(w, 200, map[string]any{"imports": imports, "limit": limit, "offset": offset, "model_available": s.qualityConfigured(id)})
}
func (s *Server) handleManuscriptImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	limit, offset, err := lifecyclePage(r)
	if err != nil {
		writeFailure(w, r, *lifecycleFailure(err))
		return
	}
	store, err := s.projects.OpenLifecycle(r.Context(), r.PathValue("id"))
	if err != nil {
		writeFailure(w, r, *lifecycleFailure(err))
		return
	}
	defer store.DB.Close()
	page, err := store.Page(r.Context(), r.PathValue("import"), limit, offset)
	if err != nil {
		writeFailure(w, r, *lifecycleFailure(err))
		return
	}
	writeJSON(w, 200, page)
}
func (s *Server) handleManuscriptStep(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeMethodNotAllowed(w, r, "POST")
		return
	}
	id := r.PathValue("id")
	batch := r.PathValue("import")
	s.executeIdempotent(w, r, "lifecycle.import.step", id, func(body []byte) (int, any, *apiFailure) {
		var in struct {
			Chapter int  `json:"chapter"`
			Analyze bool `json:"analyze"`
		}
		if f := decodeJSONBody(body, &in, false); f != nil {
			return f.Status, nil, f
		}
		if in.Chapter < 1 || in.Chapter > 1000 {
			f := lifecycleFailure(lifecycle.ErrInvalid)
			return f.Status, nil, f
		}
		var evaluate project.ImportEvaluator
		if in.Analyze {
			if !s.qualityConfigured(id) {
				f := qualityUnavailable()
				return f.Status, nil, &f
			}
			c, cleanup, f := s.chapterVersionCoordinator(r, id)
			if f != nil {
				return f.Status, nil, f
			}
			defer cleanup()
			evaluate = func(ctx context.Context, n int, key, version string) error {
				_, err := c.Check(ctx, n, key, version)
				return err
			}
		}
		chapter, err := s.projects.StepManuscript(r.Context(), id, batch, in.Chapter, in.Analyze, evaluate)
		if err != nil {
			f := lifecycleFailure(err)
			return f.Status, nil, f
		}
		return 200, map[string]any{"chapter": chapter, "accepted": false, "finalized": false}, nil
	})
}
func lifecycleInt(r *http.Request, key string) (int, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 1000 {
		return 0, lifecycle.ErrInvalid
	}
	return n, nil
}

func (s *Server) lifecycleDownload(w http.ResponseWriter, r *http.Request, filename, media string, load func() ([]byte, error)) {
	lease, err := s.projects.AcquireExecution(r.Context(), r.PathValue("id"))
	if err != nil {
		writeFailure(w, r, *lifecycleFailure(err))
		return
	}
	defer lease.Close()
	active, err := s.jobs.Active(r.Context(), r.PathValue("id"))
	if err != nil {
		writeFailure(w, r, *jobFailure(err))
		return
	}
	if active {
		writeFailure(w, r, *lifecycleFailure(lifecycle.ErrConflict))
		return
	}
	data, err := load()
	if err != nil {
		writeFailure(w, r, *lifecycleFailure(err))
		return
	}
	w.Header().Set("Content-Type", media)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.Header().Set("X-Content-SHA256", lifecycle.SHA(data))
	w.WriteHeader(200)
	_, _ = w.Write(data)
}
func (s *Server) handleManuscriptExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "md"
	}
	media := map[string]string{"txt": "text/plain; charset=utf-8", "md": "text/markdown; charset=utf-8", "epub": "application/epub+zip"}[format]
	from, e1 := lifecycleInt(r, "from")
	to, e2 := lifecycleInt(r, "to")
	if media == "" || e1 != nil || e2 != nil {
		writeFailure(w, r, *lifecycleFailure(lifecycle.ErrInvalid))
		return
	}
	s.lifecycleDownload(w, r, "novelforge."+format, media, func() ([]byte, error) {
		b, _, err := s.projects.ExportManuscript(r.Context(), r.PathValue("id"), format, from, to)
		return b, err
	})
}
func (s *Server) handleLifecycleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	s.lifecycleDownload(w, r, "novelforge-backup.zip", "application/zip", func() ([]byte, error) { return s.projects.BackupLifecycle(r.Context(), r.PathValue("id")) })
}
func (s *Server) handleStoredLifecycleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	s.lifecycleDownload(w, r, "novelforge-before-migration.zip", "application/zip", func() ([]byte, error) {
		return s.projects.ReadLifecycleBackup(r.Context(), r.PathValue("id"), r.PathValue("backup"))
	})
}
func (s *Server) handleLifecycleMigrate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeMethodNotAllowed(w, r, "POST")
		return
	}
	id := r.PathValue("id")
	s.executeIdempotent(w, r, "lifecycle.migrate", id, func(body []byte) (int, any, *apiFailure) {
		var in struct {
			Expected int    `json:"expected_format"`
			Confirm  string `json:"confirm"`
		}
		if f := decodeJSONBody(body, &in, false); f != nil {
			return f.Status, nil, f
		}
		if in.Confirm != id {
			f := lifecycleFailure(lifecycle.ErrInvalid)
			return f.Status, nil, f
		}
		result, err := s.projects.MigrateLifecycle(r.Context(), id, r.Header.Get("Idempotency-Key"), in.Expected)
		if err != nil {
			f := lifecycleFailure(err)
			return f.Status, nil, f
		}
		return 200, result, nil
	})
}
