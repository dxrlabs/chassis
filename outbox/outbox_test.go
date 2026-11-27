package outbox_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dxrlabs/chassis/outbox"
	_ "modernc.org/sqlite"
)

type memPub struct {
	mu   sync.Mutex
	msgs []struct {
		subject string
		payload []byte
	}
}

func (p *memPub) Publish(_ context.Context, subject string, payload []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.msgs = append(p.msgs, struct {
		subject string
		payload []byte
	}{subject, append([]byte(nil), payload...)})
	return nil
}

func TestOutboxRelayAndInbox(t *testing.T) {
	t.Parallel()
	db := openSQLite(t)
	store, err := outbox.NewSQLStore(db, outbox.DialectSQLite)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := store.InsertOutbox(ctx, "demo.evt", []byte("hello")); err != nil {
		t.Fatalf("insert: %v", err)
	}

	pub := &memPub{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, cleanup, err := outbox.NewRelay(store, pub, log,
		outbox.WithPollInterval(20*time.Millisecond),
		outbox.WithBatchSize(10),
		outbox.WithLease(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	deadline := time.Now().Add(2 * time.Second)
	for {
		pub.mu.Lock()
		n := len(pub.msgs)
		pub.mu.Unlock()
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("relay did not publish")
		}
		time.Sleep(20 * time.Millisecond)
	}

	h := outbox.InboxHandler(store, func(string, []byte) string { return "msg-1" },
		func(context.Context, string, []byte) error { return nil })
	if err := h(ctx, "demo.evt", []byte("hello")); err != nil {
		t.Fatalf("inbox first: %v", err)
	}
	if err := h(ctx, "demo.evt", []byte("hello")); err != nil {
		t.Fatalf("inbox second: %v", err)
	}
}

func TestClaimPending_exclusiveLease(t *testing.T) {
	t.Parallel()
	db := openSQLite(t)
	store, err := outbox.NewSQLStore(db, outbox.DialectSQLite)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.InsertOutbox(ctx, "x", []byte("1")); err != nil {
		t.Fatal(err)
	}

	first, err := store.ClaimPending(ctx, 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 {
		t.Fatalf("first claim = %d", len(first))
	}
	second, err := store.ClaimPending(ctx, 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("second claim should be empty while leased, got %d", len(second))
	}
}

func openSQLite(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "outbox.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("schema/sqlite.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
