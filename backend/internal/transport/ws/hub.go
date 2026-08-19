// Package ws implements the WebSocket transport: independent broadcast
// rooms per auction ID. Hub implements domain.EventPublisher, which is how
// the auction engine stays decoupled from this package entirely (the
// engine holds only the interface; internal/auction never imports ws).
package ws

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

const clientSendBuffer = 64

type envelope struct {
	Type      domain.EventType `json:"type"`
	AuctionID string           `json:"auction_id"`
	TS        int64            `json:"ts"`
	Data      any              `json:"data"`
}

// Hub fans out events to per-auction rooms of WebSocket clients.
//
// Its sync.RWMutex protects only the plain client-set maps -- no
// event-ordering invariant applies to "who is subscribed", mirroring the
// same channels-vs-mutex split as auction.Manager's room registry (see
// that package's doc comment for the full argument).
type Hub struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]map[*Client]struct{}
	log     *slog.Logger
}

func NewHub(log *slog.Logger) *Hub {
	return &Hub{clients: make(map[uuid.UUID]map[*Client]struct{}), log: log}
}

func (h *Hub) join(auctionID uuid.UUID, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[auctionID] == nil {
		h.clients[auctionID] = make(map[*Client]struct{})
	}
	h.clients[auctionID][c] = struct{}{}
}

func (h *Hub) leave(auctionID uuid.UUID, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[auctionID], c)
	if len(h.clients[auctionID]) == 0 {
		delete(h.clients, auctionID)
	}
}

// Publish implements domain.EventPublisher. CONTRACT: must not block. The
// envelope is marshaled once, then fanned out with a non-blocking send per
// client; a client whose buffer is full is disconnected rather than
// allowed to stall the broadcast -- "we drop a slow spectator before we
// delay an auction."
func (h *Hub) Publish(ev domain.Event) {
	payload, err := json.Marshal(envelope{
		Type:      ev.Type,
		AuctionID: ev.AuctionID.String(),
		TS:        time.Now().UTC().UnixMilli(),
		Data:      eventDataJSON(ev),
	})
	if err != nil {
		h.log.Error("failed to marshal ws event", "err", err, "type", ev.Type)
		return
	}

	h.mu.RLock()
	clients := h.clients[ev.AuctionID]
	targets := make([]*Client, 0, len(clients))
	for c := range clients {
		targets = append(targets, c)
	}
	h.mu.RUnlock()

	for _, c := range targets {
		select {
		case c.send <- payload:
		default:
			h.kill(ev.AuctionID, c)
		}
	}
}

// sendDirect delivers a single pre-marshaled payload to one client (used
// for the snapshot-on-connect message, which is per-client and not a
// broadcast).
func (h *Hub) sendDirect(c *Client, payload []byte) {
	select {
	case c.send <- payload:
	default:
		h.kill(c.auctionID, c)
	}
}

func (h *Hub) kill(auctionID uuid.UUID, c *Client) {
	h.leave(auctionID, c)
	c.closeConn()
}
