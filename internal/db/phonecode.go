package db

import (
	"context"
	"errors"
	"time"

	gerrors "github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"
)

// SavePhoneCode stores (or replaces) a login code for phone under codeHash,
// valid for ttl.
func (db *DB) SavePhoneCode(ctx context.Context, phone, codeHash, code string, ttl time.Duration) error {
	q := psql.Insert("phone_codes").
		Columns("phone", "code_hash", "code", "expires_at").
		Values(phone, codeHash, code, time.Now().Add(ttl)).
		Suffix("ON CONFLICT (phone, code_hash) DO UPDATE SET code = EXCLUDED.code, expires_at = EXCLUDED.expires_at")

	sql, args, err := q.ToSql()
	if err != nil {
		return gerrors.Wrap(err, "build query")
	}

	if _, err := db.pool.Exec(ctx, sql, args...); err != nil {
		return gerrors.Wrap(err, "exec")
	}

	return nil
}

// PhoneCode returns the unexpired code stored for (phone, codeHash).
func (db *DB) PhoneCode(ctx context.Context, phone, codeHash string) (code string, ok bool, err error) {
	q := psql.Select("code").From("phone_codes").
		Where("phone = ? AND code_hash = ? AND expires_at > now()", phone, codeHash)

	sql, args, err := q.ToSql()
	if err != nil {
		return "", false, gerrors.Wrap(err, "build query")
	}

	if err := db.pool.QueryRow(ctx, sql, args...).Scan(&code); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}

		return "", false, gerrors.Wrap(err, "scan")
	}

	return code, true, nil
}
