// Package server provides a hardened HTTP server with JSON health endpoints and
// graceful shutdown that drains in-flight requests.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

const defaultShutdownTimeout = 10 * time.Second

// Option customizes Server construction.
type Option func(*options)

type options struct {
	readTimeout     time.Duration
	writeTimeout    time.Duration
	idleTimeout     time.Duration
	shutdownTimeout time.Duration
	ready           func(context.Context) bool
}

func defaultOptions() options {
	return options{
		readTimeout:     10 * time.Second,
		writeTimeout:    30 * time.Second,
		idleTimeout:     60 * time.Second,
		shutdownTimeout: defaultShutdownTimeout,
		ready:           func(context.Context) bool { return true },
	}
}

// WithReadTimeout overrides the default ReadTimeout.
func WithReadTimeout(d time.Duration) Option {
	return func(o *options) { o.readTimeout = d }
}

// WithWriteTimeout overrides the default WriteTimeout.
func WithWriteTimeout(d time.Duration) Option {
	return func(o *options) { o.writeTimeout = d }
}

// WithIdleTimeout overrides the default IdleTimeout.
func WithIdleTimeout(d time.Duration) Option {
	return func(o *options) { o.idleTimeout = d }
}

// WithShutdownTimeout overrides how long cleanup waits for in-flight requests.
func WithShutdownTimeout(d time.Duration) Option {
	return func(o *options) { o.shutdownTimeout = d }
}

// WithReadyCheck sets the readiness probe used by GET /readyz.
func WithReadyCheck(fn func(context.Context) bool) Option {
	return func(o *options) { o.ready = fn }
}

// Server wraps http.Server with chassis lifecycle helpers.
type Server struct {
	httpServer      *http.Server
	log             *slog.Logger
	shutdownTimeout time.Duration
	started         atomic.Bool
	inFlight        atomic.Int64
	draining        atomic.Bool
}

type statusBody struct {
	Status string `json:"status"`
}

// New builds an HTTP server that mounts JSON /healthz and /readyz beside handler.
// Cleanup runs Shutdown with a bounded timeout and refuses new work while draining.
func New(addr string, handler http.Handler, log *slog.Logger, opts ...Option) (*Server, func(), error) {
	if addr == "" {
		return nil, nil, fmt.Errorf("server: addr is required")
	}
	if handler == nil {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusNotFound, statusBody{Status: "not_found"})
		})
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.With("component", "server")

	cfg := defaultOptions()
	for _, opt := range opts {
		opt(&cfg)
	}

	s := &Server{log: log, shutdownTimeout: cfg.shutdownTimeout}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, statusBody{Status: "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if s.draining.Load() || !cfg.ready(r.Context()) {
			writeJSON(w, http.StatusServiceUnavailable, statusBody{Status: "not_ready"})
			return
		}
		writeJSON(w, http.StatusOK, statusBody{Status: "ready"})
	})
	mux.Handle("/", handler)

	wrapped := s.trackInFlight(Recoverer(log)(RequestLogger(log)(mux)))

	hs := &http.Server{
		Addr:         addr,
		Handler:      wrapped,
		ReadTimeout:  cfg.readTimeout,
		WriteTimeout: cfg.writeTimeout,
		IdleTimeout:  cfg.idleTimeout,
	}
	s.httpServer = hs

	cleanup := func() {
		s.draining.Store(true)
		s.log.Info("draining", "in_flight", s.inFlight.Load(), "timeout", s.shutdownTimeout)
		ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		if err := hs.Shutdown(ctx); err != nil {
			s.log.Error("shutdown failed", "err", err, "in_flight", s.inFlight.Load())
			return
		}
		s.log.Info("server stopped")
	}
	return s, cleanup, nil
}

func (s *Server) trackInFlight(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always allow probes during drain so orchestrators see not_ready.
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		if s.draining.Load() {
			writeJSON(w, http.StatusServiceUnavailable, statusBody{Status: "shutting_down"})
			return
		}
		s.inFlight.Add(1)
		defer s.inFlight.Add(-1)
		next.ServeHTTP(w, r)
	})
}

// InFlight returns the number of non-probe requests currently being handled.
func (s *Server) InFlight() int64 { return s.inFlight.Load() }

// Start listens and serves until Shutdown. http.ErrServerClosed is not an error.
func (s *Server) Start() error {
	s.log.Info("listening", "addr", s.httpServer.Addr)
	s.started.Store(true)
	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: listen: %w", err)
	}
	return nil
}

// StartTLS is like Start but serves TLS.
func (s *Server) StartTLS(certFile, keyFile string) error {
	s.log.Info("listening tls", "addr", s.httpServer.Addr)
	s.started.Store(true)
	err := s.httpServer.ListenAndServeTLS(certFile, keyFile)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: listen tls: %w", err)
	}
	return nil
}

// Addr returns the configured or bound address.
func (s *Server) Addr() string { return s.httpServer.Addr }

// Serve is used by tests to bind a specific listener.
func (s *Server) Serve(ln net.Listener) error {
	s.httpServer.Addr = ln.Addr().String()
	s.log.Info("listening", "addr", s.httpServer.Addr)
	s.started.Store(true)
	err := s.httpServer.Serve(ln)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: serve: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
