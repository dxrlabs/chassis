// Package logger configures a process-wide slog logger with a cleanup hook.
package logger

import (
	"io"
	"log/slog"
	"os"
)

// syncer is implemented by writers that buffer and need an explicit flush.
type syncer interface {
	Sync() error
}

// New builds a slog.Logger for env. Production uses JSON; otherwise text.
// The returned cleanup flushes any Sync()-capable underlying writers.
func New(env string, level slog.Level) (*slog.Logger, func(), error) {
	w := io.Writer(os.Stdout)
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if env == "production" || env == "prod" {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	log := slog.New(handler)
	slog.SetDefault(log)

	cleanup := func() {
		if s, ok := w.(syncer); ok {
			_ = s.Sync()
		}
	}
	return log, cleanup, nil
}
