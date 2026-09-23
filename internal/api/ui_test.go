package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nullgate/api/internal/config"
	"nullgate/api/internal/webui"
)

// minimalCfg makes Handler() registrable without a real environment: ServeMux
// panics on empty patterns, so every builtin path needs a non-empty default.
func minimalCfg() *config.Config {
	return &config.Config{
		WSPath: "/ws", XHTTPPath: "/xhttp", VmessPath: "/vmess",
		TrojanPath: "/trojan", HUPath: "/hu",
	}
}

// TestUINoEmbed verifies the handler degrades to 404 when no UI was embedded
// (repo builds without the web export).
func TestUINoEmbed(t *testing.T) {
	if webuiHasIndex() {
		t.Skip("real UI embedded in this build tree; stub behaviour not testable")
	}
	s := &Server{Cfg: minimalCfg()}
	srv := httptest.NewServer(s.uiHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 without embedded UI, got %d", resp.StatusCode)
	}
}

// TestUIRoutesRegisters verifies Handler() compiles into a servable mux and
// that the UI catch-all does not shadow the API.
func TestUIRoutesRegisters(t *testing.T) {
	s := &Server{Cfg: minimalCfg()}
	h := s.Handler() // must not panic with minimal deps
	if h == nil {
		t.Fatal("Handler() returned nil")
	}
	// GET /api/state without a session cookie must hit the authed API handler
	// (401 JSON) — never the embedded UI index.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/state", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/state → want 401 from API auth, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("UI index leaked into /api/state response: %s", rec.Body.String())
	}
}

func webuiHasIndex() bool {
	h := (&Server{}).uiHandler()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Body)
	return rec.Code == 200 && len(body) > 0
}

// TestUIServesIndexAndFallback exercises the full routing chain when a real
// UI is embedded (Docker build tree).
func TestUIServesIndexAndFallback(t *testing.T) {
	if !webui.HasIndex() {
		t.Skip("no UI embedded in this build tree")
	}
	s := &Server{Cfg: minimalCfg()}
	h := s.Handler()

	// GET / → UI index
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 {
		t.Fatalf("GET / → want 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("GET / content-type = %q, want text/html", ct)
	}

	// unknown extension-less path → SPA fallback (index.html, 200)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/some/route", nil))
	if rec.Code != 200 {
		t.Fatalf("GET /some/route → want 200 SPA fallback, got %d", rec.Code)
	}

	// unknown asset-like path → 404, never HTML
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/missing.js", nil))
	if rec.Code != 404 {
		t.Fatalf("GET /missing.js → want 404, got %d", rec.Code)
	}

	// builtin Xray path must not be swallowed by the UI (proxy target is down
	// in tests → 502 with empty body; certainly not the index page)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/ws", nil))
	if strings.Contains(rec.Body.String(), "<!DOCTYPE html>") {
		t.Fatal("UI index leaked into /ws (builtin Xray path)")
	}

	// /api/state answers (401 auth JSON) before any UI fallback even with
	// nil internals — proves API precedence over the UI
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/state", nil))
	if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("GET /api/state → want 401 JSON, got %d: %s", rec.Code, rec.Body.String())
	}
}
