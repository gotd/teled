-- Durable per-account update log for updates.getDifference.
CREATE TABLE updates_log
(
    user_id   BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    pts       BIGINT      NOT NULL,
    pts_count INT         NOT NULL DEFAULT 1,
    type      TEXT        NOT NULL,
    global_id BIGINT,
    extra     JSONB,
    date      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, pts)
);
