// Package mysql opens a MySQL database/sql pool with chassis lifecycle management.
package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	defaultMaxOpenConns    = 10
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = time.Hour
	defaultPingTimeout     = 10 * time.Second
	defaultPingRetries     = 5
	defaultPingRetryDelay  = time.Second
)

// Option customizes pool creation.
type Option func(*options)

type options struct {
	maxOpenConns    int
	maxIdleConns    int
	connMaxLifetime time.Duration
	pingTimeout     time.Duration
	pingRetries     int
	pingRetryDelay  time.Duration
}

func defaultOptions() options {
	return options{
		maxOpenConns:    defaultMaxOpenConns,
		maxIdleConns:    defaultMaxIdleConns,
		connMaxLifetime: defaultConnMaxLifetime,
		pingTimeout:     defaultPingTimeout,
		pingRetries:     defaultPingRetries,
		pingRetryDelay:  defaultPingRetryDelay,
	}
}

// WithMaxOpenConns sets database/sql MaxOpenConns.
func WithMaxOpenConns(n int) Option {
	return func(o *options) { o.maxOpenConns = n }
}

// WithMaxIdleConns sets database/sql MaxIdleConns.
func WithMaxIdleConns(n int) Option {
	return func(o *options) { o.maxIdleConns = n }
}

// New opens dsn (user:pass@tcp(host:port)/dbname?parseTime=true), pings with retry, returns cleanup.
func New(ctx context.Context, dsn string, log *slog.Logger, opts ...Option) (*sql.DB, func(), error) {
	if dsn == "" {
		return nil, nil, fmt.Errorf("mysql: dsn is required")
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.With("component", "db/mysql")

	cfg := defaultOptions()
	for _, opt := range opts {
		opt(&cfg)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("mysql: open: %w", err)
	}
	db.SetMaxOpenConns(cfg.maxOpenConns)
	db.SetMaxIdleConns(cfg.maxIdleConns)
	db.SetConnMaxLifetime(cfg.connMaxLifetime)

	if err := pingWithRetry(ctx, db, log, cfg); err != nil {
		_ = db.Close()
		return nil, nil, err
	}

	log.Info("ready", "max_open", cfg.maxOpenConns)
	cleanup := func() {
		if err := db.Close(); err != nil {
			log.Warn("close failed", "err", err)
			return
		}
		log.Info("closed")
	}
	return db, cleanup, nil
}

func pingWithRetry(ctx context.Context, db *sql.DB, log *slog.Logger, cfg options) error {
	var lastErr error
	for attempt := 1; attempt <= cfg.pingRetries; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, cfg.pingTimeout)
		lastErr = db.PingContext(pingCtx)
		cancel()
		if lastErr == nil {
			return nil
		}
		log.Warn("ping failed", "attempt", attempt, "retries", cfg.pingRetries, "err", lastErr)
		if attempt == cfg.pingRetries {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("mysql: ping canceled: %w", ctx.Err())
		case <-time.After(cfg.pingRetryDelay):
		}
	}
	return fmt.Errorf("mysql: ping after %d attempts: %w", cfg.pingRetries, lastErr)
}
