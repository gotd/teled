-- drafts holds an account's unsent message per conversation.
CREATE TABLE drafts
(
    user_id      BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    peer_user_id BIGINT      NOT NULL,
    message      TEXT        NOT NULL,
    date         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, peer_user_id)
);
