CREATE TABLE IF NOT EXISTS chassis_outbox (
    id            BIGSERIAL PRIMARY KEY,
    subject       TEXT NOT NULL,
    payload       BYTEA NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at  TIMESTAMPTZ NULL,
    locked_until  TIMESTAMPTZ NULL,
    attempts      INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS chassis_outbox_pending_idx
    ON chassis_outbox (id)
    WHERE published_at IS NULL;

CREATE TABLE IF NOT EXISTS chassis_inbox (
    message_id  TEXT PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
