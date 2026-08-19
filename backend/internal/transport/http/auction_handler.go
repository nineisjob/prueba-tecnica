package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/auction"
	"github.com/geferson/bidcraft/backend/internal/domain"
)

// Narrow, handler-defined interfaces (ISP/DIP at the transport edge): this
// handler depends only on the operations it actually calls, not on the
// full domain.AuctionRepository or the concrete *auction.Manager. Depending
// on *auction.Room here (rather than an erased `any`) is a one-way import
// from transport -> auction, which deps_test.go proves is never reciprocated.
type AuctionCreator interface {
	Create(ctx context.Context, a *domain.Auction) error
}

type AuctionLister interface {
	List(ctx context.Context, f domain.AuctionFilter) ([]domain.Auction, error)
}

type AuctionSpawner interface {
	Spawn(a domain.Auction) *auction.Room
}

type AuctionSnapshotter interface {
	Snapshot(ctx context.Context, auctionID uuid.UUID) (domain.Auction, error)
}

type BidPlacer interface {
	PlaceBid(ctx context.Context, auctionID, bidderID uuid.UUID, bidderName string, amountCents int64) (domain.Bid, domain.Auction, error)
}

type BidLister interface {
	ListByAuction(ctx context.Context, auctionID uuid.UUID, limit int) ([]domain.Bid, error)
}

type UserGetter interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

type AuctionHandler struct {
	repo     AuctionCreator
	lister   AuctionLister
	spawner  AuctionSpawner
	snapshot AuctionSnapshotter
	bids     BidPlacer
	bidList  BidLister
	users    UserGetter
}

func NewAuctionHandler(repo AuctionCreator, lister AuctionLister, spawner AuctionSpawner, snap AuctionSnapshotter, bids BidPlacer, bidList BidLister, users UserGetter) *AuctionHandler {
	return &AuctionHandler{repo: repo, lister: lister, spawner: spawner, snapshot: snap, bids: bids, bidList: bidList, users: users}
}

func (h *AuctionHandler) usernameFor(ctx context.Context, id uuid.UUID) string {
	u, err := h.users.GetByID(ctx, id)
	if err != nil {
		return ""
	}
	return u.Username
}

func (h *AuctionHandler) toDetailDTO(ctx context.Context, a domain.Auction) auctionDetailDTO {
	seller := h.usernameFor(ctx, a.SellerID)
	winner := ""
	if a.CurrentWinnerID != nil {
		winner = h.usernameFor(ctx, *a.CurrentWinnerID)
	}
	return toAuctionDetailDTO(a, seller, winner)
}

func (h *AuctionHandler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	auctions, err := h.lister.List(r.Context(), domain.AuctionFilter{Status: status})
	if err != nil {
		writeError(w, r, err)
		return
	}
	dtos := make([]auctionDetailDTO, 0, len(auctions))
	for _, a := range auctions {
		dtos = append(dtos, h.toDetailDTO(r.Context(), a))
	}
	writeJSON(w, http.StatusOK, auctionListResponse{Data: dtos, ServerTimeMs: time.Now().UTC().UnixMilli()})
}

func (h *AuctionHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, r, domain.ErrInvalidInput)
		return
	}
	a, err := h.snapshot.Snapshot(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.toDetailDTO(r.Context(), a))
}

func (h *AuctionHandler) Create(w http.ResponseWriter, r *http.Request) {
	authUser, ok := AuthUserFromContext(r.Context())
	if !ok {
		writeError(w, r, domain.ErrUnauthenticated)
		return
	}

	var req createAuctionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, domain.ErrInvalidInput)
		return
	}
	if !domain.NewAuctionTitleValid(req.Title) || req.ImageURL == "" ||
		req.BasePriceCents < 0 || req.MinIncrementCents <= 0 || req.DurationSeconds <= 0 {
		writeError(w, r, domain.ErrInvalidInput)
		return
	}

	now := time.Now().UTC()
	starts := now.Add(time.Duration(req.StartsInSeconds) * time.Second)
	ends := starts.Add(time.Duration(req.DurationSeconds) * time.Second)

	status := domain.StatusActive
	if req.StartsInSeconds > 0 {
		status = domain.StatusCreated
	}

	a := domain.Auction{
		SellerID:          authUser.ID,
		Title:             req.Title,
		Description:       req.Description,
		ImageURL:          req.ImageURL,
		BasePriceCents:    req.BasePriceCents,
		CurrentPriceCents: req.BasePriceCents,
		MinIncrementCents: req.MinIncrementCents,
		Status:            status,
		StartsAt:          starts,
		EndsAt:            ends,
	}
	if err := h.repo.Create(r.Context(), &a); err != nil {
		writeError(w, r, err)
		return
	}

	h.spawner.Spawn(a) // starts the room actor immediately so it can accept bids/expire

	writeJSON(w, http.StatusCreated, h.toDetailDTO(r.Context(), a))
}

func (h *AuctionHandler) ListBids(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, r, domain.ErrInvalidInput)
		return
	}
	bids, err := h.bidList.ListByAuction(r.Context(), id, 50)
	if err != nil {
		writeError(w, r, err)
		return
	}
	dtos := make([]bidDTO, 0, len(bids))
	for _, b := range bids {
		dtos = append(dtos, toBidDTO(b))
	}
	writeJSON(w, http.StatusOK, bidListResponse{Data: dtos, ServerTimeMs: time.Now().UTC().UnixMilli()})
}

func (h *AuctionHandler) PlaceBid(w http.ResponseWriter, r *http.Request) {
	authUser, ok := AuthUserFromContext(r.Context())
	if !ok {
		writeError(w, r, domain.ErrUnauthenticated)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, r, domain.ErrInvalidInput)
		return
	}
	var req placeBidRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AmountCents <= 0 {
		writeError(w, r, domain.ErrInvalidInput)
		return
	}

	bid, a, err := h.bids.PlaceBid(r.Context(), id, authUser.ID, authUser.Username, req.AmountCents)
	if err != nil {
		if err == domain.ErrBidTooLow {
			writeErrorDetails(w, r, err, bidTooLowDetails(a.MinNextBidCents(), a.CurrentPriceCents))
			return
		}
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, placeBidResponse{Bid: toBidDTO(bid), Auction: h.toDetailDTO(r.Context(), a)})
}
