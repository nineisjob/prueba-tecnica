//go:build integration

package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/geferson/bidcraft/backend/internal/platform/db"
	"github.com/geferson/bidcraft/backend/migrations"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		panic("TEST_DATABASE_URL must be set to run integration tests (see docker-compose.yml's db-test service)")
	}

	if err := db.Migrate(dsn, migrations.FS); err != nil {
		panic("migrate test database: " + err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := db.NewPool(ctx, dsn, 25)
	if err != nil {
		panic("connect to test database: " + err.Error())
	}
	testPool = pool
	defer pool.Close()

	os.Exit(m.Run())
}

// truncateAll resets every table between tests so each test starts from a
// known-empty state without needing a fresh database per test.
func truncateAll(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `TRUNCATE TABLE bids, auctions, users RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate failed: %v", err)
	}
}
