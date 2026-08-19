package ws

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestHub_PublishNeverBlocks_SlowClientIsDropped is the empirical proof of
// the EventPublisher contract (Publish must not block): a client whose
// send buffer is full must be disconnected rather than allowed to stall
// the broadcast to everyone else -- "we drop a slow spectator before we
// delay an auction."
func TestHub_PublishNeverBlocks_SlowClientIsDropped(t *testing.T) {
	hub := NewHub(testLogger())
	auctionID := uuid.New()

	// A client whose buffer we fill manually to simulate "slow consumer":
	// nothing ever drains client.send in this test.
	slow := &Client{send: make(chan []byte, clientSendBuffer), auctionID: auctionID}
	hub.join(auctionID, slow)

	// Fill the buffer completely so the next Publish's non-blocking send fails.
	for i := 0; i < clientSendBuffer; i++ {
		slow.send <- []byte("filler")
	}

	done := make(chan struct{})
	go func() {
		hub.Publish(domain.Event{Type: domain.EventBidPlaced, AuctionID: auctionID, Data: map[string]int{"x": 1}})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked for over a second on a slow client -- it must never block")
	}

	hub.mu.RLock()
	_, stillPresent := hub.clients[auctionID][slow]
	hub.mu.RUnlock()
	if stillPresent {
		t.Fatal("slow client should have been removed from the room after a full buffer")
	}
}

// TestHub_PublishFansOutToAllClientsInRoom verifies broadcast reaches every
// subscriber of the target auction and nobody else.
func TestHub_PublishFansOutToAllClientsInRoom(t *testing.T) {
	hub := NewHub(testLogger())
	auctionA := uuid.New()
	auctionB := uuid.New()

	c1 := &Client{send: make(chan []byte, clientSendBuffer), auctionID: auctionA}
	c2 := &Client{send: make(chan []byte, clientSendBuffer), auctionID: auctionA}
	other := &Client{send: make(chan []byte, clientSendBuffer), auctionID: auctionB}

	hub.join(auctionA, c1)
	hub.join(auctionA, c2)
	hub.join(auctionB, other)

	hub.Publish(domain.Event{Type: domain.EventBidPlaced, AuctionID: auctionA, Data: map[string]int{"x": 1}})

	for _, c := range []*Client{c1, c2} {
		select {
		case <-c.send:
		case <-time.After(time.Second):
			t.Fatal("expected subscriber of auctionA to receive the broadcast")
		}
	}
	select {
	case <-other.send:
		t.Fatal("client subscribed to a different auction should not receive the broadcast")
	default:
	}
}

func TestHub_JoinLeaveCleansUpEmptyRooms(t *testing.T) {
	hub := NewHub(testLogger())
	auctionID := uuid.New()
	c := &Client{send: make(chan []byte, clientSendBuffer), auctionID: auctionID}

	hub.join(auctionID, c)
	hub.leave(auctionID, c)

	hub.mu.RLock()
	_, exists := hub.clients[auctionID]
	hub.mu.RUnlock()
	if exists {
		t.Fatal("expected the room map entry to be removed once its last client leaves")
	}
}
