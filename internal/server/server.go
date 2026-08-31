package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	webassets "github.com/voocel/ainovel-cli/web"
)

const (
	productName = "NovelForge"
	apiVersion  = "v1alpha1"
)

// Config controls the local single-binary web server.
type Config struct {
	Host      string
	Port      int
	Workspace string
	Version   string
}

// Server owns the REST/SSE transport. It intentionally depends on persisted
// project artifacts rather than an LLM so inspection remains deterministic.
type Server struct {
	cfg            Config
	startedAt      time.Time
	workspaceLabel string
	events         *EventBroker
	handler        http.Handler
}

func New(cfg Config) (*Server, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 48090
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("port must be between 1 and 65535")
	}
	if strings.TrimSpace(cfg.Workspace) == "" {
		cfg.Workspace = "."
	}
	workspace, err := filepath.Abs(cfg.Workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return nil, fmt.Errorf("prepare workspace: %w", err)
	}
	info, err := os.Stat(workspace)
	if err != nil {
		return nil, fmt.Errorf("stat workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace is not a directory")
	}
	cfg.Workspace = workspace
	if strings.TrimSpace(cfg.Version) == "" {
		cfg.Version = "dev"
	}

	staticRoot, err := fs.Sub(webassets.Assets, "dist")
	if err != nil {
		return nil, fmt.Errorf("load embedded web assets: %w", err)
	}

	s := &Server{
		cfg:            cfg,
		startedAt:      time.Now().UTC(),
		workspaceLabel: filepath.Base(workspace),
		events:         newEventBroker(),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/projects", s.handleProjects)
	mux.HandleFunc("/api/projects/", s.handleProject)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/openapi.json", s.handleOpenAPI)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeAPIError(w, http.StatusNotFound, "API route not found")
	})
	mux.Handle("/", http.FileServer(http.FS(staticRoot)))
	s.handler = securityHeaders(mux)
	return s, nil
}

func (s *Server) Address() string {
	return net.JoinHostPort(s.cfg.Host, fmt.Sprintf("%d", s.cfg.Port))
}

func (s *Server) Handler() http.Handler { return s.handler }

// Events exposes the process-local event bus to API adapters and the durable
// job runner without allowing transport code to mutate persisted truth.
func (s *Server) Events() *EventBroker { return s.events }

func (s *Server) ListenAndServe(ctx context.Context) error {
	httpServer := &http.Server{
		Addr:              s.Address(),
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()
	s.events.Publish("server.ready", "", map[string]any{
		"address":   s.Address(),
		"workspace": s.workspaceLabel,
	})

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"product":        productName,
		"status":         "ok",
		"version":        s.cfg.Version,
		"api_version":    apiVersion,
		"workspace":      s.workspaceLabel,
		"started_at":     s.startedAt,
		"uptime_seconds": int64(time.Since(s.startedAt).Seconds()),
	})
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPISpec)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"status":  status,
			"message": message,
		},
	})
}

func writeMethodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
}
