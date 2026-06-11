package db

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"strings"

	sq "github.com/Masterminds/squirrel"
	gerrors "github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"

	"github.com/gotd/teled"
)

// userColumns is the canonical select list; COALESCE keeps nullable text
// scannable into plain strings.
var userColumns = []string{
	"id",
	"access_hash",
	"COALESCE(phone, '')",
	"COALESCE(username, '')",
	"first_name",
	"last_name",
	"about",
	"created_at",
}

func scanUser(row pgx.Row, u *teled.User) error {
	return row.Scan(
		&u.ID, &u.AccessHash, &u.Phone, &u.Username,
		&u.FirstName, &u.LastName, &u.About, &u.CreatedAt,
	)
}

// genAccessHash returns a random non-zero access hash.
func genAccessHash() int64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	h := int64(binary.LittleEndian.Uint64(b[:])) // #nosec G115 -- bit reinterpretation.
	if h == 0 {
		h = 1
	}
	return h
}

// CreateUser inserts a new user with the given phone and name.
func (db *DB) CreateUser(ctx context.Context, phone, firstName, lastName string) (teled.User, error) {
	q := psql.Insert("users").
		Columns("access_hash", "phone", "first_name", "last_name").
		Values(genAccessHash(), phone, firstName, lastName).
		Suffix("RETURNING " + strings.Join(userColumns, ", "))

	sql, args, err := q.ToSql()
	if err != nil {
		return teled.User{}, gerrors.Wrap(err, "build query")
	}

	var u teled.User
	if err := scanUser(db.pool.QueryRow(ctx, sql, args...), &u); err != nil {
		return teled.User{}, gerrors.Wrap(err, "insert")
	}
	return u, nil
}

func (db *DB) userBy(ctx context.Context, where string, arg any) (*teled.User, bool, error) {
	q := psql.Select(userColumns...).From("users").Where(where, arg)
	sql, args, err := q.ToSql()
	if err != nil {
		return nil, false, gerrors.Wrap(err, "build query")
	}

	var u teled.User
	if err := scanUser(db.pool.QueryRow(ctx, sql, args...), &u); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, gerrors.Wrap(err, "scan")
	}
	return &u, true, nil
}

// UserByID returns a user by id.
func (db *DB) UserByID(ctx context.Context, id int64) (*teled.User, bool, error) {
	return db.userBy(ctx, "id = ?", id)
}

// UserByPhone returns a user by phone number.
func (db *DB) UserByPhone(ctx context.Context, phone string) (*teled.User, bool, error) {
	return db.userBy(ctx, "phone = ?", phone)
}

// UserByUsername returns a user by username.
func (db *DB) UserByUsername(ctx context.Context, username string) (*teled.User, bool, error) {
	return db.userBy(ctx, "username = ?", username)
}

// UsersByIDs returns the users with the given ids, in arbitrary order.
func (db *DB) UsersByIDs(ctx context.Context, ids []int64) ([]teled.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	q := psql.Select(userColumns...).From("users").Where(sq.Eq{"id": ids})
	sql, args, err := q.ToSql()
	if err != nil {
		return nil, gerrors.Wrap(err, "build query")
	}

	rows, err := db.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, gerrors.Wrap(err, "query")
	}
	defer rows.Close()

	var users []teled.User
	for rows.Next() {
		var u teled.User
		if err := scanUser(rows, &u); err != nil {
			return nil, gerrors.Wrap(err, "scan")
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, gerrors.Wrap(err, "rows")
	}
	return users, nil
}
