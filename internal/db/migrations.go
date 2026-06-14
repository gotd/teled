package db

import (
	"embed"
	"strings"

	"github.com/go-faster/errors"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // pgx5 migrate driver.
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed _migrations/*.sql
var migrations embed.FS

// Migrate applies all pending migrations to the database at uri.
func Migrate(uri string) error {
	src, err := iofs.New(migrations, "_migrations")
	if err != nil {
		return errors.Wrap(err, "open source")
	}

	// The pgx5 migrate driver is registered under the pgx5:// scheme.
	migrateURI := strings.Replace(uri, "postgres://", "pgx5://", 1)

	m, err := migrate.NewWithSourceInstance("iofs", src, migrateURI)
	if err != nil {
		return errors.Wrap(err, "new migrate")
	}

	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return errors.Wrap(err, "up")
	}

	return nil
}
