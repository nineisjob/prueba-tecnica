package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusCreated  Status = "created"
	StatusActive   Status = "active"
	StatusFinished Status = "finished"
)

type Auction struct {
	ID                uuid.UUID
	SellerID          uuid.UUID
	Title             string
	Description       string
	ImageURL          string
	BasePriceCents    int64
	CurrentPriceCents int64
	MinIncrementCents int64
	CurrentWinnerID   *uuid.UUID
	BidCount          int
	Status            Status
	StartsAt          time.Time
	EndsAt            time.Time
	ClosedAt          *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// MinNextBidCents is THE single definition of the bid rule. It is mirrored
// (deliberately, as a defense-in-depth duplication — see the SQL guard in
// postgres.ApplyBid) by the conditional UPDATE's CASE expression, and pinned
// by TestSQLGuardMatchesDomainRule so the two can never silently diverge.
func (a *Auction) MinNextBidCents() int64 {
	if a.BidCount == 0 {
		return a.BasePriceCents
	}
	return a.CurrentPriceCents + a.MinIncrementCents
}

// ValidateBid applies the business rule only; it does not check auction
// status/timing (that is the room actor's job, since it owns the clock
// decision — see internal/auction/room.go).
func (a *Auction) ValidateBid(amountCents int64, bidderID uuid.UUID) error {
	if amountCents <= 0 {
		return ErrInvalidInput
	}
	if a.CurrentWinnerID != nil && *a.CurrentWinnerID == bidderID {
		return ErrAlreadyHighest
	}
	if amountCents < a.MinNextBidCents() {
		return ErrBidTooLow
	}
	return nil
}

func NewAuctionTitleValid(title string) bool {
	t := strings.TrimSpace(title)
	return len(t) >= 3 && len(t) <= 140
}

type AuctionFilter struct {
	Status string // "active" | "created" | "finished" | "all" | ""
	Limit  int
	Offset int
}
