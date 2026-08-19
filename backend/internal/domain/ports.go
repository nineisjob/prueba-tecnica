package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AuctionRepository is the full persistence boundary for auctions. Handlers
// and the auction manager depend on this interface, never on *postgres.Repo
// directly (DIP).
type AuctionRepository interface {
	Create(ctx context.Context, a *Auction) error
	GetByID(ctx context.Context, id uuid.UUID) (*Auction, error)
	List(ctx context.Context, f AuctionFilter) ([]Auction, error)
	// ListOpen returns auctions with status IN ('created', 'active'), used
	// by Manager.Recover to re-arm room actors on process restart.
	ListOpen(ctx context.Context) ([]Auction, error)
	Activate(ctx context.Context, id uuid.UUID, at time.Time) error
	ApplyBid(ctx context.Context, p ApplyBidParams) (ApplyBidResult, error)
	Close(ctx context.Context, id uuid.UUID, at time.Time) (Auction, error)
}

type BidRepository interface {
	ListByAuction(ctx context.Context, auctionID uuid.UUID, limit int) ([]Bid, error)
}

type UserRepository interface {
	Create(ctx context.Context, u *User) error
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
}

// Clock exposes only Now(). A full fake-timer scheduler would make expiry
// tests deterministic but costs a scheduler abstraction and a class of
// "the test passes because the fake is wrong" bugs -- KISS wins here.
// Concurrency tests use real timers with short (100-300ms) auctions instead.
type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

// PasswordHasher and TokenIssuer/TokenVerifier are the auth service's DIP
// boundaries, implemented by platform/hash and platform/jwt respectively.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}

type TokenIssuer interface {
	Issue(u AuthUser) (token string, expiresAt time.Time, err error)
}

type TokenVerifier interface {
	Verify(token string) (AuthUser, error)
}
