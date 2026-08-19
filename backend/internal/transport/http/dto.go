package http

import (
	"time"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

// --- Auth ---

type registerRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userDTO struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type authResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      userDTO   `json:"user"`
}

func toUserDTO(u domain.User) userDTO {
	return userDTO{ID: u.ID.String(), Username: u.Username, Email: u.Email}
}

// --- Auctions ---

type createAuctionRequest struct {
	Title             string `json:"title"`
	Description       string `json:"description"`
	ImageURL          string `json:"image_url"`
	BasePriceCents    int64  `json:"base_price_cents"`
	MinIncrementCents int64  `json:"min_increment_cents"`
	DurationSeconds   int64  `json:"duration_seconds"`
	StartsInSeconds   int64  `json:"starts_in_seconds"`
}

type userRefDTO struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type auctionDetailDTO struct {
	ID                string      `json:"id"`
	Title             string      `json:"title"`
	Description       string      `json:"description"`
	ImageURL          string      `json:"image_url"`
	Status            string      `json:"status"`
	BasePriceCents    int64       `json:"base_price_cents"`
	CurrentPriceCents int64       `json:"current_price_cents"`
	MinIncrementCents int64       `json:"min_increment_cents"`
	MinNextBidCents   int64       `json:"min_next_bid_cents"`
	BidCount          int         `json:"bid_count"`
	CurrentWinner     *userRefDTO `json:"current_winner"`
	Seller            userRefDTO  `json:"seller"`
	StartsAt          time.Time   `json:"starts_at"`
	EndsAt            time.Time   `json:"ends_at"`
	ClosedAt          *time.Time  `json:"closed_at"`
	ServerTimeMs      int64       `json:"server_time_ms"`
}

func toAuctionDetailDTO(a domain.Auction, sellerUsername string, winnerUsername string) auctionDetailDTO {
	dto := auctionDetailDTO{
		ID:                a.ID.String(),
		Title:             a.Title,
		Description:       a.Description,
		ImageURL:          a.ImageURL,
		Status:            string(a.Status),
		BasePriceCents:    a.BasePriceCents,
		CurrentPriceCents: a.CurrentPriceCents,
		MinIncrementCents: a.MinIncrementCents,
		MinNextBidCents:   a.MinNextBidCents(),
		BidCount:          a.BidCount,
		Seller:            userRefDTO{ID: a.SellerID.String(), Username: sellerUsername},
		StartsAt:          a.StartsAt,
		EndsAt:            a.EndsAt,
		ClosedAt:          a.ClosedAt,
		ServerTimeMs:      time.Now().UTC().UnixMilli(),
	}
	if a.CurrentWinnerID != nil {
		dto.CurrentWinner = &userRefDTO{ID: a.CurrentWinnerID.String(), Username: winnerUsername}
	}
	return dto
}

type auctionListResponse struct {
	Data         []auctionDetailDTO `json:"data"`
	ServerTimeMs int64              `json:"server_time_ms"`
}

// --- Bids ---

type placeBidRequest struct {
	AmountCents int64 `json:"amount_cents"`
}

type bidDTO struct {
	ID          string    `json:"id"`
	Seq         int64     `json:"seq"`
	AuctionID   string    `json:"auction_id"`
	BidderID    string    `json:"bidder_id"`
	BidderName  string    `json:"bidder_name"`
	AmountCents int64     `json:"amount_cents"`
	PlacedAt    time.Time `json:"placed_at"`
}

func toBidDTO(b domain.Bid) bidDTO {
	return bidDTO{
		ID: b.ID.String(), Seq: b.Seq, AuctionID: b.AuctionID.String(),
		BidderID: b.BidderID.String(), BidderName: b.BidderName,
		AmountCents: b.AmountCents, PlacedAt: b.PlacedAt,
	}
}

type bidListResponse struct {
	Data         []bidDTO `json:"data"`
	ServerTimeMs int64    `json:"server_time_ms"`
}

type placeBidResponse struct {
	Bid     bidDTO           `json:"bid"`
	Auction auctionDetailDTO `json:"auction"`
}

type serverTimeResponse struct {
	ServerTimeMs int64 `json:"server_time_ms"`
}
