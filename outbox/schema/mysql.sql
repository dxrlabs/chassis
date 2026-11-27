CREATE TABLE IF NOT EXISTS chassis_outbox (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    subject       VARCHAR(512) NOT NULL,
    payload       BLOB NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    published_at  TIMESTAMP NULL,
    locked_until  TIMESTAMP NULL,
    attempts      INT NOT NULL DEFAULT 0,
    INDEX chassis_outbox_pending_idx (published_at, locked_until, id)
);

CREATE TABLE IF NOT EXISTS chassis_inbox (
    message_id  VARCHAR(128) PRIMARY KEY,
    received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
