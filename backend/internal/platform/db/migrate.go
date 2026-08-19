package db

import (
	"errors"
	"io/fs"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrate applies every pending migration. golang-migrate takes a Postgres
// advisory lock internally, so concurrent replicas starting simultaneously
// cannot both migrate at once.
//
// Chosen over docker-entrypoint-initdb.d (only runs on an empty volume, so
// the second `docker compose up` would silently skip schema changes) and
// over a hand-rolled `IF NOT EXISTS` schema script (no version history, no
// down path, and the spec asks for "migraciones automáticas").
//
// dsn is the app's standard postgres:// URL; the pgx/v5 migrate driver
// requires the pgx5:// scheme, so it is rewritten here rather than forcing
// every caller (and every other use of DATABASE_URL) to know about it.
func Migrate(dsn string, migrationsFS fs.FS) error {
	migrateDSN := "pgx5://" + strings.TrimPrefix(strings.TrimPrefix(dsn, "postgres://"), "postgresql://")

	src, err := iofs.New(migrationsFS, ".")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, migrateDSN)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}
