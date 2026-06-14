package db

import (
	"context"
	"errors"

	gerrors "github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"

	"github.com/gotd/teled"
)

// EditMessage updates the text of a message the caller sent, returning the data
// needed to emit edit updates to both participants.
func (db *DB) EditMessage(ctx context.Context, self, localID int64, text string) (teled.EditResult, error) {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return teled.EditResult{}, gerrors.Wrap(err, "begin")
	}

	defer func() { _ = tx.Rollback(ctx) }()

	var globalID int64
	if err := tx.QueryRow(ctx,
		`SELECT global_id FROM message_refs WHERE user_id = $1 AND message_id = $2`,
		self, localID,
	).Scan(&globalID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return teled.EditResult{}, teled.ErrMessageID
		}

		return teled.EditResult{}, gerrors.Wrap(err, "find ref")
	}

	res := teled.EditResult{SelfLocalID: localID}
	if err := tx.QueryRow(ctx,
		`UPDATE messages SET text = $1, edit_date = now()
		 WHERE global_id = $2 AND from_user_id = $3 AND NOT deleted
		 RETURNING date, edit_date, peer_user_id`,
		text, globalID, self,
	).Scan(&res.Date, &res.EditDate, &res.PeerUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return teled.EditResult{}, teled.ErrMessageID // not the sender, or deleted
		}

		return teled.EditResult{}, gerrors.Wrap(err, "update message")
	}

	if res.SelfPts, err = allocatePts(ctx, tx, self, 1); err != nil {
		return teled.EditResult{}, gerrors.Wrap(err, "allocate self")
	}

	if err := logUpdate(ctx, tx, self, res.SelfPts, 1, updEditMessage, &globalID, nil); err != nil {
		return teled.EditResult{}, gerrors.Wrap(err, "log self")
	}

	if res.PeerUserID != self {
		_ = tx.QueryRow(ctx,
			`SELECT message_id FROM message_refs WHERE user_id = $1 AND global_id = $2`,
			res.PeerUserID, globalID,
		).Scan(&res.PeerLocalID)

		if res.PeerPts, err = allocatePts(ctx, tx, res.PeerUserID, 1); err != nil {
			return teled.EditResult{}, gerrors.Wrap(err, "allocate peer")
		}

		if err := logUpdate(ctx, tx, res.PeerUserID, res.PeerPts, 1, updEditMessage, &globalID, nil); err != nil {
			return teled.EditResult{}, gerrors.Wrap(err, "log peer")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return teled.EditResult{}, gerrors.Wrap(err, "commit")
	}

	return res, nil
}

// DeleteMessages marks the caller's messages (by local id) deleted and
// allocates a pts covering them.
func (db *DB) DeleteMessages(ctx context.Context, self int64, localIDs []int64) (teled.DeleteResult, error) {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return teled.DeleteResult{}, gerrors.Wrap(err, "begin")
	}

	defer func() { _ = tx.Rollback(ctx) }()

	var deleted []int64

	for _, localID := range localIDs {
		var globalID int64

		err := tx.QueryRow(ctx,
			`SELECT global_id FROM message_refs WHERE user_id = $1 AND message_id = $2`,
			self, localID,
		).Scan(&globalID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}

		if err != nil {
			return teled.DeleteResult{}, gerrors.Wrap(err, "find ref")
		}

		if _, err := tx.Exec(ctx, `UPDATE messages SET deleted = true WHERE global_id = $1`, globalID); err != nil {
			return teled.DeleteResult{}, gerrors.Wrap(err, "delete")
		}

		deleted = append(deleted, localID)
	}

	res := teled.DeleteResult{LocalIDs: deleted, PtsCount: len(deleted)}

	if len(deleted) > 0 {
		if res.Pts, err = allocatePts(ctx, tx, self, len(deleted)); err != nil {
			return teled.DeleteResult{}, gerrors.Wrap(err, "allocate")
		}

		extra := teled.EncodeDeleted(deleted)
		if err := logUpdate(ctx, tx, self, res.Pts, len(deleted), updDelete, nil, extra); err != nil {
			return teled.DeleteResult{}, gerrors.Wrap(err, "log delete")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return teled.DeleteResult{}, gerrors.Wrap(err, "commit")
	}

	return res, nil
}
