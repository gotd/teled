-- Per-account counters: local message-id space and the common update pts.
ALTER TABLE users
    ADD COLUMN last_message_id BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN pts             BIGINT NOT NULL DEFAULT 0;

-- Canonical message content (one row per logical message, shared by both
-- participants of a DM).
CREATE TABLE messages
(
    global_id          BIGSERIAL   PRIMARY KEY,
    from_user_id       BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    peer_user_id       BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    text               TEXT        NOT NULL DEFAULT '',
    date               TIMESTAMPTZ NOT NULL DEFAULT now(),
    edit_date          TIMESTAMPTZ,
    reply_to_global_id BIGINT      REFERENCES messages (global_id) ON DELETE SET NULL,
    random_id          BIGINT      NOT NULL DEFAULT 0,
    deleted            BOOLEAN     NOT NULL DEFAULT FALSE
);

-- Per-account view: Telegram message ids are per-account local sequences, so
-- each participant references the canonical message under their own id.
CREATE TABLE message_refs
(
    user_id    BIGINT  NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    message_id BIGINT  NOT NULL,
    global_id  BIGINT  NOT NULL REFERENCES messages (global_id) ON DELETE CASCADE,
    out        BOOLEAN NOT NULL,
    unread     BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (user_id, message_id),
    UNIQUE (user_id, global_id)
);

CREATE INDEX message_refs_global_idx ON message_refs (global_id);
