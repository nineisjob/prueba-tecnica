package domain

import (
	"time"

	"github.com/google/uuid"
)

type EventType string

const (
	EventSnapshot       EventType = "snapshot"
	EventAuctionStarted EventType = "auction.started"
	EventBidPlaced      EventType = "bid.placed"
	EventBidderOutbid   EventType = "bidder.outbid"
	EventAuctionClosed  EventType = "auction.closed"
)

// Event is transport-agnostic: internal/auction publishes these without any
// knowledge of WebSockets, JSON encoding, or HTTP. transport/ws.Hub is the
// only thing that knows how to serialize and fan them out.
type Event struct {
	Type      EventType
	AuctionID uuid.UUID
	Data      any
}

type BidPlacedData struct {
	Bid               Bid
	CurrentPriceCents int64
	MinNextBidCents   int64
	CurrentWinnerID   *uuid.UUID
	CurrentWinnerName string
	BidCount          int
}

type BidderOutbidData struct {
	OutbidUserID  uuid.UUID
	NewPriceCents int64
	ByUsername    string
}

type AuctionStartedData struct {
	AuctionID uuid.UUID
	StartedAt time.Time
	EndsAt    time.Time
}

type AuctionClosedData struct {
	AuctionID       uuid.UUID
	FinalPriceCents int64
	WinnerID        *uuid.UUID
	WinnerUsername  string
	BidCount        int
	ClosedAt        time.Time
}

// EventPublisher decouples the auction engine from the WebSocket transport
// (the OCP/ISP boundary the spec explicitly asks for).
//
// CONTRACT: Publish MUST NOT block. Implementations that cannot deliver
// immediately must drop the message or drop the subscriber -- never stall
// the caller (the room actor's single goroutine).
type EventPublisher interface {
	Publish(ev Event)
}

func NewBidPlaced(res ApplyBidResult) Event {
	return Event{
		Type:      EventBidPlaced,
		AuctionID: res.Auction.ID,
		Data: BidPlacedData{
			Bid:               res.Bid,
			CurrentPriceCents: res.Auction.CurrentPriceCents,
			MinNextBidCents:   res.Auction.MinNextBidCents(),
			CurrentWinnerID:   res.Auction.CurrentWinnerID,
			CurrentWinnerName: res.Bid.BidderName,
			BidCount:          res.Auction.BidCount,
		},
	}
}

func NewBidderOutbid(res ApplyBidResult, outbidUserID uuid.UUID) Event {
	return Event{
		Type:      EventBidderOutbid,
		AuctionID: res.Auction.ID,
		Data: BidderOutbidData{
			OutbidUserID:  outbidUserID,
			NewPriceCents: res.Auction.CurrentPriceCents,
			ByUsername:    res.Bid.BidderName,
		},
	}
}

func NewAuctionStarted(a Auction) Event {
	return Event{
		Type:      EventAuctionStarted,
		AuctionID: a.ID,
		Data: AuctionStartedData{
			AuctionID: a.ID,
			StartedAt: a.StartsAt,
			EndsAt:    a.EndsAt,
		},
	}
}

func NewAuctionClosed(a Auction, winnerUsername string) Event {
	var closedAt time.Time
	if a.ClosedAt != nil {
		closedAt = *a.ClosedAt
	}
	return Event{
		Type:      EventAuctionClosed,
		AuctionID: a.ID,
		Data: AuctionClosedData{
			AuctionID:       a.ID,
			FinalPriceCents: a.CurrentPriceCents,
			WinnerID:        a.CurrentWinnerID,
			WinnerUsername:  winnerUsername,
			BidCount:        a.BidCount,
			ClosedAt:        closedAt,
		},
	}
}
