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
	indexBody := index.Body.String()
	for _, required := range []string{
		"NovelForge",
		`src="./assets/app.js"`,
		`href="./assets/app.css"`,
	} {
		if !strings.Contains(indexBody, required) {
			t.Fatalf("embedded index is missing %q: %s", required, indexBody)
		}
	}
	csp := index.Header().Get("Content-Security-Policy")
	if csp == "" || strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
		t.Fatalf("expected strict CSP without inline or evaluated code, got %q", csp)
	}

	javascript := httptest.NewRecorder()
	server.Handler().ServeHTTP(javascript, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if javascript.Code != http.StatusOK {
		t.Fatalf("app.js status = %d", javascript.Code)
	}
	if contentType := javascript.Header().Get("Content-Type"); !strings.Contains(contentType, "javascript") {
		t.Fatalf("app.js content type = %q", contentType)
	}
	javascriptBody := javascript.Body.String()
	for _, required := range []string{"NovelForge", "EventSource"} {
		if !strings.Contains(javascriptBody, required) {
			t.Fatalf("rebuilt app.js is missing %q", required)
		}
	}
	for _, forbidden := range []string{"eval(", "new Function", "unsafe-inline", "api_key"} {
		if strings.Contains(javascriptBody, forbidden) {
			t.Fatalf("rebuilt app.js contains forbidden token %q", forbidden)
		}
	}

	stylesheet := httptest.NewRecorder()
	server.Handler().ServeHTTP(stylesheet, httptest.NewRequest(http.MethodGet, "/assets/app.css", nil))
	if stylesheet.Code != http.StatusOK {
		t.Fatalf("app.css status = %d", stylesheet.Code)
	}
	if contentType := stylesheet.Header().Get("Content-Type"); !strings.Contains(contentType, "text/css") {
		t.Fatalf("app.css content type = %q", contentType)
	}
	stylesheetBody := stylesheet.Body.String()
	if !strings.Contains(stylesheetBody, "::-webkit-progress-value") ||
		!strings.Contains(stylesheetBody, "::-moz-progress-bar") {
		t.Fatalf("embedded progress styles are incomplete")
	}
	for _, forbidden := range []string{"javascript:", "@import"} {
		if strings.Contains(stylesheetBody, forbidden) {
			t.Fatalf("rebuilt app.css contains forbidden token %q", forbidden)
		}
	}
}
