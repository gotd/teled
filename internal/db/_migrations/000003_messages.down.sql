DROP TABLE message_refs;
DROP TABLE messages;
ALTER TABLE users
    DROP COLUMN last_message_id,
    DROP COLUMN pts;
