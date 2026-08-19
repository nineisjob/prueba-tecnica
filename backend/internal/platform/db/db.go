// Package db owns the pgx connection pool.
package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool opens a connection pool sized for the concurrency model: every
// active auction's room actor holds at most one connection at a time
// (serialized by the actor itself), so pool exhaustion under load comes
// from the NUMBER of concurrently active auctions and in-flight rejection
// reads, not from any single auction. maxConns=25 is a generous ceiling
// for a project of this scope -- see internal/repository/postgres's
// ApplyBid doc comment for why a rejection classification must reuse the
// same transaction's connection rather than acquiring a second one.
func NewPool(ctx context.Context, dsn string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
