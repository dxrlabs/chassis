// Package postgres provides a pgx connection pool with lifecycle management.
package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultMaxConns          = int32(10)
	defaultMinConns          = int32(2)
	defaultMaxConnLifetime   = time.Hour
	defaultHealthCheckPeriod = time.Minute
	defaultPingTimeout       = 10 * time.Second
	defaultPingRetries       = 5
	defaultPingRetryDelay    = time.Second
)

// Option customizes pool creation.
type Option func(*options)

type options struct {
	maxConns          int32
	minConns          int32
	maxConnLifetime   time.Duration
	healthCheckPeriod time.Duration
	pingTimeout       time.Duration
	pingRetries       int
	pingRetryDelay    time.Duration
}

func defaultOptions() options {
	return options{
		maxConns:          defaultMaxConns,
		minConns:          defaultMinConns,
		maxConnLifetime:   defaultMaxConnLifetime,
		healthCheckPeriod: defaultHealthCheckPeriod,
		pingTimeout:       defaultPingTimeout,
		pingRetries:       defaultPingRetries,
		pingRetryDelay:    defaultPingRetryDelay,
	}
}

// WithMaxConns sets the maximum number of connections in the pool.
func WithMaxConns(n int32) Option {
	return func(o *options) { o.maxConns = n }
}

// WithMinConns sets the minimum number of connections in the pool.
func WithMinConns(n int32) Option {
	return func(o *options) { o.minConns = n }
}

// WithMaxConnLifetime sets how long a connection may be reused.
func WithMaxConnLifetime(d time.Duration) Option {
	return func(o *options) { o.maxConnLifetime = d }
}

// WithHealthCheckPeriod sets the pool health-check interval.
func WithHealthCheckPeriod(d time.Duration) Option {
	return func(o *options) { o.healthCheckPeriod = d }
}

// New creates a pgx pool, pings with retry, and returns a cleanup that closes it.
func New(ctx context.Context, dsn string, log *slog.Logger, opts ...Option) (*pgxpool.Pool, func(), error) {
	if dsn == "" {
		return nil, nil, fmt.Errorf("postgres: dsn is required")
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.With("component", "db/postgres")

	cfg := defaultOptions()
	for _, opt := range opts {
		opt(&cfg)
	}

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("postgres: parse dsn: %w", err)
	}
	poolCfg.MaxConns = cfg.maxConns
	poolCfg.MinConns = cfg.minConns
	poolCfg.MaxConnLifetime = cfg.maxConnLifetime
	poolCfg.HealthCheckPeriod = cfg.healthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("postgres: create pool: %w", err)
	}

	if err := pingWithRetry(ctx, pool, log, cfg); err != nil {
		pool.Close()
		return nil, nil, err
	}

	log.Info("pool ready",
		"max_conns", cfg.maxConns,
		"min_conns", cfg.minConns,
	)

	cleanup := func() {
		pool.Close()
		log.Info("pool closed")
	}
	return pool, cleanup, nil
}

func pingWithRetry(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, cfg options) error {
	var lastErr error
	for attempt := 1; attempt <= cfg.pingRetries; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, cfg.pingTimeout)
		lastErr = pool.Ping(pingCtx)
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
			return fmt.Errorf("postgres: ping canceled: %w", ctx.Err())
		case <-time.After(cfg.pingRetryDelay):
		}
	}
	return fmt.Errorf("postgres: ping after %d attempts: %w", cfg.pingRetries, lastErr)
}
