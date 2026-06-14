DROP TABLE botfather_sessions;

DELETE FROM users WHERE id = 93372553;

ALTER TABLE users DROP COLUMN bot_owner_id;
