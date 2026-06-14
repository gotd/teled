-- Bots created through BotFather are owned by the user who created them.
ALTER TABLE users
    ADD COLUMN bot_owner_id BIGINT REFERENCES users (id) ON DELETE CASCADE;

-- BotFather is a built-in bot account: the server answers DMs to it directly.
-- The fixed id mirrors real Telegram (93372553) and matches teled.BotFatherID.
-- access_hash is fixed so clients can re-resolve it deterministically.
INSERT INTO users (id, access_hash, username, first_name, is_bot)
VALUES (93372553, 7264819913547, 'BotFather', 'BotFather', true);

-- botfather_sessions tracks a user's position in a multi-step BotFather flow
-- (e.g. /newbot asks for a name, then a username).
CREATE TABLE botfather_sessions
(
    user_id    BIGINT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    step       TEXT   NOT NULL,
    draft_name TEXT   NOT NULL DEFAULT ''
);
