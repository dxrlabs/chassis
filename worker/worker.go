// Package worker runs background and periodic jobs with graceful shutdown.
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const defaultShutdownTimeout = 30 * time.Second

// Option customizes Runner behavior.
type Option func(*options)

type options struct {
	shutdownTimeout time.Duration
}

// WithShutdownTimeout sets how long cleanup waits for in-flight jobs.
func WithShutdownTimeout(d time.Duration) Option {
	return func(o *options) { o.shutdownTimeout = d }
}

// Runner manages continuous and periodic background work.
type Runner struct {
	log *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	shutdownTimeout time.Duration
}

// New creates a Runner. Cleanup cancels the worker context and waits for jobs.
func New(log *slog.Logger, opts ...Option) (*Runner, func(), error) {
	if log == nil {
		log = slog.Default()
	}
	log = log.With("component", "worker")

	cfg := options{shutdownTimeout: defaultShutdownTimeout}
	for _, opt := range opts {
		opt(&cfg)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := &Runner{
		log:             log,
		ctx:             ctx,
		cancel:          cancel,
		shutdownTimeout: cfg.shutdownTimeout,
	}

	cleanup := func() {
		r.cancel()
		done := make(chan struct{})
		go func() {
			r.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			r.log.Info("workers stopped")
		case <-time.After(r.shutdownTimeout):
			r.log.Warn("worker shutdown timed out", "timeout", r.shutdownTimeout)
		}
	}
	return r, cleanup, nil
}

// Go starts a continuous background worker. fn should return when ctx is done.
func (r *Runner) Go(fn func(ctx context.Context) error) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if err := fn(r.ctx); err != nil && r.ctx.Err() == nil {
			r.log.Error("worker exited with error", "err", err)
		}
	}()
}

// Every runs fn immediately, then on every interval until the runner stops.
func (r *Runner) Every(interval time.Duration, name string, fn func(ctx context.Context) error) error {
	if interval <= 0 {
		return fmt.Errorf("worker: interval must be positive")
	}
	if name == "" {
		return fmt.Errorf("worker: name is required")
	}
	if fn == nil {
		return fmt.Errorf("worker: fn is required")
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		log := r.log.With("job", name)

		run := func() {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic in periodic job", "panic", rec)
				}
			}()
			if err := fn(r.ctx); err != nil && r.ctx.Err() == nil {
				log.Error("periodic job failed", "err", err)
			}
		}

		run()

		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-r.ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()
	return nil
}
