//go:build integration

package postgres_test

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
	"github.com/geferson/bidcraft/backend/internal/repository/postgres"
)

func seedUser(t *testing.T, repo *postgres.UserRepo, username string) domain.User {
	t.Helper()
	u := domain.User{Email: username + "@test.dev", Username: username, PasswordHash: "x"}
	if err := repo.Create(context.Background(), &u); err != nil {
		t.Fatalf("seed user %s: %v", username, err)
	}
	return u
}

func seedActiveAuction(t *testing.T, repo *postgres.AuctionRepo, sellerID uuid.UUID, baseCents, incrementCents int64, dur time.Duration) domain.Auction {
	t.Helper()
	now := time.Now().UTC()
	a := domain.Auction{
		SellerID: sellerID, Title: "Integration Test Auction", ImageURL: "https://example.com/a.png",
		BasePriceCents: baseCents, CurrentPriceCents: baseCents, MinIncrementCents: incrementCents,
		Status: domain.StatusActive, StartsAt: now.Add(-time.Second), EndsAt: now.Add(dur),
	}
	if err := repo.Create(context.Background(), &a); err != nil {
		t.Fatalf("seed auction: %v", err)
	}
	return a
}

// TestApplyBid_ConcurrentDirect proves Layer 3 (the conditional UPDATE)
// stands on its own: it calls postgres.AuctionRepo.ApplyBid directly from
// 200 goroutines, bypassing the room actor (Layer 1/2) entirely. This is
// the whole justification for duplicating the bid rule in SQL -- if this
// test only passed with the actor's serialization in front of it, the
// "defense in depth" claim in the README would be false.
func TestApplyBid_ConcurrentDirect(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userRepo := postgres.NewUserRepo(testPool)
	auctionRepo := postgres.NewAuctionRepo(testPool)

	seller := seedUser(t, userRepo, "seller")
	auction := seedActiveAuction(t, auctionRepo, seller.ID, 1000, 100, time.Hour)

	const n = 200
	bidders := make([]domain.User, n)
	for i := range bidders {
		bidders[i] = seedUser(t, userRepo, uuid.NewString()[:8])
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]struct {
		amount int64
		err    error
	}, n)

	for i := 0; i < n; i++ {
		i := i
		amount := auction.BasePriceCents + int64(i)*auction.MinIncrementCents
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := auctionRepo.ApplyBid(ctx, domain.ApplyBidParams{
				AuctionID: auction.ID, BidderID: bidders[i].ID, BidderName: bidders[i].Username,
				AmountCents: amount, PlacedAt: time.Now().UTC(),
			})
			results[i] = struct {
				amount int64
				err    error
			}{amount, err}
		}()
	}
	close(start)
	wg.Wait()

	var accepted []int64
	for _, r := range results {
		if r.err == nil {
			accepted = append(accepted, r.amount)
		}
	}
	if len(accepted) == 0 {
		t.Fatal("expected at least one accepted bid")
	}
	sort.Slice(accepted, func(i, j int) bool { return accepted[i] < accepted[j] })
	for i := 1; i < len(accepted); i++ {
		if accepted[i] == accepted[i-1] {
			t.Fatalf("duplicate accepted amount %d -- the DB should have rejected the second one", accepted[i])
		}
	}

	final, err := auctionRepo.GetByID(ctx, auction.ID)
	if err != nil {
		t.Fatal(err)
	}
	maxAccepted := accepted[len(accepted)-1]
	if final.CurrentPriceCents != maxAccepted {
		t.Fatalf("final current_price_cents=%d != max accepted amount=%d", final.CurrentPriceCents, maxAccepted)
	}
	if final.BidCount != len(accepted) {
		t.Fatalf("final bid_count=%d != accepted count=%d", final.BidCount, len(accepted))
	}
}

