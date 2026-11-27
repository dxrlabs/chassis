// Package redis provides a go-redis client with chassis lifecycle management.
package redis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	defaultPingTimeout    = 5 * time.Second
	defaultPingRetries    = 5
	defaultPingRetryDelay = time.Second
	defaultPoolSize       = 10
)

// Option customizes client creation.
type Option func(*options)

type options struct {
	password       string
	db             int
	poolSize       int
	pingTimeout    time.Duration
	pingRetries    int
	pingRetryDelay time.Duration
}

func defaultOptions() options {
	return options{
		poolSize:       defaultPoolSize,
		pingTimeout:    defaultPingTimeout,
		pingRetries:    defaultPingRetries,
		pingRetryDelay: defaultPingRetryDelay,
	}
}

// WithPassword sets the Redis AUTH password.
func WithPassword(pw string) Option {
	return func(o *options) { o.password = pw }
}

// WithDB selects the logical database index.
func WithDB(db int) Option {
	return func(o *options) { o.db = db }
}

// WithPoolSize sets the connection pool size.
func WithPoolSize(n int) Option {
	return func(o *options) { o.poolSize = n }
}

// WithPingRetries overrides startup ping attempts (default 5).
func WithPingRetries(n int) Option {
	return func(o *options) { o.pingRetries = n }
}

// WithPingRetryDelay overrides delay between ping attempts.
func WithPingRetryDelay(d time.Duration) Option {
	return func(o *options) { o.pingRetryDelay = d }
}

// New dials Redis at addr (host:port), pings with retry, and returns cleanup.
func New(ctx context.Context, addr string, log *slog.Logger, opts ...Option) (*goredis.Client, func(), error) {
	if addr == "" {
		return nil, nil, fmt.Errorf("redis: addr is required")
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.With("component", "redis")

	cfg := defaultOptions()
	for _, opt := range opts {
		opt(&cfg)
	}

	client := goredis.NewClient(&goredis.Options{
		Addr:     addr,
		Password: cfg.password,
		DB:       cfg.db,
		PoolSize: cfg.poolSize,
	})

	if err := pingWithRetry(ctx, client, log, cfg); err != nil {
		_ = client.Close()
		return nil, nil, err
	}

	log.Info("ready", "addr", addr, "db", cfg.db)
	cleanup := func() {
		if err := client.Close(); err != nil {
			log.Warn("close failed", "err", err)
			return
		}
		log.Info("closed")
	}
	return client, cleanup, nil
}

func pingWithRetry(ctx context.Context, client *goredis.Client, log *slog.Logger, cfg options) error {
	var lastErr error
	for attempt := 1; attempt <= cfg.pingRetries; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, cfg.pingTimeout)
		lastErr = client.Ping(pingCtx).Err()
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
			return fmt.Errorf("redis: ping canceled: %w", ctx.Err())
		case <-time.After(cfg.pingRetryDelay):
		}
	}
	return fmt.Errorf("redis: ping after %d attempts: %w", cfg.pingRetries, lastErr)
}
