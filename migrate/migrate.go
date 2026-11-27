// Package migrate runs SQL migrations via goose (raw .sql files) for
// Postgres, MySQL, and SQLite. Spanner uses migrate/spanner instead.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

// Dialect selects the goose dialect and database/sql driver.
type Dialect string

const (
	DialectPostgres Dialect = "postgres"
	DialectMySQL    Dialect = "mysql"
	DialectSQLite   Dialect = "sqlite3"
)

// Up applies all pending migrations from migrationsFS under dir using dialect.
// For Postgres, dsn is a pgx URL; for MySQL a go-sql-driver DSN; for SQLite a file path.
func Up(ctx context.Context, dsn string, migrationsFS fs.FS, dir string, log *slog.Logger, dialect Dialect) error {
	if dsn == "" {
		return fmt.Errorf("migrate: dsn is required")
	}
	if migrationsFS == nil {
		return fmt.Errorf("migrate: migrations FS is required")
	}
	if dir == "" {
		dir = "."
	}
	if dialect == "" {
		dialect = DialectPostgres
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.With("component", "migrate", "dialect", string(dialect))

	driver, gooseDialect, err := mapDialect(dialect)
	if err != nil {
		return err
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return fmt.Errorf("migrate: open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("migrate: ping: %w", err)
	}

	if err := goose.SetDialect(gooseDialect); err != nil {
		return fmt.Errorf("migrate: dialect: %w", err)
	}
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(&slogGooseLogger{log: log})

	if err := goose.UpContext(ctx, db, dir); err != nil {
		return fmt.Errorf("migrate: up: %w", err)
	}

	log.Info("migrations applied", "dir", dir)
	return nil
}

func mapDialect(d Dialect) (driver, gooseDialect string, err error) {
	switch d {
	case DialectPostgres:
		return "pgx", "postgres", nil
	case DialectMySQL:
		return "mysql", "mysql", nil
	case DialectSQLite:
		return "sqlite", "sqlite3", nil
	default:
		return "", "", fmt.Errorf("migrate: unsupported dialect %q", d)
	}
}

type slogGooseLogger struct {
	log *slog.Logger
}

func (l *slogGooseLogger) Fatalf(format string, v ...any) {
	l.log.Error(fmt.Sprintf(format, v...))
}

func (l *slogGooseLogger) Printf(format string, v ...any) {
	l.log.Info(fmt.Sprintf(format, v...))
}
