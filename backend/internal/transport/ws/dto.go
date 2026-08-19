package ws

import (
	"time"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

// This file is the WS transport's own DTO layer, mirroring transport/http's
// dto.go: domain event payload structs intentionally carry no JSON tags
// (the domain package must not know about wire formats), so each event
// type is mapped here to the snake_case shape documented in the API
// contract before Hub.Publish marshals it.

type userRefJSON struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type bidJSON struct {
	ID          string    `json:"id"`
	Seq         int64     `json:"seq"`
	AmountCents int64     `json:"amount_cents"`
	BidderID    string    `json:"bidder_id"`
	BidderName  string    `json:"bidder_name"`
	PlacedAt    time.Time `json:"placed_at"`
}

type bidPlacedJSON struct {
	Bid     bidJSON              `json:"bid"`
	Auction bidPlacedAuctionJSON `json:"auction"`
}

type bidPlacedAuctionJSON struct {
	CurrentPriceCents int64        `json:"current_price_cents"`
	MinNextBidCents   int64        `json:"min_next_bid_cents"`
	CurrentWinner     *userRefJSON `json:"current_winner"`
	BidCount          int          `json:"bid_count"`
}

type bidderOutbidJSON struct {
	OutbidUserID  string `json:"outbid_user_id"`
	NewPriceCents int64  `json:"new_price_cents"`
	ByUsername    string `json:"by_username"`
}

type auctionStartedJSON struct {
	AuctionID string    `json:"auction_id"`
	StartedAt time.Time `json:"started_at"`
	EndsAt    time.Time `json:"ends_at"`
}

type auctionClosedJSON struct {
	AuctionID       string       `json:"auction_id"`
	FinalPriceCents int64        `json:"final_price_cents"`
	Winner          *userRefJSON `json:"winner"`
	BidCount        int          `json:"bid_count"`
	ClosedAt        time.Time    `json:"closed_at"`
}

// eventDataJSON converts a domain.Event's payload into its documented wire
// shape. Unknown payload types pass through unchanged (defensive default,
// not expected to be hit given the closed set of EventType constants).
func eventDataJSON(ev domain.Event) any {
	switch d := ev.Data.(type) {
	case domain.BidPlacedData:
		return bidPlacedJSON{
			Bid: bidJSON{
				ID: d.Bid.ID.String(), Seq: d.Bid.Seq, AmountCents: d.Bid.AmountCents,
				BidderID: d.Bid.BidderID.String(), BidderName: d.Bid.BidderName, PlacedAt: d.Bid.PlacedAt,
			},
			Auction: bidPlacedAuctionJSON{
				CurrentPriceCents: d.CurrentPriceCents,
				MinNextBidCents:   d.MinNextBidCents,
				CurrentWinner:     winnerRef(d.CurrentWinnerID, d.CurrentWinnerName),
				BidCount:          d.BidCount,
			},
		}
	case domain.BidderOutbidData:
		return bidderOutbidJSON{
			OutbidUserID:  d.OutbidUserID.String(),
			NewPriceCents: d.NewPriceCents,
			ByUsername:    d.ByUsername,
		}
	case domain.AuctionStartedData:
		return auctionStartedJSON{AuctionID: d.AuctionID.String(), StartedAt: d.StartedAt, EndsAt: d.EndsAt}
	case domain.AuctionClosedData:
		return auctionClosedJSON{
			AuctionID:       d.AuctionID.String(),
			FinalPriceCents: d.FinalPriceCents,
			Winner:          winnerRef(d.WinnerID, d.WinnerUsername),
			BidCount:        d.BidCount,
			ClosedAt:        d.ClosedAt,
		}
	default:
		return ev.Data
	}
}

func winnerRef(id *uuid.UUID, username string) *userRefJSON {
	if id == nil {
		return nil
	}
	return &userRefJSON{ID: id.String(), Username: username}
}
