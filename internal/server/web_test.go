package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedWorkspaceHonorsStrictCSP(t *testing.T) {
	t.Parallel()
	server, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	index := httptest.NewRecorder()
	server.Handler().ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if index.Code != http.StatusOK {
		t.Fatalf("index status = %d, body = %s", index.Code, index.Body.String())
	}
	if !strings.Contains(index.Body.String(), "NovelForge") || !strings.Contains(index.Body.String(), `src="/app.js"`) {
		t.Fatalf("unexpected embedded index: %s", index.Body.String())
	}
	csp := index.Header().Get("Content-Security-Policy")
	if csp == "" || strings.Contains(csp, "unsafe-inline") {
		t.Fatalf("expected strict CSP without unsafe-inline, got %q", csp)
	}

	javascript := httptest.NewRecorder()
	server.Handler().ServeHTTP(javascript, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	if javascript.Code != http.StatusOK {
		t.Fatalf("app.js status = %d", javascript.Code)
	}
	if !strings.Contains(javascript.Body.String(), "createElement('progress')") || strings.Contains(javascript.Body.String(), ".style.width") {
		t.Fatalf("dashboard progress is not CSP-safe")
	}

	stylesheet := httptest.NewRecorder()
	server.Handler().ServeHTTP(stylesheet, httptest.NewRequest(http.MethodGet, "/app.css", nil))
	if stylesheet.Code != http.StatusOK {
		t.Fatalf("app.css status = %d", stylesheet.Code)
	}
	if !strings.Contains(stylesheet.Body.String(), "::-webkit-progress-value") || !strings.Contains(stylesheet.Body.String(), "::-moz-progress-bar") {
		t.Fatalf("embedded progress styles are incomplete")
	}
}
