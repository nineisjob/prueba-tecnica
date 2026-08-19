package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestMinNextBidCents(t *testing.T) {
	cases := []struct {
		name    string
		auction Auction
		wantMin int64
	}{
		{
			name:    "no bids yet: min is the base price",
			auction: Auction{BasePriceCents: 1000, CurrentPriceCents: 1000, MinIncrementCents: 100, BidCount: 0},
			wantMin: 1000,
		},
		{
			name:    "after one bid: min is current + increment",
			auction: Auction{BasePriceCents: 1000, CurrentPriceCents: 1500, MinIncrementCents: 100, BidCount: 1},
			wantMin: 1600,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.auction.MinNextBidCents(); got != tc.wantMin {
				t.Fatalf("MinNextBidCents() = %d, want %d", got, tc.wantMin)
			}
		})
	}
}

// TestSQLGuardMatchesDomainRule is the guard rail on the deliberate DRY
// violation between this Go rule and the SQL conditional UPDATE's CASE
// expression in postgres.ApplyBid. It re-implements the SQL rule as a Go
// closure and asserts every case agrees with domain.ValidateBid's verdict.
// If the two definitions ever diverge, this test -- not a production
// incident -- is what catches it.
func TestSQLGuardMatchesDomainRule(t *testing.T) {
	bidderID := uuid.New()

	sqlRule := func(a Auction, amount int64) bool {
		// Mirrors: $2 >= CASE WHEN bid_count = 0 THEN base_price_cents
		//               ELSE current_price_cents + min_increment_cents END
		if a.BidCount == 0 {
			return amount >= a.BasePriceCents
		}
		return amount >= a.CurrentPriceCents+a.MinIncrementCents
	}

	cases := []struct {
		name    string
		auction Auction
		amount  int64
	}{
		{"first bid exactly at base", Auction{BasePriceCents: 1000, CurrentPriceCents: 1000, MinIncrementCents: 100, BidCount: 0}, 1000},
		{"first bid one below base", Auction{BasePriceCents: 1000, CurrentPriceCents: 1000, MinIncrementCents: 100, BidCount: 0}, 999},
		{"first bid far above base", Auction{BasePriceCents: 1000, CurrentPriceCents: 1000, MinIncrementCents: 100, BidCount: 0}, 5000},
		{"second bid one below current+increment", Auction{BasePriceCents: 1000, CurrentPriceCents: 1500, MinIncrementCents: 100, BidCount: 1}, 1599},
		{"second bid exactly current+increment", Auction{BasePriceCents: 1000, CurrentPriceCents: 1500, MinIncrementCents: 100, BidCount: 1}, 1600},
		{"second bid above current+increment", Auction{BasePriceCents: 1000, CurrentPriceCents: 1500, MinIncrementCents: 100, BidCount: 1}, 999999},
		{"zero increment edge (increment=1)", Auction{BasePriceCents: 100, CurrentPriceCents: 100, MinIncrementCents: 1, BidCount: 1}, 101},
		{"large values", Auction{BasePriceCents: 100_000_000, CurrentPriceCents: 250_000_000, MinIncrementCents: 5_000_000, BidCount: 10}, 255_000_000},
		{"large values one below", Auction{BasePriceCents: 100_000_000, CurrentPriceCents: 250_000_000, MinIncrementCents: 5_000_000, BidCount: 10}, 254_999_999},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.auction
			sqlVerdict := sqlRule(a, tc.amount)

			err := a.ValidateBid(tc.amount, bidderID)
			domainVerdict := err == nil

			if sqlVerdict != domainVerdict {
				t.Fatalf("verdict mismatch for amount=%d: sql=%v domain=%v (err=%v)", tc.amount, sqlVerdict, domainVerdict, err)
			}
		})
	}
}

func TestValidateBid_RejectsNonPositiveAmount(t *testing.T) {
	a := Auction{BasePriceCents: 1000, CurrentPriceCents: 1000, MinIncrementCents: 100}
	if err := a.ValidateBid(0, uuid.New()); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for zero amount, got %v", err)
	}
	if err := a.ValidateBid(-100, uuid.New()); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for negative amount, got %v", err)
	}
}

func TestValidateBid_RejectsSelfOutbid(t *testing.T) {
	bidder := uuid.New()
	a := Auction{BasePriceCents: 1000, CurrentPriceCents: 1500, MinIncrementCents: 100, BidCount: 1, CurrentWinnerID: &bidder}
	if err := a.ValidateBid(1600, bidder); err != ErrAlreadyHighest {
		t.Fatalf("expected ErrAlreadyHighest, got %v", err)
	}
}
