// Package server provides the HTTP server for darwin-exporter.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"github.com/timansky/darwin-exporter/config"
	"github.com/timansky/darwin-exporter/version"
)

// landingTmpl is parsed once at init to catch template errors early.
var landingTmpl = template.Must(template.New("landing").Parse(`<!DOCTYPE html>
<html>
<head><title>darwin-exporter</title></head>
<body>
<h1>darwin-exporter {{.Version}}</h1>
<p>macOS-specific Prometheus exporter</p>
<ul>
  <li><a href="{{.MetricsPath}}">Metrics</a></li>
  <li><a href="{{.HealthPath}}">Health</a></li>
  <li><a href="{{.ReadyPath}}">Ready</a></li>
</ul>
</body>
</html>`))

// healthResponse is the JSON body returned by the health endpoint.
type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Server wraps the HTTP server with configuration and registry.
type Server struct {
	cfg      *config.Config
	log      *logrus.Logger
	registry *prometheus.Registry
	httpSrv  *http.Server

	shuttingDown atomic.Bool
}

// New creates a Server ready to serve metrics.
func New(cfg *config.Config, log *logrus.Logger, registry *prometheus.Registry) *Server {
	s := &Server{
		cfg:      cfg,
		log:      log,
		registry: registry,
	}

	mux := http.NewServeMux()
	mux.Handle(cfg.Server.MetricsPath, s.metricsHandler())
	mux.HandleFunc(cfg.Server.HealthPath, s.healthHandler)
	mux.HandleFunc(cfg.Server.ReadyPath, s.readyHandler)
	mux.HandleFunc("/", s.landingHandler)

	s.httpSrv = &http.Server{
		Addr:         cfg.Server.ListenAddress,
		Handler:      s.loggingMiddleware(s.securityHeadersMiddleware(mux)),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	return s
}

// ListenAndServe starts the HTTP server. It blocks until the server shuts down.
func (s *Server) ListenAndServe() error {
	s.log.WithField("address", s.cfg.Server.ListenAddress).Info("starting darwin-exporter")
	if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server error: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.log.Info("shutting down darwin-exporter")
	return s.httpSrv.Shutdown(ctx)
}

// SetShuttingDown updates readiness mode: true makes /ready return 503.
func (s *Server) SetShuttingDown(v bool) {
	s.shuttingDown.Store(v)
}

// IsShuttingDown reports current readiness drain mode.
func (s *Server) IsShuttingDown() bool {
	return s.shuttingDown.Load()
}

// metricsHandler returns the Prometheus metrics handler.
func (s *Server) metricsHandler() http.Handler {
	return promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{
		ErrorLog:      s.log.WithField("component", "promhttp"),
		ErrorHandling: promhttp.ContinueOnError,
	})
}

// healthHandler responds to liveness probes.
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	resp := healthResponse{
		Status:  "ok",
		Version: version.Version,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.log.WithError(err).Warn("failed to encode health response")
	}
}

// readyHandler responds to readiness probes.
func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.shuttingDown.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		resp := healthResponse{
			Status:  "shutting_down",
			Version: version.Version,
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			s.log.WithError(err).Warn("failed to encode ready response")
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	resp := healthResponse{
		Status:  "ready",
		Version: version.Version,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.log.WithError(err).Warn("failed to encode ready response")
	}
}

// landingHandler serves a simple HTML page with links to endpoints.
func (s *Server) landingHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := struct {
		Version     string
		MetricsPath string
		HealthPath  string
		ReadyPath   string
	}{
		Version:     version.Version,
		MetricsPath: s.cfg.Server.MetricsPath,
		HealthPath:  s.cfg.Server.HealthPath,
		ReadyPath:   s.cfg.Server.ReadyPath,
	}
	if err := landingTmpl.Execute(w, data); err != nil {
		s.log.WithError(err).Warn("failed to render landing page")
	}
}

// securityHeadersMiddleware adds defence-in-depth HTTP security headers to
// every response. Applied before the logging middleware so that headers are
// set even when inner handlers fail or redirect.
func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs each HTTP request.
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		s.log.WithFields(logrus.Fields{
			"method":   r.Method,
			"path":     r.URL.Path,
			"status":   rw.status,
			"duration": time.Since(start).String(),
		}).Debug("HTTP request")
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
