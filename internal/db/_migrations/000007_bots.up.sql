-- Bot accounts: marked by is_bot and authenticated by an opaque bot_token
-- instead of a phone number.
ALTER TABLE users
    ADD COLUMN is_bot    BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN bot_token TEXT    UNIQUE;

-- bot_commands stores a bot's published command list per (scope, language).
-- scope is the hex-encoded MTProto BotCommandScope so every scope variant,
-- including peer-specific ones, maps to a distinct row. commands is the
-- ordered list as JSON ([{command, description}, ...]).
CREATE TABLE bot_commands
(
    bot_user_id BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    scope       TEXT   NOT NULL,
    lang_code   TEXT   NOT NULL DEFAULT '',
    commands    JSONB  NOT NULL DEFAULT '[]'::jsonb,
    PRIMARY KEY (bot_user_id, scope, lang_code)
);
