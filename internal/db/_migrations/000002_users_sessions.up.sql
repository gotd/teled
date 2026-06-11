CREATE TABLE users
(
    id          BIGSERIAL   PRIMARY KEY,
    access_hash BIGINT      NOT NULL DEFAULT 0,
    phone       TEXT        UNIQUE,
    username    TEXT        UNIQUE,
    first_name  TEXT        NOT NULL DEFAULT '',
    last_name   TEXT        NOT NULL DEFAULT '',
    about       TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE phone_codes
(
    phone      TEXT        NOT NULL,
    code_hash  TEXT        NOT NULL,
    code       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (phone, code_hash)
);

-- sessions binds an MTProto auth key to a logged-in user.
CREATE TABLE sessions
(
    key_id  BYTEA  PRIMARY KEY REFERENCES auth_keys (key_id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE contacts
(
    owner_user_id   BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    contact_user_id BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    first_name      TEXT   NOT NULL DEFAULT '',
    last_name       TEXT   NOT NULL DEFAULT '',
    PRIMARY KEY (owner_user_id, contact_user_id)
);