// TestApplyBid_TieConcurrentDirect: every goroutine submits the IDENTICAL
// amount. Exactly one must be accepted, proving the unique-amount tripwire
// and the WHERE clause's price comparison both hold under real concurrent
// writes to Postgres -- not just against the in-memory fake used by the
// engine-level concurrency test.
func TestApplyBid_TieConcurrentDirect(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userRepo := postgres.NewUserRepo(testPool)
	auctionRepo := postgres.NewAuctionRepo(testPool)

	seller := seedUser(t, userRepo, "seller")
	auction := seedActiveAuction(t, auctionRepo, seller.ID, 1000, 100, time.Hour)

	const n = 100
	bidders := make([]domain.User, n)
	for i := range bidders {
		bidders[i] = seedUser(t, userRepo, uuid.NewString()[:8])
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0

	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := auctionRepo.ApplyBid(ctx, domain.ApplyBidParams{
				AuctionID: auction.ID, BidderID: bidders[i].ID, BidderName: bidders[i].Username,
				AmountCents: auction.BasePriceCents, PlacedAt: time.Now().UTC(),
			})
			if err == nil {
				mu.Lock()
				accepted++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if accepted != 1 {
		t.Fatalf("expected exactly 1 accepted bid among %d identical concurrent bids, got %d", n, accepted)
	}
}

func TestClose_Idempotent(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userRepo := postgres.NewUserRepo(testPool)
	auctionRepo := postgres.NewAuctionRepo(testPool)

	seller := seedUser(t, userRepo, "seller")
	auction := seedActiveAuction(t, auctionRepo, seller.ID, 1000, 100, time.Hour)

	now := time.Now().UTC()
	first, err := auctionRepo.Close(ctx, auction.ID, now)
	if err != nil {
		t.Fatalf("first close: %v", err)
	}
	if first.Status != domain.StatusFinished {
		t.Fatalf("expected finished, got %s", first.Status)
	}

	second, err := auctionRepo.Close(ctx, auction.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("second close should not error (idempotent), got: %v", err)
	}
	if second.ClosedAt == nil || !second.ClosedAt.Equal(*first.ClosedAt) {
		t.Fatalf("second close should return the ORIGINAL closed_at, not overwrite it: first=%v second=%v", first.ClosedAt, second.ClosedAt)
	}
}

func TestListOpen_ReturnsOnlyCreatedAndActive(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userRepo := postgres.NewUserRepo(testPool)
	auctionRepo := postgres.NewAuctionRepo(testPool)

	seller := seedUser(t, userRepo, "seller")
	active := seedActiveAuction(t, auctionRepo, seller.ID, 1000, 100, time.Hour)
	finished := seedActiveAuction(t, auctionRepo, seller.ID, 1000, 100, time.Hour)
	if _, err := auctionRepo.Close(ctx, finished.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	open, err := auctionRepo.ListOpen(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range open {
		if a.ID == finished.ID {
			t.Fatal("ListOpen must not return a finished auction")
		}
		if a.ID == active.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("ListOpen must return the active auction")
	}
}

func TestApplyBid_RejectsAfterExpiry(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userRepo := postgres.NewUserRepo(testPool)
	auctionRepo := postgres.NewAuctionRepo(testPool)

	seller := seedUser(t, userRepo, "seller")
	bidder := seedUser(t, userRepo, "bidder")
	// Already-expired window: ends_at in the past.
	now := time.Now().UTC()
	a := domain.Auction{
		SellerID: seller.ID, Title: "Expired", ImageURL: "https://example.com/a.png",
		BasePriceCents: 1000, CurrentPriceCents: 1000, MinIncrementCents: 100,
		Status: domain.StatusActive, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(-time.Minute),
	}
	if err := auctionRepo.Create(ctx, &a); err != nil {
		t.Fatal(err)
	}

	_, err := auctionRepo.ApplyBid(ctx, domain.ApplyBidParams{
		AuctionID: a.ID, BidderID: bidder.ID, BidderName: bidder.Username,
		AmountCents: 1000, PlacedAt: now,
	})
	if err == nil {
		t.Fatal("expected ApplyBid to reject a bid on an already-expired auction (the SQL guard's `now() < ends_at`)")
	}
}
