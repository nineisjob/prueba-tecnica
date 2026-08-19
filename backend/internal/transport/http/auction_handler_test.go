package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/auction"
	"github.com/geferson/bidcraft/backend/internal/domain"
)

type fakeCreator struct {
	created *domain.Auction
	err     error
}

func (f *fakeCreator) Create(ctx context.Context, a *domain.Auction) error {
	if f.err != nil {
		return f.err
	}
	a.ID = uuid.New()
	f.created = a
	return nil
}

type fakeLister struct {
	auctions []domain.Auction
	err      error
}

func (f *fakeLister) List(ctx context.Context, filter domain.AuctionFilter) ([]domain.Auction, error) {
	return f.auctions, f.err
}

type fakeSpawner struct{ spawned []domain.Auction }

func (f *fakeSpawner) Spawn(a domain.Auction) *auction.Room {
	f.spawned = append(f.spawned, a)
	return nil
}

type fakeSnapshotter struct {
	byID map[uuid.UUID]domain.Auction
	err  error
}

func (f *fakeSnapshotter) Snapshot(ctx context.Context, id uuid.UUID) (domain.Auction, error) {
	if f.err != nil {
		return domain.Auction{}, f.err
	}
	a, ok := f.byID[id]
	if !ok {
		return domain.Auction{}, domain.ErrAuctionNotFound
	}
	return a, nil
}

type fakeBidPlacer struct {
	resultBid     domain.Bid
	resultAuction domain.Auction
	err           error
}

func (f *fakeBidPlacer) PlaceBid(ctx context.Context, auctionID, bidderID uuid.UUID, bidderName string, amountCents int64) (domain.Bid, domain.Auction, error) {
	return f.resultBid, f.resultAuction, f.err
}

type fakeBidLister struct {
	bids []domain.Bid
	err  error
}

func (f *fakeBidLister) ListByAuction(ctx context.Context, auctionID uuid.UUID, limit int) ([]domain.Bid, error) {
	return f.bids, f.err
}

type fakeUserGetter struct{ names map[uuid.UUID]string }

func (f *fakeUserGetter) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	name, ok := f.names[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return &domain.User{ID: id, Username: name}, nil
}

func withAuthUser(r *http.Request, u domain.AuthUser) *http.Request {
	ctx := context.WithValue(r.Context(), ctxKeyAuthUser, u)
	return r.WithContext(ctx)
}

func TestAuctionHandler_PlaceBid_TooLow_Returns409WithMinNextBid(t *testing.T) {
	auctionID := uuid.New()
	bidder := uuid.New()

	placer := &fakeBidPlacer{
		err: domain.ErrBidTooLow,
		resultAuction: domain.Auction{
			CurrentPriceCents: 1000, MinIncrementCents: 100, BidCount: 1,
		},
	}
	h := NewAuctionHandler(&fakeCreator{}, &fakeLister{}, &fakeSpawner{}, &fakeSnapshotter{}, placer, &fakeBidLister{}, &fakeUserGetter{})

	body, _ := json.Marshal(placeBidRequest{AmountCents: 1050})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auctions/"+auctionID.String()+"/bids", bytes.NewReader(body))
	req.SetPathValue("id", auctionID.String())
	req = withAuthUser(req, domain.AuthUser{ID: bidder, Username: "alice"})
	rec := httptest.NewRecorder()

	h.PlaceBid(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	var body2 errorEnvelope
	decodeJSON(t, rec.Body.Bytes(), &body2)
	if body2.Error.Code != "BID_TOO_LOW" {
		t.Fatalf("code = %q, want BID_TOO_LOW", body2.Error.Code)
	}
	details, ok := body2.Error.Details.(map[string]any)
	if !ok {
		t.Fatalf("expected details map, got %T", body2.Error.Details)
	}
	if details["min_next_bid_cents"] != float64(1100) {
		t.Fatalf("min_next_bid_cents = %v, want 1100", details["min_next_bid_cents"])
	}
}

func TestAuctionHandler_PlaceBid_Success(t *testing.T) {
	auctionID := uuid.New()
	bidder := uuid.New()

	placer := &fakeBidPlacer{
		resultBid:     domain.Bid{ID: uuid.New(), Seq: 1, AmountCents: 1500},
		resultAuction: domain.Auction{ID: auctionID, CurrentPriceCents: 1500, MinIncrementCents: 100, SellerID: uuid.New()},
	}
	h := NewAuctionHandler(&fakeCreator{}, &fakeLister{}, &fakeSpawner{}, &fakeSnapshotter{}, placer, &fakeBidLister{}, &fakeUserGetter{})

	body, _ := json.Marshal(placeBidRequest{AmountCents: 1500})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auctions/"+auctionID.String()+"/bids", bytes.NewReader(body))
	req.SetPathValue("id", auctionID.String())
	req = withAuthUser(req, domain.AuthUser{ID: bidder, Username: "alice"})
	rec := httptest.NewRecorder()

	h.PlaceBid(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuctionHandler_PlaceBid_RequiresAuth(t *testing.T) {
	h := NewAuctionHandler(&fakeCreator{}, &fakeLister{}, &fakeSpawner{}, &fakeSnapshotter{}, &fakeBidPlacer{}, &fakeBidLister{}, &fakeUserGetter{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auctions/"+uuid.NewString()+"/bids", bytes.NewReader([]byte(`{"amount_cents":100}`)))
	req.SetPathValue("id", uuid.NewString())
	rec := httptest.NewRecorder()

	h.PlaceBid(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuctionHandler_Create_SpawnsRoom(t *testing.T) {
	creator := &fakeCreator{}
	spawner := &fakeSpawner{}
	h := NewAuctionHandler(creator, &fakeLister{}, spawner, &fakeSnapshotter{}, &fakeBidPlacer{}, &fakeBidLister{}, &fakeUserGetter{})

	reqBody := createAuctionRequest{
		Title: "Test Piece", ImageURL: "https://example.com/a.png",
		BasePriceCents: 1000, MinIncrementCents: 100, DurationSeconds: 60,
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auctions", bytes.NewReader(body))
	req = withAuthUser(req, domain.AuthUser{ID: uuid.New(), Username: "seller"})
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	if len(spawner.spawned) != 1 {
		t.Fatalf("expected Spawn to be called exactly once, got %d calls", len(spawner.spawned))
	}
	if creator.created == nil {
		t.Fatal("expected Create to be called")
	}
}

func TestAuctionHandler_Create_RejectsInvalidInput(t *testing.T) {
	h := NewAuctionHandler(&fakeCreator{}, &fakeLister{}, &fakeSpawner{}, &fakeSnapshotter{}, &fakeBidPlacer{}, &fakeBidLister{}, &fakeUserGetter{})

	reqBody := createAuctionRequest{Title: "ab", ImageURL: "", BasePriceCents: -1, MinIncrementCents: 0, DurationSeconds: 0}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auctions", bytes.NewReader(body))
	req = withAuthUser(req, domain.AuthUser{ID: uuid.New(), Username: "seller"})
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
