// Package db implements PostgreSQL-backed storage for teled.
package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gotd/teled"
)

// psql builds queries with PostgreSQL dollar placeholders.
var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

// DB is the PostgreSQL-backed storage implementation.
type DB struct {
	pool *pgxpool.Pool
}

var _ teled.DB = (*DB)(nil)

// New creates a DB over the given pool.
func New(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// Ready reports whether the database is reachable.
func (db *DB) Ready(ctx context.Context) error {
	return db.pool.Ping(ctx)
}
