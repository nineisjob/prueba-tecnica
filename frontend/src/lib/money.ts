// The backend stores money as integer cents (BIGINT) so accept/reject
// comparisons are exact -- see backend/internal/domain/auction.go. This
// module is the single place the frontend formats/parses that value, so
// the +$1 rounding-error class of bug can only ever exist in one spot.

export function formatCents(cents: number, currency = 'USD'): string {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency }).format(cents / 100);
}

/** Parses a user-typed currency string ("12.50", "$12.50") into integer cents. */
export function parseToCents(input: string): number | null {
  const cleaned = input.replace(/[^0-9.]/g, '');
  if (cleaned === '') return null;
  const value = Number.parseFloat(cleaned);
  if (Number.isNaN(value) || value < 0) return null;
  return Math.round(value * 100);
}
