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
       COALESCE(MAX(message_id) FILTER (WHERE NOT out AND NOT unread), 0) AS read_inbox,
       COALESCE(MAX(message_id) FILTER (WHERE out AND peer_read), 0) AS read_outbox
FROM (
    SELECT r.message_id, r.out, r.unread,
           CASE WHEN m.from_user_id = $1 THEN m.peer_user_id ELSE m.from_user_id END AS peer,
           (pr.global_id IS NOT NULL AND NOT pr.unread) AS peer_read
    FROM message_refs r
    JOIN messages m ON m.global_id = r.global_id
    LEFT JOIN message_refs pr ON pr.global_id = r.global_id AND NOT pr.out
         AND pr.user_id = (CASE WHEN m.from_user_id = $1 THEN m.peer_user_id ELSE m.from_user_id END)
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
		if err := rows.Scan(&d.PeerUserID, &d.TopMessageID, &d.UnreadCount, &d.ReadInboxMaxID, &d.ReadOutboxMaxID); err != nil {
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
// read, logging the caller's inbox-read event and, when any of the peer's
// messages were read, the peer's read-receipt.
func (db *DB) ReadHistory(ctx context.Context, self, peer, maxID int64) (teled.ReadResult, error) {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return teled.ReadResult{}, errors.Wrap(err, "begin")
	}

	defer func() { _ = tx.Rollback(ctx) }()

	// The peer's max outgoing local id among the messages being read, for the
	// read-receipt.
	var res teled.ReadResult
	if err := tx.QueryRow(ctx, `
SELECT COALESCE(MAX(pr.message_id), 0)
FROM message_refs r
JOIN messages m ON m.global_id = r.global_id
JOIN message_refs pr ON pr.global_id = r.global_id AND pr.user_id = $3 AND pr.out
WHERE r.user_id = $1 AND NOT r.out AND r.message_id <= $2
  AND (CASE WHEN m.from_user_id = $1 THEN m.peer_user_id ELSE m.from_user_id END) = $3`,
		self, maxID, peer,
	).Scan(&res.OutboxMaxID); err != nil {
		return teled.ReadResult{}, errors.Wrap(err, "outbox max")
	}

	if _, err := tx.Exec(ctx, `
UPDATE message_refs r SET unread = false
FROM messages m
WHERE r.global_id = m.global_id AND r.user_id = $1 AND NOT r.out AND r.message_id <= $2
  AND (CASE WHEN m.from_user_id = $1 THEN m.peer_user_id ELSE m.from_user_id END) = $3`,
		self, maxID, peer,
	); err != nil {
		return teled.ReadResult{}, errors.Wrap(err, "mark read")
	}

	if err := tx.QueryRow(ctx, `
SELECT count(*)
FROM message_refs r
JOIN messages m ON m.global_id = r.global_id
WHERE r.user_id = $1 AND NOT r.out AND r.unread
  AND (CASE WHEN m.from_user_id = $1 THEN m.peer_user_id ELSE m.from_user_id END) = $2`,
		self, peer,
	).Scan(&res.UnreadCount); err != nil {
		return teled.ReadResult{}, errors.Wrap(err, "remaining unread")
	}

	if res.InboxPts, err = allocatePts(ctx, tx, self, 1); err != nil {
		return teled.ReadResult{}, errors.Wrap(err, "allocate inbox")
	}

	if err := logUpdate(ctx, tx, self, res.InboxPts, 1, updReadInbox, nil, teled.EncodeRead(peer, maxID)); err != nil {
		return teled.ReadResult{}, errors.Wrap(err, "log read")
	}

	if res.OutboxMaxID > 0 && peer != self {
		if res.OutboxPts, err = allocatePts(ctx, tx, peer, 1); err != nil {
			return teled.ReadResult{}, errors.Wrap(err, "allocate outbox")
		}

		if err := logUpdate(ctx, tx, peer, res.OutboxPts, 1, updReadOutbox, nil, teled.EncodeRead(self, res.OutboxMaxID)); err != nil {
			return teled.ReadResult{}, errors.Wrap(err, "log receipt")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return teled.ReadResult{}, errors.Wrap(err, "commit")
	}

	return res, nil
}
