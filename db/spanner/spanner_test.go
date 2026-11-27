package spanner_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/dxrlabs/chassis/db/spanner"
)

func TestNew_requiresDatabase(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, _, err := spanner.New(context.Background(), "", log)
	if err == nil {
		t.Fatal("expected error")
	}
}
