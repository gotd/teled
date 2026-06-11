// Package queue wires the River background job queue over PostgreSQL.
//
// River runs in-process workers against the same database, using
// SELECT ... FOR UPDATE SKIP LOCKED. Job kinds (media processing, cleanup,
// delivery retry) are registered here as later milestones add them.
package queue

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// Migrate applies River's own schema migrations to the database.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return errors.Wrap(err, "new migrator")
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return errors.Wrap(err, "migrate")
	}
	return nil
}

// Queue is the background job queue.
type Queue struct {
	client *river.Client[pgx.Tx]
}

// noopArgs is a placeholder job kind so the worker loop has at least one
// registered worker before real job kinds (media, cleanup) are added.
type noopArgs struct{}

// Kind implements river.JobArgs.
func (noopArgs) Kind() string { return "teled_noop" }

type noopWorker struct {
	river.WorkerDefaults[noopArgs]
}

func (noopWorker) Work(context.Context, *river.Job[noopArgs]) error { return nil }

// New creates a Queue over the given pool. Callers register their job workers on
// workers before calling New; an internal no-op worker is always registered so
// the client can start with an empty registry.
func New(pool *pgxpool.Pool, workers *river.Workers) (*Queue, error) {
	river.AddWorker(workers, &noopWorker{})

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
		Workers: workers,
	})
	if err != nil {
		return nil, errors.Wrap(err, "new client")
	}
	return &Queue{client: client}, nil
}

// Start begins processing jobs.
func (q *Queue) Start(ctx context.Context) error {
	return q.client.Start(ctx)
}

// Stop gracefully stops processing.
func (q *Queue) Stop(ctx context.Context) error {
	return q.client.Stop(ctx)
}
