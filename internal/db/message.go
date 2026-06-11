package db

import (
	"context"
	"strconv"
	"time"

	"github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"

	"github.com/gotd/teled"
)

// Update log types.
const (
	updNewMessage  = "new"
	updEditMessage = "edit"
	updDelete      = "delete"
	updReadInbox   = "readinbox"
)

// allocate bumps the per-account local message id and common pts in one step,
// returning the new values. Must run inside the send transaction.
func allocate(ctx context.Context, tx pgx.Tx, userID int64) (localID int64, pts int, err error) {
	err = tx.QueryRow(ctx,
		`UPDATE users SET last_message_id = last_message_id + 1, pts = pts + 1
		 WHERE id = $1 RETURNING last_message_id, pts`, userID,
	).Scan(&localID, &pts)
	return localID, pts, err
}

// allocatePts advances only the common pts by count, returning the new value.
func allocatePts(ctx context.Context, tx pgx.Tx, userID int64, count int) (pts int, err error) {
	err = tx.QueryRow(ctx,
		`UPDATE users SET pts = pts + $2 WHERE id = $1 RETURNING pts`, userID, count,
	).Scan(&pts)
	return pts, err
}

// logUpdate appends an entry to the per-account update log.
func logUpdate(ctx context.Context, tx pgx.Tx, userID int64, pts, ptsCount int, typ string, globalID *int64, extra []byte) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO updates_log (user_id, pts, pts_count, type, global_id, extra)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, pts, ptsCount, typ, globalID, extra)
	return err
}

// SendMessage persists a DM atomically: the canonical message plus a per-account
// ref (with its own local id and pts) for sender and recipient.
func (db *DB) SendMessage(ctx context.Context, fromID, peerID int64, text string, randomID int64) (teled.SentMessage, error) {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return teled.SentMessage{}, errors.Wrap(err, "begin")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var sent teled.SentMessage
	sent.SelfChat = fromID == peerID

	if err := tx.QueryRow(ctx,
		`INSERT INTO messages (from_user_id, peer_user_id, text, random_id)
		 VALUES ($1, $2, $3, $4) RETURNING global_id, date`,
		fromID, peerID, text, randomID,
	).Scan(&sent.GlobalID, &sent.Date); err != nil {
		return teled.SentMessage{}, errors.Wrap(err, "insert message")
	}

	sent.SenderLocalID, sent.SenderPts, err = allocate(ctx, tx, fromID)
	if err != nil {
		return teled.SentMessage{}, errors.Wrap(err, "allocate sender")
	}
	if err := insertRef(ctx, tx, fromID, sent.SenderLocalID, sent.GlobalID, true, false); err != nil {
		return teled.SentMessage{}, errors.Wrap(err, "sender ref")
	}
	if err := logUpdate(ctx, tx, fromID, sent.SenderPts, 1, updNewMessage, &sent.GlobalID, nil); err != nil {
		return teled.SentMessage{}, errors.Wrap(err, "log sender")
	}

	if !sent.SelfChat {
		sent.RecipientLocalID, sent.RecipientPts, err = allocate(ctx, tx, peerID)
		if err != nil {
			return teled.SentMessage{}, errors.Wrap(err, "allocate recipient")
		}
		if err := insertRef(ctx, tx, peerID, sent.RecipientLocalID, sent.GlobalID, false, true); err != nil {
			return teled.SentMessage{}, errors.Wrap(err, "recipient ref")
		}
		if err := logUpdate(ctx, tx, peerID, sent.RecipientPts, 1, updNewMessage, &sent.GlobalID, nil); err != nil {
			return teled.SentMessage{}, errors.Wrap(err, "log recipient")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return teled.SentMessage{}, errors.Wrap(err, "commit")
	}
	return sent, nil
}

func insertRef(ctx context.Context, tx pgx.Tx, userID, localID, globalID int64, out, unread bool) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO message_refs (user_id, message_id, global_id, out, unread)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, localID, globalID, out, unread)
	return err
}

// historyColumns / scan for viewer-relative messages.
const historySelect = `
SELECT r.message_id, r.out, m.global_id, m.from_user_id, m.text, m.date, m.edit_date, m.random_id,
       CASE WHEN m.from_user_id = $1 THEN m.peer_user_id ELSE m.from_user_id END AS other
FROM message_refs r
JOIN messages m ON m.global_id = r.global_id
WHERE r.user_id = $1 AND NOT m.deleted
  AND (CASE WHEN m.from_user_id = $1 THEN m.peer_user_id ELSE m.from_user_id END) = $2`

func scanMessage(rows pgx.Rows) (teled.Message, error) {
	var (
		m        teled.Message
		editDate *time.Time
	)
	if err := rows.Scan(
		&m.LocalID, &m.Out, &m.GlobalID, &m.FromUserID, &m.Text, &m.Date, &editDate, &m.RandomID, &m.PeerUserID,
	); err != nil {
		return teled.Message{}, err
	}
	if editDate != nil {
		m.EditDate = *editDate
	}
	return m, nil
}

// GetHistory returns up to limit messages between self and peer, newest first.
// When offsetID > 0 only messages with a smaller local id are returned.
func (db *DB) GetHistory(ctx context.Context, self, peer, offsetID int64, limit int) ([]teled.Message, error) {
	q := historySelect
	args := []any{self, peer}
	if offsetID > 0 {
		q += " AND r.message_id < $3"
		args = append(args, offsetID)
	}
	q += " ORDER BY r.message_id DESC LIMIT " + strconv.Itoa(limit)

	rows, err := db.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, errors.Wrap(err, "query")
	}
	defer rows.Close()

	var msgs []teled.Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, errors.Wrap(err, "scan")
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "rows")
	}
	return msgs, nil
}
