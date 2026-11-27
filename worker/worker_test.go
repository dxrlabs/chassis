package worker_test

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dxrlabs/chassis/worker"
)

func TestRunner_Go_cancelsAndWaits(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewTextHandler(ioDiscard{}, nil))
	r, cleanup, err := worker.New(log, worker.WithShutdownTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var started, finished atomic.Bool
	r.Go(func(ctx context.Context) error {
		started.Store(true)
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond)
		finished.Store(true)
		return ctx.Err()
	})

	deadline := time.Now().Add(time.Second)
	for !started.Load() {
		if time.Now().After(deadline) {
			t.Fatal("worker never started")
		}
		time.Sleep(5 * time.Millisecond)
	}

	done := make(chan struct{})
	go func() {
		cleanup()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cleanup did not return")
	}

	if !finished.Load() {
		t.Fatal("worker did not finish before cleanup returned")
	}
}

func TestRunner_Every_runsAndStops(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewTextHandler(ioDiscard{}, nil))
	r, cleanup, err := worker.New(log, worker.WithShutdownTimeout(time.Second))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cleanup()

	var n atomic.Int64
	if err := r.Every(20*time.Millisecond, "tick", func(ctx context.Context) error {
		n.Add(1)
		return nil
	}); err != nil {
		t.Fatalf("Every: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for n.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("ticks = %d, want >= 2", n.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRunner_Every_rejectsBadInterval(t *testing.T) {
	t.Parallel()
	r, cleanup, err := worker.New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cleanup()
	if err := r.Every(0, "x", func(context.Context) error { return nil }); err == nil {
		t.Fatal("expected error")
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
