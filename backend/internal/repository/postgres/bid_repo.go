package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

// ListByAuction returns bids in chronological order (ORDER BY seq ASC).
// `seq` is a Postgres identity column, which removes an entire class of
// same-millisecond ordering flakiness that sorting by placed_at alone would
// have under Windows' clock granularity.
func (r *BidRepo) ListByAuction(ctx context.Context, auctionID uuid.UUID, limit int) ([]domain.Bid, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT b.id, b.seq, b.auction_id, b.bidder_id, u.username, b.amount_cents, b.placed_at
		FROM bids b
		JOIN users u ON u.id = b.bidder_id
		WHERE b.auction_id = $1
		ORDER BY b.seq ASC
		LIMIT $2`, auctionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Bid
	for rows.Next() {
		var b domain.Bid
		if err := rows.Scan(&b.ID, &b.Seq, &b.AuctionID, &b.BidderID, &b.BidderName, &b.AmountCents, &b.PlacedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
