// Package spanner provides a Cloud Spanner client with chassis lifecycle management.
package spanner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	gspanner "cloud.google.com/go/spanner"
)

const (
	defaultPingTimeout    = 10 * time.Second
	defaultPingRetries    = 5
	defaultPingRetryDelay = time.Second
)

// Option customizes client creation.
type Option func(*options)

type options struct {
	pingTimeout    time.Duration
	pingRetries    int
	pingRetryDelay time.Duration
}

func defaultOptions() options {
	return options{
		pingTimeout:    defaultPingTimeout,
		pingRetries:    defaultPingRetries,
		pingRetryDelay: defaultPingRetryDelay,
	}
}

// WithPingRetries overrides startup ping attempts (default 5).
func WithPingRetries(n int) Option {
	return func(o *options) { o.pingRetries = n }
}

// WithPingTimeout overrides the per-attempt ping timeout.
func WithPingTimeout(d time.Duration) Option {
	return func(o *options) { o.pingTimeout = d }
}

// New creates a Spanner client for database (projects/…/instances/…/databases/…).
// Credentials follow Application Default Credentials.
func New(ctx context.Context, database string, log *slog.Logger, opts ...Option) (*gspanner.Client, func(), error) {
	if database == "" {
		return nil, nil, fmt.Errorf("spanner: database is required")
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.With("component", "db/spanner")

	cfg := defaultOptions()
	for _, opt := range opts {
		opt(&cfg)
	}

	client, err := gspanner.NewClient(ctx, database)
	if err != nil {
		return nil, nil, fmt.Errorf("spanner: client: %w", err)
	}

	if err := pingWithRetry(ctx, client, log, cfg); err != nil {
		client.Close()
		return nil, nil, err
	}

	log.Info("ready", "database", database)
	cleanup := func() {
		client.Close()
		log.Info("closed")
	}
	return client, cleanup, nil
}

func pingWithRetry(ctx context.Context, client *gspanner.Client, log *slog.Logger, cfg options) error {
	var lastErr error
	for attempt := 1; attempt <= cfg.pingRetries; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, cfg.pingTimeout)
		iter := client.Single().Query(pingCtx, gspanner.Statement{SQL: "SELECT 1"})
		_, lastErr = iter.Next()
		iter.Stop()
		cancel()
		if lastErr == nil {
			return nil
		}
		log.Warn("ping failed",
			"attempt", attempt,
			"retries", cfg.pingRetries,
			"err", lastErr,
		)
		if attempt == cfg.pingRetries {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("spanner: ping canceled: %w", ctx.Err())
		case <-time.After(cfg.pingRetryDelay):
		}
	}
	return fmt.Errorf("spanner: ping after %d attempts: %w", cfg.pingRetries, lastErr)
}
