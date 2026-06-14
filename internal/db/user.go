package db

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"strconv"
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
	"is_bot",
	"COALESCE(bot_token, '')",
	"created_at",
}

func scanUser(row pgx.Row, u *teled.User) error {
	return row.Scan(
		&u.ID, &u.AccessHash, &u.Phone, &u.Username,
		&u.FirstName, &u.LastName, &u.About, &u.IsBot, &u.BotToken, &u.CreatedAt,
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

// CreateBot inserts a new bot account authenticated by token. username may be
// empty.
func (db *DB) CreateBot(ctx context.Context, token, username, firstName string) (teled.User, error) {
	cols := []string{"access_hash", "bot_token", "first_name", "is_bot"}
	vals := []any{genAccessHash(), token, firstName, true}

	if username != "" {
		cols = append(cols, "username")
		vals = append(vals, username)
	}

	q := psql.Insert("users").
		Columns(cols...).
		Values(vals...).
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

// BotByToken returns the bot account holding token, if any.
func (db *DB) BotByToken(ctx context.Context, token string) (*teled.User, bool, error) {
	return db.userBy(ctx, "bot_token = ?", token)
}

// CreateOwnedBot creates a bot owned by ownerID, mints its token as
// "<bot_id>:<secret>" once the id is known, and returns the persisted account
// (with BotToken populated).
func (db *DB) CreateOwnedBot(ctx context.Context, username, firstName string, ownerID int64, secret string) (teled.User, error) {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return teled.User{}, gerrors.Wrap(err, "begin")
	}

	defer func() { _ = tx.Rollback(ctx) }()

	var id int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO users (access_hash, username, first_name, is_bot, bot_owner_id)
		 VALUES ($1, $2, $3, true, $4) RETURNING id`,
		genAccessHash(), username, firstName, ownerID,
	).Scan(&id); err != nil {
		return teled.User{}, gerrors.Wrap(err, "insert")
	}

	token := strconv.FormatInt(id, 10) + ":" + secret

	var u teled.User

	if err := scanUser(tx.QueryRow(ctx,
		`UPDATE users SET bot_token = $1 WHERE id = $2 RETURNING `+strings.Join(userColumns, ", "),
		token, id,
	), &u); err != nil {
		return teled.User{}, gerrors.Wrap(err, "set token")
	}

	if err := tx.Commit(ctx); err != nil {
		return teled.User{}, gerrors.Wrap(err, "commit")
	}

	return u, nil
}

// SetBotToken replaces a bot's auth token, e.g. on /revoke. A missing bot is a
// no-op.
func (db *DB) SetBotToken(ctx context.Context, botID int64, token string) error {
	if _, err := db.pool.Exec(ctx,
		`UPDATE users SET bot_token = $1 WHERE id = $2 AND is_bot`, token, botID,
	); err != nil {
		return gerrors.Wrap(err, "update")
	}

	return nil
}

// BotsByOwner returns the bots created by ownerID, oldest first.
func (db *DB) BotsByOwner(ctx context.Context, ownerID int64) ([]teled.User, error) {
	q := psql.Select(userColumns...).From("users").
		Where("bot_owner_id = ?", ownerID).OrderBy("id")

	sql, args, err := q.ToSql()
	if err != nil {
		return nil, gerrors.Wrap(err, "build query")
	}

	rows, err := db.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, gerrors.Wrap(err, "query")
	}
	defer rows.Close()

	var bots []teled.User

	for rows.Next() {
		var u teled.User
		if err := scanUser(rows, &u); err != nil {
			return nil, gerrors.Wrap(err, "scan")
		}

		bots = append(bots, u)
	}

	if err := rows.Err(); err != nil {
		return nil, gerrors.Wrap(err, "rows")
	}

	return bots, nil
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

// SearchUsers returns users matching query by username prefix or name
// substring, case-insensitively, ordered by id, up to limit.
func (db *DB) SearchUsers(ctx context.Context, query string, limit int) ([]teled.User, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	if limit <= 0 {
		limit = 20
	}

	sub := "%" + query + "%"
	q := psql.Select(userColumns...).From("users").
		Where("username ILIKE ? OR first_name ILIKE ? OR last_name ILIKE ?", query+"%", sub, sub).
		OrderBy("id").Limit(uint64(limit)) // #nosec G115 -- limit is bounded above.

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

// SetUsername sets (or clears, when empty) a user's username. An empty username
// is stored as NULL so it does not collide under the UNIQUE constraint.
func (db *DB) SetUsername(ctx context.Context, userID int64, username string) (teled.User, error) {
	var arg any
	if username != "" {
		arg = username
	}

	var u teled.User
	if err := scanUser(db.pool.QueryRow(ctx,
		`UPDATE users SET username = $1 WHERE id = $2 RETURNING `+strings.Join(userColumns, ", "),
		arg, userID,
	), &u); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return teled.User{}, gerrors.Errorf("user %d not found", userID)
		}

		return teled.User{}, gerrors.Wrap(err, "update")
	}

	return u, nil
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
