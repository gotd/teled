package db

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Open creates a new connection pool to the given PostgreSQL DSN.
func Open(ctx context.Context, uri string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(uri)
	if err != nil {
		return nil, errors.Wrap(err, "parse config")
	}
	cfg.MaxConns = 20
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 2 * time.Minute

	// Trace every query and record query metrics.
	cfg.ConnConfig.Tracer = queryTracer{}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.Wrap(err, "new pool")
	}
	return pool, nil
}
