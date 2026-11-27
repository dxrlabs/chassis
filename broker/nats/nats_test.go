package nats_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	natshub "github.com/dxrlabs/chassis/broker/nats"
	natsserver "github.com/nats-io/nats-server/v2/server"
)

func TestPublishSubscribeRoundTrip(t *testing.T) {
	t.Parallel()

	ns, err := natsserver.NewServer(&natsserver.Options{
		Host:       "127.0.0.1",
		Port:       -1,
		NoLog:      true,
		NoSigs:     true,
		DontListen: false,
	})
	if err != nil {
		t.Fatalf("nats server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats server not ready")
	}
	t.Cleanup(ns.Shutdown)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()
	client, cleanup, err := natshub.New(ctx, ns.ClientURL(), log)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cleanup()

	got := make(chan []byte, 1)
	if err := client.Subscribe("demo.test", func(_ context.Context, _ string, payload []byte) error {
		got <- append([]byte(nil), payload...)
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if err := client.Publish(ctx, "demo.test", []byte("ping")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case payload := <-got:
		if string(payload) != "ping" {
			t.Fatalf("payload = %q", payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for message")
	}
}
