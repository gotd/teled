-- Associate a canonical message with stored media so it re-renders in history.
ALTER TABLE messages
    ADD COLUMN media_file_id BIGINT REFERENCES files (id) ON DELETE SET NULL;
