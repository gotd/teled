package cmd

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/gotd/log"
	"github.com/gotd/teled/internal/db"
	"github.com/gotd/teled/internal/obs"
	"github.com/gotd/teled/internal/queue"
)

// setupStorage initializes persistence and returns the PostgreSQL pool plus a
// cleanup function. With no --postgres-uri it returns a nil pool so the server
// runs standalone with in-memory auth keys (lost on restart).
func (a *application) setupStorage(ctx context.Context, providers obs.Providers) (*pgxpool.Pool, func(), error) {
	if a.PostgresURI == "" {
		log.For(a.lg).Warn(ctx, "No --postgres-uri set; auth keys are kept in memory and lost on restart")
		return nil, func() {}, nil
	}

	if err := db.Migrate(a.PostgresURI); err != nil {
		return nil, nil, errors.Wrap(err, "migrate")
	}

	pool, err := db.Open(ctx, a.PostgresURI, providers)
	if err != nil {
		return nil, nil, errors.Wrap(err, "open database")
	}

	if err := queue.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, nil, errors.Wrap(err, "migrate queue")
	}

	q, err := queue.New(pool, river.NewWorkers())
	if err != nil {
		pool.Close()
		return nil, nil, errors.Wrap(err, "new queue")
	}

	if err := q.Start(ctx); err != nil {
		pool.Close()
		return nil, nil, errors.Wrap(err, "start queue")
	}

	cleanup := func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := q.Stop(stopCtx); err != nil {
			log.For(a.lg).Warn(stopCtx, "Stop queue", log.Error(err))
		}

		pool.Close()
	}

	log.For(a.lg).Info(ctx, "Connected to PostgreSQL")

	return pool, cleanup, nil
}
