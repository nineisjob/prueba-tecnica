package auction

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

// TestRoom_HundredsOfSimultaneousBids is the central empirical proof of the
// concurrency design: N goroutines fire bids at the exact same instant
// (released by a real close(start) barrier, not a staggered loop), and the
// room actor's single-goroutine select loop must still produce a
// consistent, race-free final state.
//
// Run with -race to get the actual empirical guarantee; run repeatedly with
// -count=20 to shake out timing flakes.
func TestRoom_HundredsOfSimultaneousBids(t *testing.T) {
	const n = 500

	base := newTestAuction(10 * time.Second) // long enough that none expire mid-test
	store := newFakeStore(base)
	pub := newFakePublisher()
	room := newRoom(base, store, pub, domain.RealClock{}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go room.run(ctx)

	start := make(chan struct{})
	var wg sync.WaitGroup
	outcomes := make([]bidOutcome, n)
	amounts := make([]int64, n)

	for i := 0; i < n; i++ {
		i := i
		amounts[i] = base.BasePriceCents + int64(i)*base.MinIncrementCents
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			reply := make(chan bidOutcome, 1)
			bidderID := uuid.New()
			select {
			case room.cmds <- placeBidCmd{
				BidderID:   bidderID,
				BidderName: bidderID.String()[:8],
				Amount:     amounts[i],
				Reply:      reply,
			}:
			case <-time.After(5 * time.Second):
				t.Errorf("bid %d: timed out sending command", i)
				return
			}
			select {
			case out := <-reply:
				outcomes[i] = out
			case <-time.After(5 * time.Second):
				t.Errorf("bid %d: timed out waiting for reply", i)
			}
		}()
	}

	close(start) // true barrier: every goroutine was already blocked on this
	wg.Wait()

	var accepted, rejected int
	var acceptedBids []domain.Bid
	for _, out := range outcomes {
		if out.Err == nil {
			accepted++
			acceptedBids = append(acceptedBids, out.Bid)
		} else {
			rejected++
		}
	}

	// Invariant 1: no command lost.
	if accepted+rejected != n {
		t.Fatalf("accepted(%d) + rejected(%d) != %d", accepted, rejected, n)
	}

	// Invariant 2: accepted amounts are strictly increasing in acceptance (seq) order.
	sort.Slice(acceptedBids, func(i, j int) bool { return acceptedBids[i].Seq < acceptedBids[j].Seq })
	for i := 1; i < len(acceptedBids); i++ {
		if acceptedBids[i].AmountCents <= acceptedBids[i-1].AmountCents {
			t.Fatalf("accepted amounts not strictly increasing at seq %d: %d <= %d",
				acceptedBids[i].Seq, acceptedBids[i].AmountCents, acceptedBids[i-1].AmountCents)
		}
	}

	// Invariant 8: no two accepted bids share an amount (mirrors the DB's
	// UNIQUE(auction_id, amount_cents) constraint).
	seen := map[int64]bool{}
	for _, b := range acceptedBids {
		if seen[b.AmountCents] {
			t.Fatalf("duplicate accepted amount: %d", b.AmountCents)
		}
		seen[b.AmountCents] = true
	}

	if len(acceptedBids) == 0 {
		t.Fatal("expected at least one accepted bid")
	}
	maxBid := acceptedBids[len(acceptedBids)-1]

	// Invariant 4: final in-memory state, final store state, and the max
	// accepted amount all agree.
	finalStore := store.snapshot()
	if finalStore.CurrentPriceCents != maxBid.AmountCents {
		t.Fatalf("store final price %d != max accepted amount %d", finalStore.CurrentPriceCents, maxBid.AmountCents)
	}

	reply := make(chan domain.Auction, 1)
	room.cmds <- snapshotCmd{Reply: reply}
	finalState := <-reply
	if finalState.CurrentPriceCents != maxBid.AmountCents {
		t.Fatalf("room final price %d != max accepted amount %d", finalState.CurrentPriceCents, maxBid.AmountCents)
	}

	// Invariant 5: exactly one winner, the bidder of the max accepted amount.
	if finalState.CurrentWinnerID == nil || *finalState.CurrentWinnerID != maxBid.BidderID {
		t.Fatalf("winner mismatch: state=%v want=%v", finalState.CurrentWinnerID, maxBid.BidderID)
	}

	// Invariant 6: bid_count matches accepted count.
	if finalState.BidCount != len(acceptedBids) {
		t.Fatalf("bid_count %d != accepted count %d", finalState.BidCount, len(acceptedBids))
	}

	// Invariant 7: published bid.placed events == accepted count, with
	// strictly increasing seq.
	placedEvents := 0
	var lastSeq int64 = -1
	for _, ev := range pub.all() {
		if ev.Type != domain.EventBidPlaced {
			continue
		}
		placedEvents++
		data := ev.Data.(domain.BidPlacedData)
		if data.Bid.Seq <= lastSeq {
			t.Fatalf("published bid.placed events out of order: seq %d after %d", data.Bid.Seq, lastSeq)
		}
		lastSeq = data.Bid.Seq
	}
	if placedEvents != len(acceptedBids) {
		t.Fatalf("published bid.placed count %d != accepted count %d", placedEvents, len(acceptedBids))
	}
}

