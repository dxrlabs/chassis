// Command fullapp wires chassis modules with LIFO defer teardown.
package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dxrlabs/chassis/broker"
	natshub "github.com/dxrlabs/chassis/broker/nats"
	"github.com/dxrlabs/chassis/config"
	"github.com/dxrlabs/chassis/db/postgres"
	"github.com/dxrlabs/chassis/httpx"
	"github.com/dxrlabs/chassis/logger"
	"github.com/dxrlabs/chassis/migrate"
	"github.com/dxrlabs/chassis/outbox"
	"github.com/dxrlabs/chassis/redis"
	"github.com/dxrlabs/chassis/server"
	"github.com/dxrlabs/chassis/telemetry"
	"github.com/dxrlabs/chassis/worker"
	"github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// AppConfig is loaded from the environment via chassis/config.
type AppConfig struct {
	Env          string `env:"ENV" envDefault:"development"`
	LogLevel     string `env:"LOG_LEVEL" envDefault:"INFO"`
	HTTPAddr     string `env:"HTTP_ADDR" envDefault:":8080"`
	DatabaseURL  string `env:"DATABASE_URL,required"`
	RedisAddr    string `env:"REDIS_ADDR" envDefault:"127.0.0.1:6379"`
	NATSURL      string `env:"NATS_URL" envDefault:"nats://127.0.0.1:4222"`
	OTLPEndpoint string `env:"OTLP_ENDPOINT"`
	ServiceName  string `env:"SERVICE_NAME" envDefault:"fullapp"`
	Version      string `env:"VERSION" envDefault:"0.0.0"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load[AppConfig]()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	level := parseLevel(cfg.LogLevel)
	log, closeLogger, err := logger.New(cfg.Env, level)
	if err != nil {
		slog.Error("logger", "err", err)
		os.Exit(1)
	}
	defer closeLogger()

	shutdownTelemetry, err := telemetry.New(ctx, cfg.ServiceName, cfg.Version, cfg.OTLPEndpoint)
	if err != nil {
		log.Error("telemetry", "err", err)
		os.Exit(1)
	}
	defer shutdownTelemetry()

	pool, closeDB, err := postgres.New(ctx, cfg.DatabaseURL, log)
	if err != nil {
		log.Error("postgres", "err", err)
		os.Exit(1)
	}
	defer closeDB()

	if err := migrate.Up(ctx, cfg.DatabaseURL, migrationsFS, "migrations", log, migrate.DialectPostgres); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}

	sqlDB := stdlib.OpenDBFromPool(pool)
	// Do not close sqlDB: it shares the pool; closeDB owns lifecycle.

	rdb, closeRedis, err := redis.New(ctx, cfg.RedisAddr, log)
	if err != nil {
		log.Error("redis", "err", err)
		os.Exit(1)
	}
	defer closeRedis()

	bus, closeBroker, err := natshub.New(ctx, cfg.NATSURL, log)
	if err != nil {
		log.Error("broker", "err", err)
		os.Exit(1)
	}
	defer closeBroker()
	var _ broker.Bus = bus

	store, err := outbox.NewSQLStore(sqlDB, outbox.DialectPostgres)
	if err != nil {
		log.Error("outbox store", "err", err)
		os.Exit(1)
	}
	_, stopRelay, err := outbox.NewRelay(store, bus, log, outbox.WithPollInterval(500*time.Millisecond))
	if err != nil {
		log.Error("outbox relay", "err", err)
		os.Exit(1)
	}
	defer stopRelay()

	runners, stopWorkers, err := worker.New(log)
	if err != nil {
		log.Error("worker", "err", err)
		os.Exit(1)
	}
	defer stopWorkers()

	_ = runners.Every(time.Minute, "heartbeat", func(context.Context) error {
		log.Info("heartbeat")
		return nil
	})

	if err := bus.Subscribe("demo.ping", outbox.InboxHandler(store,
		func(_ string, payload []byte) string { return "demo.ping:" + string(payload) },
		func(ctx context.Context, subject string, payload []byte) error {
			log.Info("message", "subject", subject, "bytes", len(payload))
			return rdb.Set(ctx, "fullapp:last_ping", string(payload), time.Hour).Err()
		},
	)); err != nil {
		log.Error("subscribe", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /api/ping", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "pong"})
	}))
	mux.Handle("POST /api/echo", httpx.RequireJSON(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type req struct {
			Message string `json:"message"`
		}
		body, err := httpx.DecodeJSON[req](r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if err := enqueueEcho(r.Context(), sqlDB, body.Message); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "queued", "message": body.Message})
	})))

	handler := telemetry.HTTPMiddleware(cfg.ServiceName, httpx.RequestID(httpx.CORS(httpx.CORSConfig{})(mux)))

	srv, shutdownServer, err := server.New(cfg.HTTPAddr, handler, log,
		server.WithReadyCheck(func(c context.Context) bool {
			if pool.Ping(c) != nil {
				return false
			}
			return rdb.Ping(c).Err() == nil
		}),
		server.WithShutdownTimeout(15*time.Second),
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

func enqueueEcho(ctx context.Context, sqlDB *sql.DB, message string) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `INSERT INTO items (name) VALUES ($1)`, message); err != nil {
		return fmt.Errorf("insert item: %w", err)
	}
	if err := outbox.InsertOutboxTx(ctx, tx, outbox.DialectPostgres, "demo.ping", []byte(message)); err != nil {
		return fmt.Errorf("outbox: %w", err)
	}
	return tx.Commit()
}

func parseLevel(s string) slog.Level {
	var level slog.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return level
}
