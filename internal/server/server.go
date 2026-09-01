package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/project"
	"github.com/voocel/ainovel-cli/internal/server/engineadapter"
	"github.com/voocel/ainovel-cli/internal/server/eventstore"
	"github.com/voocel/ainovel-cli/internal/server/idempotency"
	serverrepository "github.com/voocel/ainovel-cli/internal/server/repository"
	webassets "github.com/voocel/ainovel-cli/web"
)

const (
	productName = "NovelForge"
	apiVersion  = "v1alpha1"
)

// Config controls the local single-binary web server.
type Config struct {
	Host            string
	Port            int
	Workspace       string
	Version         string
	Engine          engineadapter.EngineService
	EventRepository eventstore.Repository
}

// Server owns REST/SSE transport and delegates deterministic work to
// repositories and engine adapters.
type Server struct {
	cfg            Config
	startedAt      time.Time
	workspaceLabel string
	projects       *project.Repository
	events         *EventBroker
	idempotency    idempotency.Store
	engine         engineadapter.EngineService
	handler        http.Handler
}

// New initializes workspace durability before exposing routes.
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
	cfg.Workspace = filepath.Clean(workspace)
	if strings.TrimSpace(cfg.Version) == "" {
		cfg.Version = "dev"
	}

	paths, err := serverrepository.Initialize(context.Background(), cfg.Workspace)
	if err != nil {
		return nil, fmt.Errorf("initialize server repository: %w", err)
	}
	projectRepository, err := project.NewRepository(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	eventRepository := cfg.EventRepository
	if eventRepository == nil {
		eventRepository = eventstore.SQLiteRepository{DatabasePath: paths.Database}
	}
	engine := cfg.Engine
	if engine == nil {
		engine = engineadapter.LegacyAdapter{}
	}

	staticRoot, err := fs.Sub(webassets.Assets, "dist")
	if err != nil {
		return nil, fmt.Errorf("load embedded web assets: %w", err)
	}

	s := &Server{
		cfg:            cfg,
		startedAt:      time.Now().UTC(),
		workspaceLabel: safeWorkspaceLabel(cfg.Workspace),
		projects:       projectRepository,
		events:         newEventBroker(eventRepository),
		idempotency: idempotency.Store{
			DatabasePath: paths.Database,
			TTL:          24 * time.Hour,
		},
		engine: engine,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/projects", s.handleProjects)
	mux.HandleFunc("/api/projects/", s.handleProject)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/openapi.json", s.handleOpenAPI)
	s.registerWorkspaceRoutes(mux)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, r, http.StatusNotFound, "API_ROUTE_NOT_FOUND", "API route not found")
	})
	mux.Handle("/", http.FileServer(http.FS(staticRoot)))
	s.handler = securityHeaders(traceMiddleware(recoveryMiddleware(mux)))
	return s, nil
}

func safeWorkspaceLabel(value string) string {
	clean := filepath.Clean(value)
	volumeRoot := filepath.VolumeName(clean) + string(filepath.Separator)
	if clean == string(filepath.Separator) || (filepath.VolumeName(clean) != "" && clean == volumeRoot) {
		return "workspace"
	}
	label := filepath.Base(clean)
	if label == "" || label == "." || label == string(filepath.Separator) {
		return "workspace"
	}
	return label
}

func (s *Server) Address() string {
	return net.JoinHostPort(s.cfg.Host, fmt.Sprintf("%d", s.cfg.Port))
}

func (s *Server) Handler() http.Handler { return s.handler }

// Events exposes the durable, non-blocking event broker to later job adapters.
func (s *Server) Events() *EventBroker { return s.events }

// Engine exposes the adapter boundary without allowing API handlers to
// duplicate engine internals.
func (s *Server) Engine() engineadapter.EngineService { return s.engine }

// Close exists for lifecycle symmetry. Repositories open SQLite handles per
// operation, so no file lock remains held after a request.
func (s *Server) Close() error { return nil }

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
	if _, err := s.events.PublishContext(ctx, "server.ready", "", map[string]any{
		"address":   s.Address(),
		"workspace": s.workspaceLabel,
	}); err != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		<-errCh
		return fmt.Errorf("persist server ready event: %w", err)
	}

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
		writeMethodNotAllowed(w, r, http.MethodGet)
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
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(openAPISpec)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPISpec)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set(
			"Content-Security-Policy",
			"default-src 'self'; connect-src 'self'; img-src 'self' data:; "+
				"style-src 'self'; script-src 'self'; base-uri 'none'; frame-ancestors 'none'",
		)
		next.ServeHTTP(w, r)
	})
}
