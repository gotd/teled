package db

import (
	"context"
	"net/url"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// newTestPool starts a throwaway PostgreSQL container, applies migrations and
// returns a connected pool. The container is terminated on test cleanup.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if runtime.GOOS != "linux" {
		t.Skip("Skipping container test on non-linux architecture")
	}

	ctx := context.Background()
	const (
		dbName     = "test_db"
		dbUser     = "test_user"
		dbPassword = "test_password"
	)

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_PASSWORD": dbPassword,
				"POSTGRES_USER":     dbUser,
				"POSTGRES_DB":       dbName,
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(time.Minute),
		},
		Started: true,
	}
	container, err := testcontainers.GenericContainer(ctx, req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	endpoint, err := container.PortEndpoint(ctx, "5432", "")
	require.NoError(t, err)

	dsn := (&url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(dbUser, dbPassword),
		Host:   endpoint,
		Path:   dbName,
	}).String()

	require.NoError(t, Migrate(dsn))

	pool, err := Open(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pool
}
