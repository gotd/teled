-- temp_auth_keys maps a temporary (PFS) auth key to the permanent auth key it
-- was bound to via auth.bindTempAuthKey. Requests arriving on the temp key are
-- resolved to the permanent key so the logged-in user binding (kept on the
-- permanent key) survives temp-key rotation and server restarts.
CREATE TABLE temp_auth_keys
(
    temp_key_id BYTEA       PRIMARY KEY,
    perm_key_id BYTEA       NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
