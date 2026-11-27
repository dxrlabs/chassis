package sqlite_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/dxrlabs/chassis/db/sqlite"
)

func TestNew_memory(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, cleanup, err := sqlite.New(context.Background(), ":memory:", log)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cleanup()

	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO t(name) VALUES (?)`, "ada"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var name string
	if err := db.QueryRow(`SELECT name FROM t WHERE id = 1`).Scan(&name); err != nil {
		t.Fatalf("select: %v", err)
	}
	if name != "ada" {
		t.Fatalf("name = %q", name)
	}
}

func TestNew_file(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	path := filepath.Join(t.TempDir(), "test.db")
	db, cleanup, err := sqlite.New(context.Background(), path, log)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cleanup()
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestNew_requiresPath(t *testing.T) {
	t.Parallel()
	_, _, err := sqlite.New(context.Background(), "", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
