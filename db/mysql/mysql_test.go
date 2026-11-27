package mysql_test

import (
	"context"
	"testing"

	"github.com/dxrlabs/chassis/db/mysql"
)

func TestNew_requiresDSN(t *testing.T) {
	t.Parallel()
	_, _, err := mysql.New(context.Background(), "", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
