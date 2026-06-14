package db

import (
	"context"
	"strconv"

	"github.com/go-faster/errors"

	"github.com/gotd/teled"
)

// GetDialogs returns the caller's conversations, newest activity first.
func (db *DB) GetDialogs(ctx context.Context, self int64, limit int) ([]teled.Dialog, error) {
	q := `
SELECT peer,
       MAX(message_id) AS top,
       COUNT(*) FILTER (WHERE unread AND NOT out) AS unread,
       COALESCE(MAX(message_id) FILTER (WHERE NOT out AND NOT unread), 0) AS read_inbox
FROM (
    SELECT r.message_id, r.out, r.unread,
           CASE WHEN m.from_user_id = $1 THEN m.peer_user_id ELSE m.from_user_id END AS peer
    FROM message_refs r
    JOIN messages m ON m.global_id = r.global_id
    WHERE r.user_id = $1 AND NOT m.deleted
) s
GROUP BY peer
ORDER BY top DESC
LIMIT ` + strconv.Itoa(limit)

	rows, err := db.pool.Query(ctx, q, self)
	if err != nil {
		return nil, errors.Wrap(err, "query")
	}
	defer rows.Close()

	var dialogs []teled.Dialog

	for rows.Next() {
		var d teled.Dialog
		if err := rows.Scan(&d.PeerUserID, &d.TopMessageID, &d.UnreadCount, &d.ReadInboxMaxID); err != nil {
			return nil, errors.Wrap(err, "scan")
		}

		dialogs = append(dialogs, d)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "rows")
	}

	return dialogs, nil
}

// ReadHistory marks the caller's incoming messages from peer up to maxID as
// read and allocates a pts for the read event.
func (db *DB) ReadHistory(ctx context.Context, self, peer, maxID int64) (pts int, err error) {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "begin")
	}

	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
UPDATE message_refs r SET unread = false
FROM messages m
WHERE r.global_id = m.global_id AND r.user_id = $1 AND NOT r.out AND r.message_id <= $2
  AND (CASE WHEN m.from_user_id = $1 THEN m.peer_user_id ELSE m.from_user_id END) = $3`,
		self, maxID, peer,
	); err != nil {
		return 0, errors.Wrap(err, "mark read")
	}

	if pts, err = allocatePts(ctx, tx, self, 1); err != nil {
		return 0, errors.Wrap(err, "allocate")
	}

	extra := teled.EncodeRead(peer, maxID)
	if err := logUpdate(ctx, tx, self, pts, 1, updReadInbox, nil, extra); err != nil {
		return 0, errors.Wrap(err, "log read")
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, errors.Wrap(err, "commit")
	}

	return pts, nil
}
