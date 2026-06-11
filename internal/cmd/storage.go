package cmd

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/riverqueue/river"
	"go.uber.org/zap"

	"github.com/gotd/teled/internal/db"
	"github.com/gotd/teled/internal/mtproto"
	"github.com/gotd/teled/internal/queue"
)

// setupStorage initializes persistence and returns the auth-key store plus a
// cleanup function. With no --postgres-uri it falls back to in-memory keys so
// the server is runnable standalone (keys are then lost on restart).
func (a *application) setupStorage(ctx context.Context) (mtproto.KeyStorage, *db.DB, func(), error) {
	if a.PostgresURI == "" {
		a.lg.Warn("No --postgres-uri set; auth keys are kept in memory and lost on restart")
		return mtproto.NewInMemoryKeys(), nil, func() {}, nil
	}

	if err := db.Migrate(a.PostgresURI); err != nil {
		return nil, nil, nil, errors.Wrap(err, "migrate")
	}

	pool, err := db.Open(ctx, a.PostgresURI)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "open database")
	}

	if err := queue.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, nil, nil, errors.Wrap(err, "migrate queue")
	}

	q, err := queue.New(pool, river.NewWorkers())
	if err != nil {
		pool.Close()
		return nil, nil, nil, errors.Wrap(err, "new queue")
	}
	if err := q.Start(ctx); err != nil {
		pool.Close()
		return nil, nil, nil, errors.Wrap(err, "start queue")
	}

	cleanup := func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := q.Stop(stopCtx); err != nil {
			a.lg.Warn("Stop queue", zap.Error(err))
		}
		pool.Close()
	}

	a.lg.Info("Connected to PostgreSQL")
	return db.NewKeyStore(pool), db.New(pool), cleanup, nil
}
