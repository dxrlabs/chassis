package redis_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/dxrlabs/chassis/redis"
)

func TestNew_requiresAddr(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, _, err := redis.New(context.Background(), "", log)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNew_pingFailsFastWithCancel(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := redis.New(ctx, "127.0.0.1:1", log,
		redis.WithPoolSize(1),
	)
	if err == nil {
		t.Fatal("expected error against canceled context / bad addr")
	}
	if !errors.Is(err, context.Canceled) && ctx.Err() != nil {
		// Either canceled during retry or connection refused after attempts.
		t.Logf("got err (ok): %v", err)
	}
}

func TestNew_unreachableAddr(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Port 1 is almost never a Redis instance; expect ping failure.
	_, cleanup, err := redis.New(ctx, "127.0.0.1:1", log,
		redis.WithPingRetries(1),
		redis.WithPingRetryDelay(0),
	)
	if err == nil {
		cleanup()
		t.Fatal("expected ping failure")
	}
}
