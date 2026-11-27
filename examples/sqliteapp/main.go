// Command sqliteapp is a zero-dependency-local example: SQLite + HTTP + outbox table.
package main

import (
	"context"
	"database/sql"
	"embed"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/dxrlabs/chassis/config"
	"github.com/dxrlabs/chassis/db/sqlite"
	"github.com/dxrlabs/chassis/httpx"
	"github.com/dxrlabs/chassis/logger"
	"github.com/dxrlabs/chassis/migrate"
	"github.com/dxrlabs/chassis/outbox"
	"github.com/dxrlabs/chassis/server"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type AppConfig struct {
	Env      string `env:"ENV" envDefault:"development"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"INFO"`
	HTTPAddr string `env:"HTTP_ADDR" envDefault:":8081"`
	SQLite   string `env:"SQLITE_PATH" envDefault:"file:sqliteapp.db?cache=shared"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load[AppConfig]()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	log, closeLogger, err := logger.New(cfg.Env, slog.LevelInfo)
	if err != nil {
		slog.Error("logger", "err", err)
		os.Exit(1)
	}
	defer closeLogger()

	db, closeDB, err := sqlite.New(ctx, cfg.SQLite, log)
	if err != nil {
		log.Error("sqlite", "err", err)
		os.Exit(1)
	}
	defer closeDB()

	if err := migrate.Up(ctx, cfg.SQLite, migrationsFS, "migrations", log, migrate.DialectSQLite); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}

	store, err := outbox.NewSQLStore(db, outbox.DialectSQLite)
	if err != nil {
		log.Error("outbox store", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("POST /api/notes", httpx.RequireJSON(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type req struct {
			Body string `json:"body"`
		}
		body, err := httpx.DecodeJSON[req](r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if err := insertNote(r.Context(), db, body.Body); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, map[string]string{"status": "created"})
	})))
	_ = store // ready for NewRelay when a broker is wired

	srv, shutdownServer, err := server.New(cfg.HTTPAddr, httpx.RequestID(mux), log,
		server.WithReadyCheck(func(c context.Context) bool { return db.PingContext(c) == nil }),
	)
	if err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}
	defer shutdownServer()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			log.Error("server exited", "err", err)
			os.Exit(1)
		}
	}
}

func insertNote(ctx context.Context, db *sql.DB, body string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `INSERT INTO notes (body) VALUES (?)`, body); err != nil {
		return err
	}
	if err := outbox.InsertOutboxTx(ctx, tx, outbox.DialectSQLite, "notes.created", []byte(body)); err != nil {
		return err
	}
	return tx.Commit()
}
