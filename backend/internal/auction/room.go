// Package auction implements the concurrent bidding engine. It is the
// correctness core of BidCraft and is deliberately transport-agnostic: this
// package imports neither net/http nor any WebSocket package (enforced by
// deps_test.go), which is the concrete, testable proof of the OCP/ISP
// decoupling the spec asks for.
//
// Concurrency design (see README "Race Conditions" section for the full
// rationale):
//
//	Layer 1 - Room actor:      one goroutine + a command channel serializes
//	                            every bid AND the expiry timer through a
//	                            single select loop, so two bids can never be
//	                            evaluated against the same price at once,
//	                            and a bid can never be "in evaluation" at
//	                            the same instant the auction closes.
//	Layer 2 - Expiry guard:     select{} picks uniformly at random among
//	                            ready cases, so a fired end-timer does NOT
//	                            guarantee it will be chosen over an already
//	                            queued bid command. handleBid re-checks wall
//	                            clock time explicitly before accepting.
//	Layer 3 - Conditional SQL:  the repository's ApplyBid re-expresses the
//	                            bid rule independently in a single
//	                            conditional UPDATE, so correctness does not
//	                            rely solely on this process's in-memory
//	                            state (see postgres.ApplyBid).
package auction

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

// bidStore is a narrow, consumer-defined interface (ISP): the room only
// needs three repository operations, not the full domain.AuctionRepository.
// This keeps the room's test fake to ~30 lines instead of ~120.
type bidStore interface {
	ApplyBid(ctx context.Context, p domain.ApplyBidParams) (domain.ApplyBidResult, error)
	Activate(ctx context.Context, id uuid.UUID, at time.Time) error
	Close(ctx context.Context, id uuid.UUID, at time.Time) (domain.Auction, error)
}

const defaultCmdBuffer = 256

type command interface{ isCommand() }

type placeBidCmd struct {
	BidderID   uuid.UUID
	BidderName string
	Amount     int64
	Reply      chan bidOutcome // ALWAYS buffered cap 1 -- see handleBid's send.
}

func (placeBidCmd) isCommand() {}

type snapshotCmd struct {
	Reply chan domain.Auction // buffered cap 1
}

func (snapshotCmd) isCommand() {}

type bidOutcome struct {
	Bid     domain.Bid
	Auction domain.Auction
	Err     error
}

// Room is one actor goroutine owning the full lifecycle of a single auction.
type Room struct {
	id        uuid.UUID
	cmds      chan command
	done      chan struct{} // closed when run() returns -- the liveness signal
	store     bidStore
	publisher domain.EventPublisher
	clock     domain.Clock
	log       *slog.Logger

	// state is owned EXCLUSIVELY by run(). No mutex protects it, because no
	// other goroutine can reach it -- that confinement IS the synchronization.
	state domain.Auction

	// winnerName mirrors state.CurrentWinnerID, set from the accepted bid's
	// BidderName. Avoids giving the room a UserRepository dependency (ISP)
	// just to resolve a username for the auction.closed event.
	winnerName string
}

func newRoom(a domain.Auction, store bidStore, pub domain.EventPublisher, clock domain.Clock, log *slog.Logger) *Room {
	return &Room{
		id:        a.ID,
		cmds:      make(chan command, defaultCmdBuffer),
		done:      make(chan struct{}),
		store:     store,
		publisher: pub,
		clock:     clock,
		log:       log,
		state:     a,
	}
}

func (r *Room) run(ctx context.Context) {
	defer close(r.done)

	var startC <-chan time.Time
	if r.state.Status == domain.StatusCreated {
		t := time.NewTimer(time.Until(r.state.StartsAt))
		defer t.Stop()
		startC = t.C
	}
	endT := time.NewTimer(time.Until(r.state.EndsAt))
	defer endT.Stop()

	for {
		select {
		case <-endT.C:
			r.finish(ctx)
			r.drainRejecting()
			return

		case <-startC:
			r.activate(ctx)
			startC = nil

		case c := <-r.cmds:
			switch cmd := c.(type) {
			case placeBidCmd:
				r.handleBid(ctx, cmd)
			case snapshotCmd:
				cmd.Reply <- r.state
			}

		case <-ctx.Done():
			// Graceful shutdown: leave DB state alone. Manager.Recover
			// re-arms this auction's room on the next process boot.
			r.drainRejecting()
			return
		}
	}
}

