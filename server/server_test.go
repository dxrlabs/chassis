package server_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/dxrlabs/chassis/server"
)

func TestServer_healthAndGracefulShutdown(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"hi"}`))
	})

	srv, cleanup, err := server.New("127.0.0.1:0", mux, log, server.WithShutdownTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	base := "http://" + ln.Addr().String()
	waitReady(t, base+"/healthz")

	res, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d", res.StatusCode)
	}
	if !bytes.Contains(body, []byte(`"status"`)) || !bytes.Contains(body, []byte(`ok`)) {
		t.Fatalf("healthz body = %s", body)
	}

	res, err = http.Get(base + "/readyz")
	if err != nil {
		t.Fatalf("readyz: %v", err)
	}
	body, _ = io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("readyz status = %d", res.StatusCode)
	}
	if !bytes.Contains(body, []byte(`ready`)) {
		t.Fatalf("readyz body = %s", body)
	}

	res, err = http.Get(base + "/hello")
	if err != nil {
		t.Fatalf("hello: %v", err)
	}
	body, _ = io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(`hi`)) {
		t.Fatalf("hello = %d %q", res.StatusCode, body)
	}

	done := make(chan struct{})
	go func() {
		cleanup()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("cleanup timed out")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after shutdown")
	}
}

func TestServer_readyCheck(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, cleanup, err := server.New("127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}), log,
		server.WithReadyCheck(func(context.Context) bool { return false }),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cleanup()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()

	base := "http://" + ln.Addr().String()
	waitReady(t, base+"/healthz")

	res, err := http.Get(base + "/readyz")
	if err != nil {
		t.Fatalf("readyz: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readyz status = %d, want 503", res.StatusCode)
	}
	if !bytes.Contains(body, []byte(`not_ready`)) {
		t.Fatalf("readyz body = %s", body)
	}
}

func TestRecoverer(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := server.Recoverer(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := newRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	if rec.code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.code)
	}
	if !bytes.Contains(rec.body, []byte(`error`)) {
		t.Fatalf("body = %s", rec.body)
	}
}

type recorder struct {
	header http.Header
	code   int
	body   []byte
}

func newRecorder() *recorder {
	return &recorder{header: make(http.Header), code: http.StatusOK}
}

func (r *recorder) Header() http.Header         { return r.header }
func (r *recorder) Write(b []byte) (int, error) { r.body = append(r.body, b...); return len(b), nil }
func (r *recorder) WriteHeader(code int)        { r.code = code }

func waitReady(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		res, err := http.Get(url)
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("server not ready: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
