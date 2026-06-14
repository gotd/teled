package db

import (
	"context"
	"strings"
	"time"

	gerrors "github.com/go-faster/errors"

	"github.com/gotd/teled"
)

// SaveDraft stores (or, when text is blank, clears) the caller's draft for a
// peer and returns its date.
func (db *DB) SaveDraft(ctx context.Context, userID, peerID int64, text string) (time.Time, error) {
	if strings.TrimSpace(text) == "" {
		if _, err := db.pool.Exec(ctx,
			`DELETE FROM drafts WHERE user_id = $1 AND peer_user_id = $2`, userID, peerID,
		); err != nil {
			return time.Time{}, gerrors.Wrap(err, "delete draft")
		}

		return time.Time{}, nil
	}

	var date time.Time
	if err := db.pool.QueryRow(ctx,
		`INSERT INTO drafts (user_id, peer_user_id, message) VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, peer_user_id) DO UPDATE SET message = EXCLUDED.message, date = now()
		 RETURNING date`,
		userID, peerID, text,
	).Scan(&date); err != nil {
		return time.Time{}, gerrors.Wrap(err, "save draft")
	}

	return date, nil
}

// Drafts returns all of the caller's saved drafts, newest first.
func (db *DB) Drafts(ctx context.Context, userID int64) ([]teled.Draft, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT peer_user_id, message, date FROM drafts WHERE user_id = $1 ORDER BY date DESC`, userID,
	)
	if err != nil {
		return nil, gerrors.Wrap(err, "query")
	}
	defer rows.Close()

	var drafts []teled.Draft

	for rows.Next() {
		var d teled.Draft
		if err := rows.Scan(&d.PeerUserID, &d.Text, &d.Date); err != nil {
			return nil, gerrors.Wrap(err, "scan")
		}

		drafts = append(drafts, d)
	}

	if err := rows.Err(); err != nil {
		return nil, gerrors.Wrap(err, "rows")
	}

	return drafts, nil
}
