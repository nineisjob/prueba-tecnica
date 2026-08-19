package auction

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

// fakeAuctionRepo is a minimal domain.AuctionRepository for exercising the
// Manager without a real database.
type fakeAuctionRepo struct {
	mu    sync.Mutex
	byID  map[uuid.UUID]domain.Auction
	store map[uuid.UUID]*fakeStore
}

func newFakeAuctionRepo() *fakeAuctionRepo {
	return &fakeAuctionRepo{byID: map[uuid.UUID]domain.Auction{}, store: map[uuid.UUID]*fakeStore{}}
}

func (r *fakeAuctionRepo) put(a domain.Auction) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[a.ID] = a
}

func (r *fakeAuctionRepo) Create(ctx context.Context, a *domain.Auction) error { r.put(*a); return nil }

func (r *fakeAuctionRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Auction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrAuctionNotFound
	}
	return &a, nil
}

func (r *fakeAuctionRepo) List(ctx context.Context, f domain.AuctionFilter) ([]domain.Auction, error) {
	return nil, nil
}

func (r *fakeAuctionRepo) ListOpen(ctx context.Context) ([]domain.Auction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Auction
	for _, a := range r.byID {
		if a.Status != domain.StatusFinished {
			out = append(out, a)
		}
	}
	return out, nil
}

func (r *fakeAuctionRepo) Activate(ctx context.Context, id uuid.UUID, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.byID[id]
	a.Status = domain.StatusActive
	r.byID[id] = a
	return nil
}

func (r *fakeAuctionRepo) ApplyBid(ctx context.Context, p domain.ApplyBidParams) (domain.ApplyBidResult, error) {
	r.mu.Lock()
	a := r.byID[p.AuctionID]
	r.mu.Unlock()
	if a.Status != domain.StatusActive {
		return domain.ApplyBidResult{}, domain.ErrAuctionEnded
	}
	if p.AmountCents < a.MinNextBidCents() {
		return domain.ApplyBidResult{}, domain.ErrBidTooLow
	}
	bidderID := p.BidderID
	a.CurrentPriceCents = p.AmountCents
	a.CurrentWinnerID = &bidderID
	a.BidCount++
	r.mu.Lock()
	r.byID[p.AuctionID] = a
	r.mu.Unlock()
	return domain.ApplyBidResult{
		Auction: a,
		Bid: domain.Bid{
			ID: uuid.New(), Seq: int64(a.BidCount), AuctionID: p.AuctionID,
			BidderID: p.BidderID, BidderName: p.BidderName, AmountCents: p.AmountCents, PlacedAt: p.PlacedAt,
		},
	}, nil
}

func (r *fakeAuctionRepo) Close(ctx context.Context, id uuid.UUID, at time.Time) (domain.Auction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.byID[id]
	a.Status = domain.StatusFinished
	a.ClosedAt = &at
	r.byID[id] = a
	return a, nil
}

// TestManager_BidDuringRoomShutdown spams PlaceBid calls while auctions are
// closing (rooms exiting and being removed from the registry) to prove the
// room.done liveness guard prevents the lookup/close race from deadlocking
// or blocking forever. The only assertion is: no deadlock, no panic,
// -race clean, and every call returns within a bounded time.
func TestManager_BidDuringRoomShutdown(t *testing.T) {
	repo := newFakeAuctionRepo()
	pub := newFakePublisher()
	mgr := NewManager(repo, pub, domain.RealClock{}, testLogger())
	defer mgr.Shutdown(context.Background())

	const numAuctions = 20
	var auctionIDs []uuid.UUID
	for i := 0; i < numAuctions; i++ {
		a := newTestAuction(30 * time.Millisecond) // short-lived: rooms will exit quickly
		repo.put(a)
		mgr.Spawn(a)
		auctionIDs = append(auctionIDs, a.ID)
	}

	var wg sync.WaitGroup
	deadline := time.Now().Add(300 * time.Millisecond)
	for w := 0; w < 30; w++ {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			i := 0
			for time.Now().Before(deadline) {
				id := auctionIDs[(w+i)%numAuctions]
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				_, _, _ = mgr.PlaceBid(ctx, id, uuid.New(), "bidder", 1000+int64(i))
				cancel()
				i++
			}
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("PlaceBid calls appear stuck during room shutdown -- possible lookup/close race deadlock")
	}
}

// TestManager_Recover verifies that an auction whose end time already
// passed while the process was "down" is closed immediately without
// spawning a room, while one still within its window is spawned and armed.
func TestManager_Recover(t *testing.T) {
	repo := newFakeAuctionRepo()
	pub := newFakePublisher()
	mgr := NewManager(repo, pub, domain.RealClock{}, testLogger())
	defer mgr.Shutdown(context.Background())

	now := time.Now().UTC()

	expired := newTestAuction(time.Second)
	expired.EndsAt = now.Add(-time.Hour)
	expired.StartsAt = now.Add(-2 * time.Hour)
	repo.put(expired)

	live := newTestAuction(time.Hour)
	repo.put(live)

	if err := mgr.Recover(context.Background()); err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	got, err := repo.GetByID(context.Background(), expired.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusFinished {
		t.Fatalf("expired auction should be closed by Recover, got status=%s", got.Status)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err = mgr.PlaceBid(ctx, live.ID, uuid.New(), "bidder", live.BasePriceCents)
	if err != nil {
		t.Fatalf("expected the live auction's room to be spawned and accept a bid, got: %v", err)
	}
}
