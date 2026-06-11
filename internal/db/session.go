package db

import (
	"context"
	"errors"

	gerrors "github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"
)

// BindSession binds an MTProto auth key to a logged-in user.
func (db *DB) BindSession(ctx context.Context, keyID [8]byte, userID int64) error {
	q := psql.Insert("sessions").
		Columns("key_id", "user_id").
		Values(keyID[:], userID).
		Suffix("ON CONFLICT (key_id) DO UPDATE SET user_id = EXCLUDED.user_id")

	sql, args, err := q.ToSql()
	if err != nil {
		return gerrors.Wrap(err, "build query")
	}
	if _, err := db.pool.Exec(ctx, sql, args...); err != nil {
		return gerrors.Wrap(err, "exec")
	}
	return nil
}

// SessionUserID returns the user bound to the given auth key, if any.
func (db *DB) SessionUserID(ctx context.Context, keyID [8]byte) (userID int64, ok bool, err error) {
	q := psql.Select("user_id").From("sessions").Where("key_id = ?", keyID[:])
	sql, args, err := q.ToSql()
	if err != nil {
		return 0, false, gerrors.Wrap(err, "build query")
	}

	if err := db.pool.QueryRow(ctx, sql, args...).Scan(&userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, gerrors.Wrap(err, "scan")
	}
	return userID, true, nil
}

// Unbind removes the user binding for an auth key (logout).
func (db *DB) Unbind(ctx context.Context, keyID [8]byte) error {
	q := psql.Delete("sessions").Where("key_id = ?", keyID[:])
	sql, args, err := q.ToSql()
	if err != nil {
		return gerrors.Wrap(err, "build query")
	}
	if _, err := db.pool.Exec(ctx, sql, args...); err != nil {
		return gerrors.Wrap(err, "exec")
	}
	return nil
}
