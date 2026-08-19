package auction

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

func TestRoom_AcceptsFirstBidAtBasePrice(t *testing.T) {
	base := newTestAuction(2 * time.Second)
	store := newFakeStore(base)
	pub := newFakePublisher()
	room := newRoom(base, store, pub, domain.RealClock{}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go room.run(ctx)

	reply := make(chan bidOutcome, 1)
	room.cmds <- placeBidCmd{BidderID: uuid.New(), BidderName: "alice", Amount: base.BasePriceCents, Reply: reply}
	out := <-reply
	if out.Err != nil {
		t.Fatalf("expected first bid at base price to be accepted, got: %v", out.Err)
	}
}

func TestRoom_RejectsBidBelowBasePrice(t *testing.T) {
	base := newTestAuction(2 * time.Second)
	store := newFakeStore(base)
	pub := newFakePublisher()
	room := newRoom(base, store, pub, domain.RealClock{}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go room.run(ctx)

	reply := make(chan bidOutcome, 1)
	room.cmds <- placeBidCmd{BidderID: uuid.New(), BidderName: "alice", Amount: base.BasePriceCents - 1, Reply: reply}
	out := <-reply
	if out.Err != domain.ErrBidTooLow {
		t.Fatalf("expected ErrBidTooLow, got: %v", out.Err)
	}
}

func TestRoom_RejectsBidBelowCurrentPlusIncrement(t *testing.T) {
	base := newTestAuction(2 * time.Second)
	store := newFakeStore(base)
	pub := newFakePublisher()
	room := newRoom(base, store, pub, domain.RealClock{}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go room.run(ctx)

	reply1 := make(chan bidOutcome, 1)
	room.cmds <- placeBidCmd{BidderID: uuid.New(), BidderName: "alice", Amount: base.BasePriceCents, Reply: reply1}
	out1 := <-reply1
	if out1.Err != nil {
		t.Fatalf("setup bid failed: %v", out1.Err)
	}

	wantMin := out1.Auction.CurrentPriceCents + out1.Auction.MinIncrementCents

	reply2 := make(chan bidOutcome, 1)
	room.cmds <- placeBidCmd{BidderID: uuid.New(), BidderName: "bob", Amount: wantMin - 1, Reply: reply2}
	out2 := <-reply2
	if out2.Err != domain.ErrBidTooLow {
		t.Fatalf("expected ErrBidTooLow for bid below current+increment, got: %v", out2.Err)
	}

	reply3 := make(chan bidOutcome, 1)
	room.cmds <- placeBidCmd{BidderID: uuid.New(), BidderName: "bob", Amount: wantMin, Reply: reply3}
	out3 := <-reply3
	if out3.Err != nil {
		t.Fatalf("expected bid at exactly current+increment to be accepted, got: %v", out3.Err)
	}
}

func TestRoom_RejectsSelfOutbid(t *testing.T) {
	base := newTestAuction(2 * time.Second)
	store := newFakeStore(base)
	pub := newFakePublisher()
	room := newRoom(base, store, pub, domain.RealClock{}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go room.run(ctx)

	alice := uuid.New()
	reply1 := make(chan bidOutcome, 1)
	room.cmds <- placeBidCmd{BidderID: alice, BidderName: "alice", Amount: base.BasePriceCents, Reply: reply1}
	out1 := <-reply1
	if out1.Err != nil {
		t.Fatalf("setup bid failed: %v", out1.Err)
	}

	reply2 := make(chan bidOutcome, 1)
	nextMin := out1.Auction.CurrentPriceCents + out1.Auction.MinIncrementCents
	room.cmds <- placeBidCmd{BidderID: alice, BidderName: "alice", Amount: nextMin, Reply: reply2}
	out2 := <-reply2
	if out2.Err != domain.ErrAlreadyHighest {
		t.Fatalf("expected ErrAlreadyHighest, got: %v", out2.Err)
	}
}

// TestRoom_RejectsBidAfterClose enqueues a bid BEFORE starting run(), on an
// auction whose EndsAt is already in the past. select{} in run() will pick
// between the already-ready endT.C case and the already-queued cmds case at
// random -- exercising both the Layer-2 expiry guard (if cmds wins) and
// drainRejecting (if endT.C wins). Either way the bid must be rejected with
// ErrAuctionEnded, which is the point: once a room is gone, nothing sent to
// it after the fact can ever be silently lost or accepted. (Routing a bid
// to an already-finished room, detected via room.done, is the Manager's
// job -- see TestManager_BidDuringRoomShutdown -- not the room's own.)
func TestRoom_RejectsBidAfterClose(t *testing.T) {
	base := newTestAuction(time.Second)
	base.EndsAt = time.Now().UTC().Add(-time.Millisecond)
	store := newFakeStore(base)
	pub := newFakePublisher()
	room := newRoom(base, store, pub, domain.RealClock{}, testLogger())

	reply := make(chan bidOutcome, 1)
	room.cmds <- placeBidCmd{BidderID: uuid.New(), BidderName: "late", Amount: base.BasePriceCents, Reply: reply}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go room.run(ctx)

	select {
	case out := <-reply:
		if out.Err != domain.ErrAuctionEnded {
			t.Fatalf("expected ErrAuctionEnded, got: %v", out.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reply")
	}
}

func TestRoom_ClosesExactlyOnceAndPublishesOneClosedEvent(t *testing.T) {
	base := newTestAuction(30 * time.Millisecond)
	store := newFakeStore(base)
	pub := newFakePublisher()
	room := newRoom(base, store, pub, domain.RealClock{}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go room.run(ctx)

	<-room.done

	if got := store.closeCount(); got != 1 {
		t.Fatalf("Close called %d times, want 1", got)
	}
	if got := pub.countType(domain.EventAuctionClosed); got != 1 {
		t.Fatalf("auction.closed published %d times, want 1", got)
	}
}
