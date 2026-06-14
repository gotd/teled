package db

import (
	"context"
	"errors"

	gerrors "github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"

	"github.com/gotd/teled"
)

// BotFatherState returns the user's pending BotFather flow. A zero-value state
// (empty Step) and no error means no flow is in progress.
func (db *DB) BotFatherState(ctx context.Context, userID int64) (teled.BotFatherState, error) {
	var s teled.BotFatherState

	err := db.pool.QueryRow(ctx,
		`SELECT step, draft_name FROM botfather_sessions WHERE user_id = $1`, userID,
	).Scan(&s.Step, &s.DraftName)
	if errors.Is(err, pgx.ErrNoRows) {
		return teled.BotFatherState{}, nil
	}

	if err != nil {
		return teled.BotFatherState{}, gerrors.Wrap(err, "query")
	}

	return s, nil
}

// SetBotFatherState upserts the user's pending BotFather flow.
func (db *DB) SetBotFatherState(ctx context.Context, userID int64, s teled.BotFatherState) error {
	if _, err := db.pool.Exec(ctx,
		`INSERT INTO botfather_sessions (user_id, step, draft_name) VALUES ($1, $2, $3)
		 ON CONFLICT (user_id) DO UPDATE SET step = EXCLUDED.step, draft_name = EXCLUDED.draft_name`,
		userID, s.Step, s.DraftName,
	); err != nil {
		return gerrors.Wrap(err, "upsert")
	}

	return nil
}

// ClearBotFatherState ends any pending BotFather flow for the user.
func (db *DB) ClearBotFatherState(ctx context.Context, userID int64) error {
	if _, err := db.pool.Exec(ctx, `DELETE FROM botfather_sessions WHERE user_id = $1`, userID); err != nil {
		return gerrors.Wrap(err, "delete")
	}

	return nil
}
