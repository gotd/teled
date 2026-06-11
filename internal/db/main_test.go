package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/gotd/teled/internal/obs"
	"github.com/gotd/teled/internal/pgtest"
)

// newTestPool starts a throwaway PostgreSQL, applies migrations and returns a
// connected pool. The container is terminated on test cleanup.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := pgtest.New(t)
	require.NoError(t, Migrate(dsn))

	pool, err := Open(context.Background(), dsn, obs.Providers{})
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pool
}
