package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

// AuctionSnapshotter and BidLister mirror the narrow interfaces used by
// transport/http (ISP) -- this package intentionally does not import
// transport/http to avoid coupling the two transports' wire formats.
type AuctionSnapshotter interface {
	Snapshot(ctx context.Context, auctionID uuid.UUID) (domain.Auction, error)
}

type BidLister interface {
	ListByAuction(ctx context.Context, auctionID uuid.UUID, limit int) ([]domain.Bid, error)
}

type UserGetter interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

// Handler upgrades /api/v1/auctions/{id}/ws connections.
//
// The socket is intentionally anonymous and read-only: browsers cannot set
// headers on the WebSocket constructor, so authenticating it would force a
// `?token=` query parameter -- which proxies log and browsers keep in
// history. Bids go over authenticated REST regardless; the feed carries
// only public data (usernames + amounts), and bidder.outbid carries
// outbid_user_id so each client can decide locally whether it applies to
// them. This is simultaneously the KISS and the more secure choice.
type Handler struct {
	hub      *Hub
	snapshot AuctionSnapshotter
	bids     BidLister
	users    UserGetter
	allowed  []string
	log      *slog.Logger
}

func NewHandler(hub *Hub, snap AuctionSnapshotter, bids BidLister, users UserGetter, corsOrigins string, log *slog.Logger) *Handler {
	var patterns []string
	for _, o := range strings.Split(corsOrigins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			patterns = append(patterns, hostOnly(o))
		}
	}
	return &Handler{hub: hub, snapshot: snap, bids: bids, users: users, allowed: patterns, log: log}
}

func hostOnly(origin string) string {
	o := strings.TrimPrefix(strings.TrimPrefix(origin, "http://"), "https://")
	return o
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	auctionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid auction id", http.StatusBadRequest)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.allowed})
	if err != nil {
		return // Accept already wrote the appropriate HTTP error response.
	}

	client := newClient(conn, auctionID)
	h.hub.join(auctionID, client)

	// conn.CloseRead drains and discards any client-sent frames (there are
	// none, by design) while transparently answering ping/pong and close
	// control frames -- exactly the read-loop a write-only server push
	// connection needs, without a bespoke read goroutine.
	ctx := conn.CloseRead(context.Background())

	h.sendSnapshot(ctx, client)

	client.writePump(ctx) // blocks until disconnect or a write error
	h.hub.leave(auctionID, client)
}

func (h *Handler) sendSnapshot(ctx context.Context, c *Client) {
	a, err := h.snapshot.Snapshot(ctx, c.auctionID)
	if err != nil {
		return // auction not found; client just won't receive a snapshot
	}
	bids, _ := h.bids.ListByAuction(ctx, c.auctionID, 20)

	payload, err := json.Marshal(envelope{
		Type:      domain.EventSnapshot,
		AuctionID: a.ID.String(),
		TS:        time.Now().UTC().UnixMilli(),
		Data: map[string]any{
			"auction": h.auctionSnapshotJSON(ctx, a),
			"bids":    bidsSnapshotJSON(bids),
		},
	})
	if err != nil {
		h.log.Error("failed to marshal snapshot", "err", err)
		return
	}
	h.hub.sendDirect(c, payload)
}

func (h *Handler) usernameFor(ctx context.Context, id uuid.UUID) string {
	u, err := h.users.GetByID(ctx, id)
	if err != nil {
		return ""
	}
	return u.Username
}

func (h *Handler) auctionSnapshotJSON(ctx context.Context, a domain.Auction) map[string]any {
	var winner map[string]string
	if a.CurrentWinnerID != nil {
		winner = map[string]string{"id": a.CurrentWinnerID.String(), "username": h.usernameFor(ctx, *a.CurrentWinnerID)}
	}
	return map[string]any{
		"id":                  a.ID.String(),
		"title":               a.Title,
		"status":              string(a.Status),
		"current_price_cents": a.CurrentPriceCents,
		"min_next_bid_cents":  a.MinNextBidCents(),
		"bid_count":           a.BidCount,
		"current_winner":      winner,
		"starts_at":           a.StartsAt,
		"ends_at":             a.EndsAt,
		"closed_at":           a.ClosedAt,
	}
}

func bidsSnapshotJSON(bids []domain.Bid) []map[string]any {
	out := make([]map[string]any, 0, len(bids))
	for _, b := range bids {
		out = append(out, map[string]any{
			"id": b.ID.String(), "seq": b.Seq, "bidder_id": b.BidderID.String(),
			"bidder_name": b.BidderName, "amount_cents": b.AmountCents, "placed_at": b.PlacedAt,
		})
	}
	return out
}