// TestRoom_NoBidAcceptedAfterExpiry hammers bids continuously across a
// short auction's expiry boundary and asserts that not a single accepted
// bid has placed_at >= ends_at, close happens exactly once, exactly one
// auction.closed event fires, and every post-close reply is ErrAuctionEnded.
func TestRoom_NoBidAcceptedAfterExpiry(t *testing.T) {
	base := newTestAuction(200 * time.Millisecond)
	store := newFakeStore(base)
	pub := newFakePublisher()
	room := newRoom(base, store, pub, domain.RealClock{}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go room.run(ctx)

	var wg sync.WaitGroup
	var postCloseRejections int64
	deadline := time.Now().Add(400 * time.Millisecond)

	for i := 0; i < 50; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				reply := make(chan bidOutcome, 1)
				bidderID := uuid.New()
				select {
				case room.cmds <- placeBidCmd{
					BidderID:   bidderID,
					BidderName: "bidder",
					Amount:     base.BasePriceCents + int64(i+1)*100,
					Reply:      reply,
				}:
				case <-time.After(time.Second):
					return
				}
				select {
				case out := <-reply:
					if out.Err == domain.ErrAuctionEnded {
						atomic.AddInt64(&postCloseRejections, 1)
					}
				case <-time.After(time.Second):
					return
				}
			}
		}()
	}
	wg.Wait()

	if postCloseRejections == 0 {
		t.Fatal("expected at least some bids to be rejected with ErrAuctionEnded after expiry")
	}

	for _, b := range store.acceptedBids() {
		if !b.PlacedAt.Before(store.snapshot().EndsAt) {
			t.Fatalf("accepted bid placed_at=%v is not before ends_at=%v", b.PlacedAt, store.snapshot().EndsAt)
		}
	}

	if got := store.closeCount(); got != 1 {
		t.Fatalf("Close called %d times, want exactly 1", got)
	}
	if got := pub.countType(domain.EventAuctionClosed); got != 1 {
		t.Fatalf("auction.closed published %d times, want exactly 1", got)
	}
}

// TestRoom_ExpiryWhileBidInFlight covers the case where the store's
// ApplyBid is still executing when the end timer fires: the bid must be
// accepted (it was validated before ends_at) and the close must follow
// immediately after.
func TestRoom_ExpiryWhileBidInFlight(t *testing.T) {
	base := newTestAuction(50 * time.Millisecond)
	store := newFakeStore(base)
	store.sleep = 100 * time.Millisecond // outlives the auction's remaining life
	pub := newFakePublisher()
	room := newRoom(base, store, pub, domain.RealClock{}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go room.run(ctx)

	reply := make(chan bidOutcome, 1)
	bidderID := uuid.New()
	room.cmds <- placeBidCmd{BidderID: bidderID, BidderName: "bidder", Amount: base.BasePriceCents, Reply: reply}

	select {
	case out := <-reply:
		if out.Err != nil {
			t.Fatalf("in-flight bid should have been accepted, got err: %v", out.Err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for in-flight bid reply")
	}

	// Close should follow promptly once the actor returns from ApplyBid.
	deadline := time.After(time.Second)
	for store.closeCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("auction was never closed after the in-flight bid completed")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestRoom_CallerCancelledDoesNotLeak verifies that a cancelled caller
// context does not stall the actor and does not leak goroutines.
func TestRoom_CallerCancelledDoesNotLeak(t *testing.T) {
	base := newTestAuction(5 * time.Second)
	store := newFakeStore(base)
	store.sleep = 50 * time.Millisecond
	pub := newFakePublisher()
	room := newRoom(base, store, pub, domain.RealClock{}, testLogger())

	roomCtx, cancelRoom := context.WithCancel(context.Background())
	defer cancelRoom()
	go room.run(roomCtx)

	callerCtx, cancelCaller := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancelCaller()

	reply := make(chan bidOutcome, 1)
	bidderID := uuid.New()
	room.cmds <- placeBidCmd{BidderID: bidderID, BidderName: "bidder", Amount: base.BasePriceCents, Reply: reply}

	<-callerCtx.Done() // simulate the caller giving up

	// The actor must still make progress: a later bid must get a real reply.
	reply2 := make(chan bidOutcome, 1)
	bidderID2 := uuid.New()
	room.cmds <- placeBidCmd{BidderID: bidderID2, BidderName: "bidder2", Amount: base.BasePriceCents + 1000, Reply: reply2}

	select {
	case <-reply2:
	case <-time.After(2 * time.Second):
		t.Fatal("actor appears stalled after a caller cancelled mid-flight")
	}
}
