package domain

import (
	"time"

	"github.com/google/uuid"
)

type Bid struct {
	ID          uuid.UUID
	Seq         int64
	AuctionID   uuid.UUID
	BidderID    uuid.UUID
	BidderName  string
	AmountCents int64
	PlacedAt    time.Time
}

// ApplyBidParams / ApplyBidResult are the request/response pair for the
// conditional-UPDATE bid application at the repository layer (defense in
// depth, Layer 3 of the concurrency design — see postgres.ApplyBid).
type ApplyBidParams struct {
	AuctionID   uuid.UUID
	BidderID    uuid.UUID
	BidderName  string
	AmountCents int64
	PlacedAt    time.Time
}

type ApplyBidResult struct {
	Auction Auction
	Bid     Bid
}
