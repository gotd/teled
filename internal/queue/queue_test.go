package queue_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/gotd/teled/internal/pgtest"
	"github.com/gotd/teled/internal/queue"
)

func TestQueueStartStop(t *testing.T) {
	ctx := context.Background()
	dsn := pgtest.New(t)

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	require.NoError(t, queue.Migrate(ctx, pool))

	q, err := queue.New(pool, river.NewWorkers())
	require.NoError(t, err)

	require.NoError(t, q.Start(ctx))
	require.NoError(t, q.Stop(ctx))
}
