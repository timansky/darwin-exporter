package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/timansky/darwin-exporter/config"
	"github.com/timansky/darwin-exporter/version"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: ":0",
			MetricsPath:   "/metrics",
			HealthPath:    "/health",
			ReadyPath:     "/ready",
			ReadTimeout:   5 * time.Second,
			WriteTimeout:  5 * time.Second,
		},
		Logging: config.LoggingConfig{Level: "error", Format: "logfmt"},
	}
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	reg := prometheus.NewRegistry()
	return New(cfg, log, reg)
}

func TestHealthHandler(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	s.healthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var resp healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding health response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status=ok, got %q", resp.Status)
	}
}

func TestReadyHandler(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	s.readyHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding ready response: %v", err)
	}
	if resp.Status != "ready" {
		t.Errorf("expected status=ready, got %q", resp.Status)
	}
}

func TestReadyHandler_ShuttingDown(t *testing.T) {
	s := newTestServer(t)
	s.SetShuttingDown(true)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	s.readyHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rec.Code)
	}

	var resp healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding ready response: %v", err)
	}
	if resp.Status != "shutting_down" {
		t.Errorf("expected status=shutting_down, got %q", resp.Status)
	}
}

func TestLandingHandler(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.landingHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if len(body) == 0 {
		t.Error("expected non-empty landing page body")
	}
	// Should contain links to endpoints.
	for _, path := range []string{"/metrics", "/health", "/ready"} {
		if !contains(body, path) {
			t.Errorf("expected landing page to contain %q", path)
		}
	}
}

func TestLandingHandler_NotFound(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	rec := httptest.NewRecorder()
	s.landingHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown path, got %d", rec.Code)
	}
}

func TestMetricsHandler(t *testing.T) {
	s := newTestServer(t)
	// Register a simple gauge.
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "darwin_test_gauge",
		Help: "A test gauge.",
	})
	g.Set(1.0)
	s.registry.MustRegister(g)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	s.metricsHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !contains(body, "darwin_test_gauge") {
		t.Error("expected metrics output to contain darwin_test_gauge")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestLandingHandler_EscapesVersion verifies that HTML-special characters in
// version.Version are escaped in the landing page output.
func TestLandingHandler_EscapesVersion(t *testing.T) {
	s := newTestServer(t)

	// Inject a version string containing a script tag.
	orig := version.Version
	version.Version = `<script>alert(1)</script>`
	defer func() { version.Version = orig }()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.landingHandler(rec, req)

	body := rec.Body.String()
	if contains(body, "<script>") {
		t.Error("landing page must not contain unescaped <script> tag from version string")
	}
	if !contains(body, "&lt;script&gt;") {
		t.Error("landing page should contain HTML-escaped version string")
	}
}

// TestLandingHandler_EscapesConfigPaths verifies that HTML-special characters
// in config path values are escaped in the landing page output.
func TestLandingHandler_EscapesConfigPaths(t *testing.T) {
	s := newTestServer(t)
	s.cfg.Server.MetricsPath = `"><script>alert(1)</script>`

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.landingHandler(rec, req)

	body := rec.Body.String()
	if contains(body, "<script>") {
		t.Error("landing page must not contain unescaped <script> tag from MetricsPath")
	}
}

// TestLoggingMiddleware verifies that the logging middleware passes requests
// through and captures the status code.
func TestLoggingMiddleware(t *testing.T) {
	s := newTestServer(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := s.loggingMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("expected status 418, got %d", rec.Code)
	}
}

// TestSecurityHeadersMiddleware verifies that securityHeadersMiddleware injects
// the required defence-in-depth HTTP headers into every response.
func TestSecurityHeadersMiddleware(t *testing.T) {
	s := newTestServer(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := s.securityHeadersMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	wantHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Cache-Control":          "no-store",
	}
	for header, want := range wantHeaders {
		got := rec.Header().Get(header)
		if got != want {
			t.Errorf("header %s: expected %q, got %q", header, want, got)
		}
	}

	// CSP must be set (value may vary; just verify it is non-empty).
	if csp := rec.Header().Get("Content-Security-Policy"); csp == "" {
		t.Error("expected Content-Security-Policy header to be set")
	}
}

// TestSecurityHeaders_InFullServer verifies that security headers are present
// when requests go through the full server handler chain.
func TestSecurityHeaders_InFullServer(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	s.httpSrv.Handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("expected X-Content-Type-Options=nosniff, got %q", got)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("expected X-Frame-Options=DENY, got %q", got)
	}
}

// TestResponseWriter_WriteHeader verifies that responseWriter captures the status code.
func TestResponseWriter_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, status: http.StatusOK}
	rw.WriteHeader(http.StatusNotFound)
	if rw.status != http.StatusNotFound {
		t.Errorf("expected captured status 404, got %d", rw.status)
	}
}
