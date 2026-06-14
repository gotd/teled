DROP TABLE bot_commands;

ALTER TABLE users
    DROP COLUMN bot_token,
    DROP COLUMN is_bot;
