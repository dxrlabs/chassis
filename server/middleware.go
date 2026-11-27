package server

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// Middleware transforms an http.Handler.
type Middleware func(http.Handler) http.Handler

// Recoverer recovers from panics and returns JSON 500.
func Recoverer(log *slog.Logger) Middleware {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered",
						"panic", rec,
						"path", r.URL.Path,
						"method", r.Method,
						"stack", string(debug.Stack()),
					)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": http.StatusText(http.StatusInternalServerError)})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// RequestLogger logs method, path, status, bytes, and duration.
func RequestLogger(log *slog.Logger) Middleware {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"bytes", sw.bytes,
				"duration", time.Since(start),
				"remote", r.RemoteAddr,
			)
		})
	}
}
