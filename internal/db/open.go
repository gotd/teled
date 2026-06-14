package db

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gotd/teled/internal/obs"
)

// Open creates a new connection pool to the given PostgreSQL DSN. providers
// supplies the OpenTelemetry tracer and meter used to instrument queries.
func Open(ctx context.Context, uri string, providers obs.Providers) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(uri)
	if err != nil {
		return nil, errors.Wrap(err, "parse config")
	}

	cfg.MaxConns = 20
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 2 * time.Minute

	// Trace every query and record query metrics.
	cfg.ConnConfig.Tracer = newQueryTracer(providers)

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.Wrap(err, "new pool")
	}

	return pool, nil
}
