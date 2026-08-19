// Package seed inserts demo data for the video demo and local development.
// It is NOT a migration: seeding is idempotent (ON CONFLICT DO NOTHING) and
// environment-dependent (SEED_DEMO_DATA=true), whereas migrations are
// unconditional schema changes. Mixing the two would make migrations
// non-idempotent across environments and pollute the schema history.
package seed

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

// DemoUser is (username, email, plaintext password). The plaintext lives
// here -- and only here, in code the reviewer can read -- rather than as a
// pre-computed hash constant, so the demo credentials are self-documenting.
type DemoUser struct {
	Email    string
	Username string
	Password string
}

var DemoUsers = []DemoUser{
	{Email: "alice@bidcraft.dev", Username: "alice", Password: "demo1234"},
	{Email: "bob@bidcraft.dev", Username: "bob", Password: "demo1234"},
	{Email: "carol@bidcraft.dev", Username: "carol", Password: "demo1234"},
}

func Run(ctx context.Context, pool *pgxpool.Pool, hasher domain.PasswordHasher, log *slog.Logger) error {
	userIDs := make(map[string]string, len(DemoUsers))

	for _, du := range DemoUsers {
		hash, err := hasher.Hash(du.Password)
		if err != nil {
			return err
		}
		var id string
		err = pool.QueryRow(ctx, `
			INSERT INTO users (email, username, password_hash)
			VALUES ($1, $2, $3)
			ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email
			RETURNING id`, du.Email, du.Username, hash,
		).Scan(&id)
		if err != nil {
			return err
		}
		userIDs[du.Username] = id
	}

	if err := seedAuction(ctx, pool, userIDs["alice"], "", "Chromatic Drift #12", "Generative art, 4K, animated gradient field.", "https://picsum.photos/seed/bidcraft1/800/600", 5000, 250, domain.StatusActive, -time.Minute, 20*time.Minute); err != nil {
		return err
	}
	if err := seedAuction(ctx, pool, userIDs["bob"], "", "Neon Ruins", "Cyberpunk cityscape, hand-inked digital painting.", "https://picsum.photos/seed/bidcraft2/800/600", 8000, 500, domain.StatusActive, -time.Second, 90*time.Second); err != nil {
		return err
	}
	if err := seedAuction(ctx, pool, userIDs["carol"], "", "Quiet Orbit", "Minimalist planetary study, limited edition.", "https://picsum.photos/seed/bidcraft3/800/600", 3000, 100, domain.StatusCreated, 5*time.Minute, 25*time.Minute); err != nil {
		return err
	}
	if err := seedAuction(ctx, pool, userIDs["alice"], userIDs["bob"], "Faded Signal", "Glitch-art series, piece 3 of 7.", "https://picsum.photos/seed/bidcraft4/800/600", 4500, 200, domain.StatusFinished, -2*time.Hour, -time.Hour); err != nil {
		return err
	}

	log.Info("seed complete", "users", len(DemoUsers))
	return nil
}

func seedAuction(ctx context.Context, pool *pgxpool.Pool, sellerID, winnerID, title, desc, imageURL string, baseCents, incrementCents int64, status domain.Status, startOffset, endOffset time.Duration) error {
	now := time.Now().UTC()
	starts := now.Add(startOffset)
	ends := now.Add(endOffset)

	var closedAt *time.Time
	currentPrice := baseCents
	bidCount := 0
	var winner *string
	if winnerID != "" {
		t := ends
		closedAt = &t
		currentPrice = baseCents + incrementCents*3
		bidCount = 3
		winner = &winnerID
	}

	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auctions WHERE title = $1)`, title).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO auctions (
			seller_id, title, description, image_url,
			base_price_cents, current_price_cents, min_increment_cents,
			current_winner_id, bid_count, status, starts_at, ends_at, closed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		sellerID, title, desc, imageURL,
		baseCents, currentPrice, incrementCents,
		winner, bidCount, status, starts, ends, closedAt,
	)
	return err
}
