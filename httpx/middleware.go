package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

type ctxKey int

const requestIDKey ctxKey = 1

// RequestIDFromContext returns the request ID previously set by RequestID.
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

// RequestID ensures every request has an X-Request-ID header and context value.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000000")))
	}
	return hex.EncodeToString(b[:])
}

// CORSConfig controls Cross-Origin Resource Sharing headers.
type CORSConfig struct {
	AllowOrigins []string
	AllowMethods []string
	AllowHeaders []string
}

// CORS returns middleware that applies a simple CORS policy.
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	origins := cfg.AllowOrigins
	if len(origins) == 0 {
		origins = []string{"*"}
	}
	methods := cfg.AllowMethods
	if len(methods) == 0 {
		methods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions}
	}
	headers := cfg.AllowHeaders
	if len(headers) == 0 {
		headers = []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"}
	}

	allowMethods := strings.Join(methods, ", ")
	allowHeaders := strings.Join(headers, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && originAllowed(origin, origins) {
				if origins[0] == "*" {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				}
				w.Header().Set("Access-Control-Allow-Methods", allowMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func originAllowed(origin string, allowed []string) bool {
	for _, a := range allowed {
		if a == "*" || a == origin {
			return true
		}
	}
	return false
}

// Timeout cancels the request context after d and responds with JSON 503 on timeout.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()

			tw := &timeoutWriter{ResponseWriter: w}
			done := make(chan struct{})
			go func() {
				next.ServeHTTP(tw, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded && !tw.wrote {
					WriteJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "request timeout"})
				}
				<-done
			}
		})
	}
}

type timeoutWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *timeoutWriter) WriteHeader(code int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *timeoutWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}
