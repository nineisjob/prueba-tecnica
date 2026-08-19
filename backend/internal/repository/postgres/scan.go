package postgres

import (
	"time"

	"github.com/google/uuid"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

// rowScanner is satisfied by both pgx.Row (QueryRow) and pgx.Rows (Query),
// so auctionColumns/scanAuction can back both a single-row GetByID and a
// multi-row List without duplicating the column list.
type rowScanner interface {
	Scan(dest ...any) error
}

const auctionColumns = `
	id, seller_id, title, description, image_url,
	base_price_cents, current_price_cents, min_increment_cents,
	current_winner_id, bid_count, status,
	starts_at, ends_at, closed_at, created_at, updated_at`

func scanAuction(row rowScanner) (domain.Auction, error) {
	var a domain.Auction
	var winnerID *uuid.UUID
	var closedAt *time.Time

	err := row.Scan(
		&a.ID, &a.SellerID, &a.Title, &a.Description, &a.ImageURL,
		&a.BasePriceCents, &a.CurrentPriceCents, &a.MinIncrementCents,
		&winnerID, &a.BidCount, &a.Status,
		&a.StartsAt, &a.EndsAt, &closedAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return domain.Auction{}, err
	}
	a.CurrentWinnerID = winnerID
	a.ClosedAt = closedAt
	return a, nil
}
