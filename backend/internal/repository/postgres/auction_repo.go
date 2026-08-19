package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/geferson/bidcraft/backend/internal/domain"
)

func (r *AuctionRepo) Create(ctx context.Context, a *domain.Auction) error {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO auctions (
			seller_id, title, description, image_url,
			base_price_cents, current_price_cents, min_increment_cents,
			status, starts_at, ends_at
		) VALUES ($1, $2, $3, $4, $5, $5, $6, $7, $8, $9)
		RETURNING `+auctionColumns,
		a.SellerID, a.Title, a.Description, a.ImageURL,
		a.BasePriceCents, a.MinIncrementCents,
		a.Status, a.StartsAt, a.EndsAt,
	)
	created, err := scanAuction(row)
	if err != nil {
		return err
	}
	*a = created
	return nil
}

func (r *AuctionRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Auction, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+auctionColumns+` FROM auctions WHERE id = $1`, id)
	a, err := scanAuction(row)
	if err != nil {
		if isNoRows(err) {
			return nil, domain.ErrAuctionNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (r *AuctionRepo) List(ctx context.Context, f domain.AuctionFilter) ([]domain.Auction, error) {
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := `SELECT ` + auctionColumns + ` FROM auctions`
	args := []any{}
	switch f.Status {
	case "", "all":
		query += ` ORDER BY created_at DESC LIMIT $1 OFFSET $2`
		args = append(args, limit, f.Offset)
	case "active", "created", "finished":
		query += ` WHERE status = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
		args = append(args, f.Status, limit, f.Offset)
	default:
		return nil, domain.ErrInvalidInput
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Auction
	for rows.Next() {
		a, err := scanAuction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListOpen returns every auction not yet finished, used by Manager.Recover
// to re-arm room actors on process restart.
func (r *AuctionRepo) ListOpen(ctx context.Context) ([]domain.Auction, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+auctionColumns+` FROM auctions WHERE status IN ('created', 'active')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Auction
	for rows.Next() {
		a, err := scanAuction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *AuctionRepo) Activate(ctx context.Context, id uuid.UUID, at time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE auctions SET status = 'active', updated_at = $2
		WHERE id = $1 AND status = 'created'`, id, at)
	return err
}

// ApplyBid is Layer 3 of the concurrency design: a single conditional
// UPDATE re-expresses the bid rule independently of the Go-side check in
// domain.Auction.ValidateBid / the room actor's Layer 2 guard, so
// correctness does not rely solely on this process's in-memory state (a
// second replica, a restart, or a bug upstream all degrade gracefully to
// "the database is the tiebreaker" rather than corrupting data).
//
// Why a conditional UPDATE instead of SELECT ... FOR UPDATE: FOR UPDATE
// holds a row lock across at least two network round-trips, and the
// accept/reject logic would then live in Go *between* those round-trips --
// the SQL layer stops being an independent check and becomes just a lock.
// A single UPDATE is atomic in one statement.
//
// Why not SERIALIZABLE: it forces a retry loop on error code 40001 in every
// caller, real complexity for a guarantee this single-row conditional
// UPDATE already provides. Rejected on KISS.
//
// The duplication of the bid rule (Go AND SQL) is a deliberate DRY
// violation, pinned by domain.TestSQLGuardMatchesDomainRule so the two
// definitions can never silently diverge.
func (r *AuctionRepo) ApplyBid(ctx context.Context, p domain.ApplyBidParams) (domain.ApplyBidResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.ApplyBidResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op if committed

	row := tx.QueryRow(ctx, `
		UPDATE auctions SET
			current_price_cents = $2,
			current_winner_id   = $3,
			bid_count           = bid_count + 1,
			updated_at          = now()
		WHERE id = $1
		  AND status = 'active'
		  AND now() < ends_at
		  AND $2 >= CASE WHEN bid_count = 0
		                 THEN base_price_cents
		                 ELSE current_price_cents + min_increment_cents END
		RETURNING `+auctionColumns,
		p.AuctionID, p.AmountCents, p.BidderID,
	)
	updated, err := scanAuction(row)
	if err != nil {
		if isNoRows(err) {
			// RowsAffected == 0: the DB disagrees with the caller's belief
			// that this bid should win. Classify why for a precise error.
			return domain.ApplyBidResult{}, r.classifyBidRejection(ctx, tx, p)
		}
		return domain.ApplyBidResult{}, err
	}

	var bid domain.Bid
	err = tx.QueryRow(ctx, `
		INSERT INTO bids (auction_id, bidder_id, amount_cents, placed_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, seq, placed_at`,
		p.AuctionID, p.BidderID, p.AmountCents, p.PlacedAt,
	).Scan(&bid.ID, &bid.Seq, &bid.PlacedAt)
	if err != nil {
		return domain.ApplyBidResult{}, classifyWriteErr(err)
	}
	bid.AuctionID = p.AuctionID
	bid.BidderID = p.BidderID
	bid.BidderName = p.BidderName
	bid.AmountCents = p.AmountCents

	if err := tx.Commit(ctx); err != nil {
		return domain.ApplyBidResult{}, err
	}

	return domain.ApplyBidResult{Auction: updated, Bid: bid}, nil
}

// classifyBidRejection re-reads the current row (outside the failed
// UPDATE's WHERE clause) to give the caller a precise sentinel instead of a
// generic "rejected".
//
// It MUST run the read on the same tx (not r.pool): under saturated-pool
// load, acquiring a second connection from r.pool while this goroutine
// still holds tx's connection is a classic connection-pool self-deadlock
// -- every goroutine that reaches this rejection path would hold one
// connection and block forever waiting for a second one that can never
// free up, because every other waiter is stuck the same way. Reusing tx
// costs no extra connection at all.
func (r *AuctionRepo) classifyBidRejection(ctx context.Context, tx pgx.Tx, p domain.ApplyBidParams) error {
	row := tx.QueryRow(ctx, `SELECT `+auctionColumns+` FROM auctions WHERE id = $1`, p.AuctionID)
	a, err := scanAuction(row)
	if err != nil {
		if isNoRows(err) {
			return domain.ErrAuctionNotFound
		}
		return err
	}
	if a.Status != domain.StatusActive {
		return domain.ErrAuctionEnded
	}
	if p.AmountCents < a.MinNextBidCents() {
		return domain.ErrBidTooLow
	}
	return errors.New("bid rejected: auction state changed concurrently")
}

func (r *AuctionRepo) Close(ctx context.Context, id uuid.UUID, at time.Time) (domain.Auction, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE auctions SET status = 'finished', closed_at = $2, updated_at = $2
		WHERE id = $1 AND status = 'active'
		RETURNING `+auctionColumns, id, at)
	closed, err := scanAuction(row)
	if err != nil {
		if isNoRows(err) {
			// Idempotent: a second Close call (e.g. a repeated Recover
			// pass) affects zero rows rather than erroring. Return the
			// current (already finished) state.
			a, gerr := r.GetByID(ctx, id)
			if gerr != nil {
				return domain.Auction{}, gerr
			}
			return *a, nil
		}
		return domain.Auction{}, err
	}
	return closed, nil
}
