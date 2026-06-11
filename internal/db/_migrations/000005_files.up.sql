CREATE TABLE files
(
    id             BIGSERIAL   PRIMARY KEY,
    owner_user_id  BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    access_hash    BIGINT      NOT NULL,
    object_key     TEXT        NOT NULL,
    size           BIGINT      NOT NULL,
    mime           TEXT        NOT NULL DEFAULT '',
    sha256         BYTEA       NOT NULL,
    file_reference BYTEA       NOT NULL,
    kind           TEXT        NOT NULL DEFAULT 'photo',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
