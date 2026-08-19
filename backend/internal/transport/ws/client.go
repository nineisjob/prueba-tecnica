package ws

import (
	"context"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

const (
	pingInterval = 30 * time.Second
	writeTimeout = 10 * time.Second
)

// Client wraps one WebSocket connection. The socket is anonymous and
// read-only by design (see handler.go's doc comment): writePump is the
// only thing that ever calls conn.Write; incoming frames are drained by
// conn.CloseRead so pings/pongs/close handshakes still work without a
// dedicated read loop or any application-level message handling.
type Client struct {
	conn      *websocket.Conn
	send      chan []byte
	auctionID uuid.UUID
	closeOnce sync.Once
}

func newClient(conn *websocket.Conn, auctionID uuid.UUID) *Client {
	return &Client{conn: conn, send: make(chan []byte, clientSendBuffer), auctionID: auctionID}
}

// closeConn is idempotent and safe to call from multiple goroutines (the
// hub may call it concurrently from two Publish calls racing to detect the
// same slow client -- see hub.go's kill).
func (c *Client) closeConn() {
	c.closeOnce.Do(func() {
		if c.conn != nil { // nil in unit tests that exercise the hub without a real socket
			c.conn.Close(websocket.StatusNormalClosure, "closing")
		}
	})
}

// writePump owns the connection's write side exclusively (a websocket.Conn
// must not be written from multiple goroutines concurrently). ctx is the
// context returned by conn.CloseRead, so it is cancelled the moment the
// client disconnects or the connection errors.
func (c *Client) writePump(ctx context.Context) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	defer c.closeConn()

	for {
		select {
		case payload, ok := <-c.send:
			if !ok {
				return
			}
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.conn.Write(wctx, websocket.MessageText, payload)
			cancel()
			if err != nil {
				return
			}
		case <-ticker.C:
			pctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
