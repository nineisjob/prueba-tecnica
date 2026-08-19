package auction

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeStore is an in-memory bidStore implementing the same conditional-write
// semantics as postgres.ApplyBid (Layer 3), used to test the room actor
// (Layer 1+2) in isolation from a real database.
//
// It deliberately holds its own mutex and sleeps ~50us inside ApplyBid to
// widen the race window: a fake that responds instantly would hide bugs
// that only manifest when a bid is genuinely in flight while another
// arrives or the auction expires.
type fakeStore struct {
	mu      sync.Mutex
	auction domain.Auction
	bids    []domain.Bid
	nextSeq int64
	sleep   time.Duration
	closed  bool
	closeN  int
}

func newFakeStore(a domain.Auction) *fakeStore {
	return &fakeStore{auction: a, sleep: 50 * time.Microsecond}
}

func (s *fakeStore) ApplyBid(ctx context.Context, p domain.ApplyBidParams) (domain.ApplyBidResult, error) {
	if s.sleep > 0 {
		time.Sleep(s.sleep)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.auction.Status != domain.StatusActive {
		return domain.ApplyBidResult{}, domain.ErrAuctionEnded
	}
	minNext := s.auction.MinNextBidCents()
	if p.AmountCents < minNext {
		return domain.ApplyBidResult{}, domain.ErrBidTooLow
	}
	for _, b := range s.bids {
		if b.AmountCents == p.AmountCents {
			return domain.ApplyBidResult{}, domain.ErrDuplicateAmount
		}
	}

	s.nextSeq++
	bidderID := p.BidderID
	bid := domain.Bid{
		ID:          uuid.New(),
		Seq:         s.nextSeq,
		AuctionID:   p.AuctionID,
		BidderID:    bidderID,
		BidderName:  p.BidderName,
		AmountCents: p.AmountCents,
		PlacedAt:    p.PlacedAt,
	}
	s.bids = append(s.bids, bid)

	s.auction.CurrentPriceCents = p.AmountCents
	s.auction.CurrentWinnerID = &bidderID
	s.auction.BidCount++

	return domain.ApplyBidResult{Auction: s.auction, Bid: bid}, nil
}

func (s *fakeStore) Activate(ctx context.Context, id uuid.UUID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auction.Status = domain.StatusActive
	return nil
}

func (s *fakeStore) Close(ctx context.Context, id uuid.UUID, at time.Time) (domain.Auction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeN++
	s.closed = true
	s.auction.Status = domain.StatusFinished
	s.auction.ClosedAt = &at
	return s.auction, nil
}

func (s *fakeStore) snapshot() domain.Auction {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.auction
}

func (s *fakeStore) acceptedBids() []domain.Bid {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Bid, len(s.bids))
	copy(out, s.bids)
	return out
}

func (s *fakeStore) closeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeN
}

// fakePublisher records every published event for assertions. Publish must
// never block (the EventPublisher contract), so the channel is generously
// buffered for tests.
type fakePublisher struct {
	mu     sync.Mutex
	events []domain.Event
}

func newFakePublisher() *fakePublisher {
	return &fakePublisher{}
}

func (p *fakePublisher) Publish(ev domain.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, ev)
}

func (p *fakePublisher) all() []domain.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]domain.Event, len(p.events))
	copy(out, p.events)
	return out
}

func (p *fakePublisher) countType(t domain.EventType) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, ev := range p.events {
		if ev.Type == t {
			n++
		}
	}
	return n
}

// newTestAuction builds an already-active auction whose window lasts `dur`
// starting now, suitable for exercising expiry timing precisely.
func newTestAuction(dur time.Duration) domain.Auction {
	now := time.Now().UTC()
	return domain.Auction{
		ID:                uuid.New(),
		SellerID:          uuid.New(),
		Title:             "Test Auction",
		ImageURL:          "https://example.com/a.png",
		BasePriceCents:    1000,
		CurrentPriceCents: 1000,
		MinIncrementCents: 100,
		BidCount:          0,
		Status:            domain.StatusActive,
		StartsAt:          now.Add(-time.Second),
		EndsAt:            now.Add(dur),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}
