// Package spanner applies ordered DDL files to a Cloud Spanner database.
// Goose is not used — Spanner schema changes go through the Database Admin API.
package spanner

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"sort"
	"strings"
	"time"

	gspanner "cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
)

const versionTable = "chassis_schema_migrations"

// Up applies pending *.sql files from migrationsFS (sorted by name).
// databasePath is projects/.../instances/.../databases/...
// Each file may contain multiple DDL statements separated by semicolons.
func Up(ctx context.Context, databasePath string, migrationsFS fs.FS, dir string, log *slog.Logger) error {
	if databasePath == "" {
		return fmt.Errorf("migrate/spanner: database is required")
	}
	if migrationsFS == nil {
		return fmt.Errorf("migrate/spanner: migrations FS is required")
	}
	if dir == "" {
		dir = "."
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.With("component", "migrate/spanner")

	files, err := loadFiles(migrationsFS, dir)
	if err != nil {
		return err
	}

	adminClient, err := database.NewDatabaseAdminClient(ctx)
	if err != nil {
		return fmt.Errorf("migrate/spanner: admin client: %w", err)
	}
	defer func() { _ = adminClient.Close() }()

	dataClient, err := gspanner.NewClient(ctx, databasePath)
	if err != nil {
		return fmt.Errorf("migrate/spanner: data client: %w", err)
	}
	defer dataClient.Close()

	if err := ensureVersionTable(ctx, adminClient, databasePath, log); err != nil {
		return err
	}

	applied, err := listApplied(ctx, dataClient)
	if err != nil {
		return err
	}

	for _, f := range files {
		if applied[f.name] {
			continue
		}
		stmts := splitDDL(f.body)
		if len(stmts) == 0 {
			continue
		}
		op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
			Database:   databasePath,
			Statements: stmts,
		})
		if err != nil {
			return fmt.Errorf("migrate/spanner: apply %s: %w", f.name, err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		err = op.Wait(waitCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("migrate/spanner: wait %s: %w", f.name, err)
		}
		if err := recordApplied(ctx, dataClient, f.name); err != nil {
			return fmt.Errorf("migrate/spanner: record %s: %w", f.name, err)
		}
		log.Info("applied", "version", f.name)
	}
	return nil
}

type fileDDL struct {
	name string
	body string
}

func loadFiles(migrationsFS fs.FS, dir string) ([]fileDDL, error) {
	entries, err := fs.ReadDir(migrationsFS, dir)
	if err != nil {
		return nil, fmt.Errorf("migrate/spanner: read dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	out := make([]fileDDL, 0, len(names))
	for _, name := range names {
		raw, err := fs.ReadFile(migrationsFS, path.Join(dir, name))
		if err != nil {
			return nil, err
		}
		out = append(out, fileDDL{name: name, body: string(raw)})
	}
	return out, nil
}

func splitDDL(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" || strings.HasPrefix(part, "--") {
			continue
		}
		out = append(out, part)
	}
	return out
}

func ensureVersionTable(ctx context.Context, admin *database.DatabaseAdminClient, databasePath string, log *slog.Logger) error {
	op, err := admin.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database: databasePath,
		Statements: []string{`CREATE TABLE chassis_schema_migrations (
  version STRING(128) NOT NULL,
  applied_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true)
) PRIMARY KEY (version)`},
	})
	if err != nil {
		if alreadyExists(err) {
			return nil
		}
		return fmt.Errorf("migrate/spanner: create version table: %w", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := op.Wait(waitCtx); err != nil {
		if alreadyExists(err) {
			return nil
		}
		return err
	}
	log.Info("ensured version table", "table", versionTable)
	return nil
}

func listApplied(ctx context.Context, client *gspanner.Client) (map[string]bool, error) {
	iter := client.Single().Query(ctx, gspanner.Statement{
		SQL: `SELECT version FROM chassis_schema_migrations`,
	})
	defer iter.Stop()
	out := map[string]bool{}
	if err := iter.Do(func(row *gspanner.Row) error {
		var v string
		if err := row.Columns(&v); err != nil {
			return err
		}
		out[v] = true
		return nil
	}); err != nil {
		return nil, fmt.Errorf("migrate/spanner: list versions: %w", err)
	}
	return out, nil
}

func recordApplied(ctx context.Context, client *gspanner.Client, version string) error {
	_, err := client.Apply(ctx, []*gspanner.Mutation{
		gspanner.Insert(versionTable, []string{"version", "applied_at"}, []any{version, gspanner.CommitTimestamp}),
	})
	return err
}

func alreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate name") ||
		strings.Contains(msg, "Already exists") ||
		strings.Contains(msg, "already exists")
}