func (r *Room) activate(ctx context.Context) {
	if err := r.store.Activate(ctx, r.id, r.clock.Now()); err != nil {
		r.log.Error("activate auction failed", "auction_id", r.id, "err", err)
		return
	}
	r.state.Status = domain.StatusActive
	r.publisher.Publish(domain.NewAuctionStarted(r.state))
}

func (r *Room) finish(ctx context.Context) {
	closed, err := r.store.Close(ctx, r.id, r.clock.Now())
	if err != nil {
		r.log.Error("close auction failed", "auction_id", r.id, "err", err)
		return
	}
	r.state = closed
	r.publisher.Publish(domain.NewAuctionClosed(r.state, r.winnerName))
}

// handleBid is the correctness core: the bid rule is applied here (Layer 2's
// explicit expiry guard) and then re-verified independently by the database
// (Layer 3) before being admitted.
func (r *Room) handleBid(ctx context.Context, cmd placeBidCmd) {
	now := r.clock.Now()

	// LAYER 2. select{} chooses randomly among ready cases, so a fired
	// end-timer does NOT guarantee we won't be handed a queued bid first.
	// Check wall-clock time explicitly rather than trusting scheduling order.
	if !now.Before(r.state.EndsAt) || r.state.Status == domain.StatusFinished {
		cmd.Reply <- bidOutcome{Err: domain.ErrAuctionEnded}
		return
	}
	if r.state.Status != domain.StatusActive {
		cmd.Reply <- bidOutcome{Err: domain.ErrAuctionNotStarted}
		return
	}
	if err := r.state.ValidateBid(cmd.Amount, cmd.BidderID); err != nil {
		cmd.Reply <- bidOutcome{Err: err, Auction: r.state}
		return
	}

	// The DB write must not be cancelled by the HTTP caller hanging up
	// mid-flight: the bid has already been admitted into the serialized
	// order and must be persisted to keep the actor's state and the DB
	// in sync.
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	res, err := r.store.ApplyBid(wctx, domain.ApplyBidParams{
		AuctionID:   r.id,
		BidderID:    cmd.BidderID,
		BidderName:  cmd.BidderName,
		AmountCents: cmd.Amount,
		PlacedAt:    now,
	})
	cancel()
	if err != nil {
		// Includes ErrBidRejected surfaced by Layer 3 (the conditional
		// UPDATE affected zero rows): the DB disagreed with our in-memory
		// state, which the DB always wins.
		cmd.Reply <- bidOutcome{Err: err}
		return
	}

	prevWinner := r.state.CurrentWinnerID
	r.state = res.Auction // authoritative post-image from RETURNING
	r.winnerName = cmd.BidderName

	// Reply to the caller BEFORE publishing: the HTTP 201 is what the
	// bidder needs immediately; the broadcast is best-effort. This also
	// means the bidder's own POST response and the WS event can arrive in
	// either order -- clients dedupe by bid.seq.
	cmd.Reply <- bidOutcome{Bid: res.Bid, Auction: res.Auction}

	r.publisher.Publish(domain.NewBidPlaced(res))
	if prevWinner != nil && *prevWinner != cmd.BidderID {
		r.publisher.Publish(domain.NewBidderOutbid(res, *prevWinner))
	}
}

// drainRejecting empties any commands left in the buffer after the room has
// decided to stop, replying with a definitive error instead of leaving
// callers to block until Manager's room.done guard times them out.
func (r *Room) drainRejecting() {
	for {
		select {
		case c := <-r.cmds:
			switch cmd := c.(type) {
			case placeBidCmd:
				cmd.Reply <- bidOutcome{Err: domain.ErrAuctionEnded}
			case snapshotCmd:
				cmd.Reply <- r.state
			}
		default:
			return
		}
	}
}
