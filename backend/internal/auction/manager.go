package auction

import (
	"context"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

// Manager is the auction-room registry. Its sync.RWMutex protects ONLY the
// `rooms` map -- a plain lookup table with no event-ordering semantics
// ("was room X registered before room Y" is a question nobody asks). Every
// bid and every WebSocket connect reads it; only auction creation/closure
// writes it, so RWMutex expresses the access pattern precisely.
//
// Per-auction bid state is the opposite: the accept/reject decision and the
// expiry decision must observe a single total order of events, which is
// exactly what each Room's channel-fed select loop provides and a mutex
// alone cannot (a mutex serializes accesses but cannot serialize a *timer*
// against them without extra machinery). Hence the split: channels where
// order is the invariant, mutexes where only mutual exclusion is.
type Manager struct {
	mu    sync.RWMutex
	rooms map[uuid.UUID]*Room

	repo      domain.AuctionRepository
	publisher domain.EventPublisher
	clock     domain.Clock
	log       *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewManager(repo domain.AuctionRepository, pub domain.EventPublisher, clock domain.Clock, log *slog.Logger) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		rooms:     make(map[uuid.UUID]*Room),
		repo:      repo,
		publisher: pub,
		clock:     clock,
		log:       log,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Recover re-arms room actors for every auction left open (status IN
// (created, active)) when the process last stopped. Called from main.go
// BEFORE the HTTP listener starts, so no request can reach a
// not-yet-recovered auction.
//
// An auction whose end time already passed while the process was down is
// closed immediately without spawning a room. A negative time.Until yields
// an already-fired timer, so even a race between this query and Spawn is
// safe.
//
// Rejected as YAGNI: a periodic reaper ticker sweeping for stale auctions.
// Single replica + rooms owning their own timers + startup recovery covers
// every real case.
func (m *Manager) Recover(ctx context.Context) error {
	open, err := m.repo.ListOpen(ctx)
	if err != nil {
		return err
	}
	now := m.clock.Now()
	for _, a := range open {
		if !now.Before(a.EndsAt) {
			if _, err := m.repo.Close(ctx, a.ID, now); err != nil {
				m.log.Error("recover: close expired auction failed", "auction_id", a.ID, "err", err)
			}
			continue
		}
		m.Spawn(a)
	}
	return nil
}

// Spawn starts a room actor goroutine for the given auction and registers it.
func (m *Manager) Spawn(a domain.Auction) *Room {
	room := newRoom(a, m.repo, m.publisher, m.clock, m.log)

	m.mu.Lock()
	m.rooms[a.ID] = room
	m.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		room.run(m.ctx)
		m.mu.Lock()
		delete(m.rooms, a.ID)
		m.mu.Unlock()
	}()

	return room
}

// PlaceBid routes a bid to its room actor and waits for the reply. The
// room.done cases below guard the lookup/close race -- the classic bug in
// this pattern: without them, a bid routed to a room that just exited would
// block until the reply channel's buffer... except there's no writer left,
// so it would block forever.
func (m *Manager) PlaceBid(ctx context.Context, auctionID, bidderID uuid.UUID, bidderName string, amountCents int64) (domain.Bid, domain.Auction, error) {
	m.mu.RLock()
	room, ok := m.rooms[auctionID]
	m.mu.RUnlock()
	if !ok {
		return domain.Bid{}, domain.Auction{}, m.rejectFromDB(ctx, auctionID)
	}

	reply := make(chan bidOutcome, 1)
	select {
	case room.cmds <- placeBidCmd{BidderID: bidderID, BidderName: bidderName, Amount: amountCents, Reply: reply}:
	case <-room.done:
		return domain.Bid{}, domain.Auction{}, m.rejectFromDB(ctx, auctionID)
	case <-ctx.Done():
		return domain.Bid{}, domain.Auction{}, ctx.Err()
	default:
		return domain.Bid{}, domain.Auction{}, domain.ErrEngineBusy
	}

	select {
	case out := <-reply:
		return out.Bid, out.Auction, out.Err
	case <-room.done:
		return domain.Bid{}, domain.Auction{}, m.rejectFromDB(ctx, auctionID)
	case <-ctx.Done():
		return domain.Bid{}, domain.Auction{}, ctx.Err()
	}
}

// rejectFromDB classifies why a room isn't running (never existed / already
// finished) by falling back to the persisted auction state.
func (m *Manager) rejectFromDB(ctx context.Context, auctionID uuid.UUID) error {
	a, err := m.repo.GetByID(ctx, auctionID)
	if err != nil {
		return domain.ErrAuctionNotFound
	}
	if a.Status == domain.StatusFinished {
		return domain.ErrAuctionEnded
	}
	return domain.ErrAuctionNotStarted
}

// Snapshot returns the room's current in-memory state if it is running,
// falling back to a direct repository read otherwise (finished auctions).
func (m *Manager) Snapshot(ctx context.Context, auctionID uuid.UUID) (domain.Auction, error) {
	m.mu.RLock()
	room, ok := m.rooms[auctionID]
	m.mu.RUnlock()
	if !ok {
		a, err := m.repo.GetByID(ctx, auctionID)
		if err != nil {
			return domain.Auction{}, domain.ErrAuctionNotFound
		}
		return *a, nil
	}

	reply := make(chan domain.Auction, 1)
	select {
	case room.cmds <- snapshotCmd{Reply: reply}:
	case <-room.done:
		a, err := m.repo.GetByID(ctx, auctionID)
		if err != nil {
			return domain.Auction{}, domain.ErrAuctionNotFound
		}
		return *a, nil
	case <-ctx.Done():
		return domain.Auction{}, ctx.Err()
	}

	select {
	case a := <-reply:
		return a, nil
	case <-ctx.Done():
		return domain.Auction{}, ctx.Err()
	}
}

// Shutdown signals every room to stop and waits (bounded by ctx) for them to
// exit. Rooms leave DB state untouched on shutdown; Recover re-arms them on
// the next boot.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.cancel()
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
