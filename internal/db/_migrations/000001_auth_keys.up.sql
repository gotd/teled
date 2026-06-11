CREATE TABLE auth_keys
(
    key_id     BYTEA       PRIMARY KEY,
    auth_key   BYTEA       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
