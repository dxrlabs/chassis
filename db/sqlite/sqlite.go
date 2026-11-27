// Package sqlite opens a modernc.org/sqlite database with chassis lifecycle.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "modernc.org/sqlite"
)

const (
	defaultPingTimeout     = 5 * time.Second
	defaultMaxOpenConns    = 1 // SQLite writes are serialized; keep pool small
	defaultMaxIdleConns    = 1
	defaultConnMaxLifetime = time.Hour
)

// Option customizes SQLite open behavior.
type Option func(*options)

type options struct {
	maxOpenConns    int
	maxIdleConns    int
	connMaxLifetime time.Duration
	pingTimeout     time.Duration
	busyTimeout     time.Duration
}

func defaultOptions() options {
	return options{
		maxOpenConns:    defaultMaxOpenConns,
		maxIdleConns:    defaultMaxIdleConns,
		connMaxLifetime: defaultConnMaxLifetime,
		pingTimeout:     defaultPingTimeout,
		busyTimeout:     5 * time.Second,
	}
}

// WithMaxOpenConns overrides the connection pool size (default 1).
func WithMaxOpenConns(n int) Option {
	return func(o *options) { o.maxOpenConns = n }
}

// WithBusyTimeout sets the SQLite busy_timeout pragma.
func WithBusyTimeout(d time.Duration) Option {
	return func(o *options) { o.busyTimeout = d }
}

// New opens path (use ":memory:" for an in-memory DB), pings, and returns cleanup.
func New(ctx context.Context, path string, log *slog.Logger, opts ...Option) (*sql.DB, func(), error) {
	if path == "" {
		return nil, nil, fmt.Errorf("sqlite: path is required")
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.With("component", "db/sqlite")

	cfg := defaultOptions()
	for _, opt := range opts {
		opt(&cfg)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite: open: %w", err)
	}
	db.SetMaxOpenConns(cfg.maxOpenConns)
	db.SetMaxIdleConns(cfg.maxIdleConns)
	db.SetConnMaxLifetime(cfg.connMaxLifetime)

	pingCtx, cancel := context.WithTimeout(ctx, cfg.pingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("sqlite: ping: %w", err)
	}

	if cfg.busyTimeout > 0 {
		ms := int(cfg.busyTimeout / time.Millisecond)
		if _, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", ms)); err != nil {
			_ = db.Close()
			return nil, nil, fmt.Errorf("sqlite: busy_timeout: %w", err)
		}
	}
	// WAL is a sensible default for file-backed DBs.
	if path != ":memory:" {
		if _, err := db.ExecContext(ctx, "PRAGMA journal_mode = WAL"); err != nil {
			log.Warn("wal pragma failed", "err", err)
		}
	}

	log.Info("ready", "path", path)
	cleanup := func() {
		if err := db.Close(); err != nil {
			log.Warn("close failed", "err", err)
			return
		}
		log.Info("closed")
	}
	return db, cleanup, nil
}
