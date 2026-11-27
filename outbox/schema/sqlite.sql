CREATE TABLE IF NOT EXISTS chassis_outbox (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    subject       TEXT NOT NULL,
    payload       BLOB NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    published_at  TIMESTAMP NULL,
    locked_until  TIMESTAMP NULL,
    attempts      INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS chassis_outbox_pending_idx
    ON chassis_outbox (published_at, locked_until, id);

CREATE TABLE IF NOT EXISTS chassis_inbox (
    message_id  TEXT PRIMARY KEY,
    received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
