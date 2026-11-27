package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Dialect selects SQL placeholder and claim strategy.
type Dialect string

const (
	DialectPostgres Dialect = "postgres"
	DialectMySQL    Dialect = "mysql"
	DialectSQLite   Dialect = "sqlite"
)

// Message is a durable outbox row awaiting publish.
type Message struct {
	ID        int64
	Subject   string
	Payload   []byte
	CreatedAt time.Time
}

// Store persists outbox and inbox rows.
type Store interface {
	InsertOutbox(ctx context.Context, subject string, payload []byte) error
	// ClaimPending locks up to limit unpublished rows for lease and returns them.
	// Safe for multiple relays (Postgres/MySQL: SKIP LOCKED; SQLite: IMMEDIATE tx).
	ClaimPending(ctx context.Context, limit int, lease time.Duration) ([]Message, error)
	MarkPublished(ctx context.Context, id int64) error
	TryClaimInbox(ctx context.Context, messageID string) (claimed bool, err error)
}

// SQLStore is a Store backed by database/sql.
type SQLStore struct {
	db      *sql.DB
	dialect Dialect
}

// NewSQLStore wraps db for the given dialect. Apply schema migrations before use.
func NewSQLStore(db *sql.DB, dialect Dialect) (*SQLStore, error) {
	if db == nil {
		return nil, fmt.Errorf("outbox: db is required")
	}
	switch dialect {
	case DialectPostgres, DialectMySQL, DialectSQLite:
	default:
		return nil, fmt.Errorf("outbox: unsupported dialect %q", dialect)
	}
	return &SQLStore{db: db, dialect: dialect}, nil
}

func (s *SQLStore) InsertOutbox(ctx context.Context, subject string, payload []byte) error {
	var q string
	switch s.dialect {
	case DialectPostgres:
		q = `INSERT INTO chassis_outbox (subject, payload) VALUES ($1, $2)`
	default:
		q = `INSERT INTO chassis_outbox (subject, payload) VALUES (?, ?)`
	}
	_, err := s.db.ExecContext(ctx, q, subject, payload)
	return err
}

// InsertOutboxTx enqueues within an open transaction (preferred with business writes).
func InsertOutboxTx(ctx context.Context, tx *sql.Tx, dialect Dialect, subject string, payload []byte) error {
	var q string
	switch dialect {
	case DialectPostgres:
		q = `INSERT INTO chassis_outbox (subject, payload) VALUES ($1, $2)`
	default:
		q = `INSERT INTO chassis_outbox (subject, payload) VALUES (?, ?)`
	}
	_, err := tx.ExecContext(ctx, q, subject, payload)
	return err
}

func (s *SQLStore) ClaimPending(ctx context.Context, limit int, lease time.Duration) ([]Message, error) {
	if limit <= 0 {
		limit = 1
	}
	if lease <= 0 {
		lease = 30 * time.Second
	}
	switch s.dialect {
	case DialectPostgres:
		return s.claimPostgres(ctx, limit, lease)
	case DialectMySQL:
		return s.claimMySQL(ctx, limit, lease)
	default:
		return s.claimSQLite(ctx, limit, lease)
	}
}

func (s *SQLStore) claimPostgres(ctx context.Context, limit int, lease time.Duration) ([]Message, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
WITH cte AS (
  SELECT id FROM chassis_outbox
  WHERE published_at IS NULL
    AND (locked_until IS NULL OR locked_until < NOW())
  ORDER BY id
  LIMIT $1
  FOR UPDATE SKIP LOCKED
)
UPDATE chassis_outbox o
SET locked_until = NOW() + make_interval(secs => $2),
    attempts = attempts + 1
FROM cte
WHERE o.id = cte.id
RETURNING o.id, o.subject, o.payload, o.created_at`, limit, int(lease.Seconds()))
	if err != nil {
		return nil, err
	}
	msgs, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (s *SQLStore) claimMySQL(ctx context.Context, limit int, lease time.Duration) ([]Message, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
SELECT id FROM chassis_outbox
WHERE published_at IS NULL
  AND (locked_until IS NULL OR locked_until < UTC_TIMESTAMP())
ORDER BY id
LIMIT ?
FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	secs := int(lease.Seconds())
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `
UPDATE chassis_outbox
SET locked_until = TIMESTAMPADD(SECOND, ?, UTC_TIMESTAMP()),
    attempts = attempts + 1
WHERE id = ?`, secs, id); err != nil {
			return nil, err
		}
	}

	msgs, err := s.loadByIDsTx(ctx, tx, ids, true)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (s *SQLStore) claimSQLite(ctx context.Context, limit int, lease time.Duration) ([]Message, error) {
	// A write transaction serializes access; SQLite has no SKIP LOCKED.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
SELECT id FROM chassis_outbox
WHERE published_at IS NULL
  AND (locked_until IS NULL OR locked_until < CURRENT_TIMESTAMP)
ORDER BY id
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, tx.Commit()
	}

	until := time.Now().UTC().Add(lease).Format("2006-01-02 15:04:05")
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `
UPDATE chassis_outbox
SET locked_until = ?, attempts = attempts + 1
WHERE id = ?`, until, id); err != nil {
			return nil, err
		}
	}
	msgs, err := s.loadByIDsTx(ctx, tx, ids, false)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (s *SQLStore) loadByIDsTx(ctx context.Context, tx *sql.Tx, ids []int64, mysql bool) ([]Message, error) {
	out := make([]Message, 0, len(ids))
	for _, id := range ids {
		var q string
		if mysql || s.dialect != DialectPostgres {
			q = `SELECT id, subject, payload, created_at FROM chassis_outbox WHERE id = ?`
		} else {
			q = `SELECT id, subject, payload, created_at FROM chassis_outbox WHERE id = $1`
		}
		var m Message
		if err := tx.QueryRowContext(ctx, q, id).Scan(&m.ID, &m.Subject, &m.Payload, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func scanMessages(rows *sql.Rows) ([]Message, error) {
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Subject, &m.Payload, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *SQLStore) MarkPublished(ctx context.Context, id int64) error {
	var q string
	switch s.dialect {
	case DialectPostgres:
		q = `UPDATE chassis_outbox SET published_at = NOW(), locked_until = NULL WHERE id = $1`
	case DialectMySQL:
		q = `UPDATE chassis_outbox SET published_at = UTC_TIMESTAMP(), locked_until = NULL WHERE id = ?`
	default:
		q = `UPDATE chassis_outbox SET published_at = CURRENT_TIMESTAMP, locked_until = NULL WHERE id = ?`
	}
	_, err := s.db.ExecContext(ctx, q, id)
	return err
}

func (s *SQLStore) TryClaimInbox(ctx context.Context, messageID string) (bool, error) {
	var q string
	switch s.dialect {
	case DialectPostgres:
		q = `INSERT INTO chassis_inbox (message_id) VALUES ($1) ON CONFLICT DO NOTHING`
	case DialectMySQL:
		q = `INSERT IGNORE INTO chassis_inbox (message_id) VALUES (?)`
	default:
		q = `INSERT OR IGNORE INTO chassis_inbox (message_id) VALUES (?)`
	}
	res, err := s.db.ExecContext(ctx, q, messageID)
	if err != nil {
		if isDuplicate(err) {
			return false, nil
		}
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func isDuplicate(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate") ||
		strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "unique constraint")
}
