// Mirrors backend/internal/transport/http/dto.go. Kept as one file so the
// wire contract has a single place to update on either side.

export type AuctionStatus = 'created' | 'active' | 'finished';

export interface UserRef {
  id: string;
  username: string;
}

export interface AuctionDetail {
  id: string;
  title: string;
  description: string;
  image_url: string;
  status: AuctionStatus;
  base_price_cents: number;
  current_price_cents: number;
  min_increment_cents: number;
  min_next_bid_cents: number;
  bid_count: number;
  current_winner: UserRef | null;
  seller: UserRef;
  starts_at: string;
  ends_at: string;
  closed_at: string | null;
  server_time_ms: number;
}

export interface Bid {
  id: string;
  seq: number;
  auction_id: string;
  bidder_id: string;
  bidder_name: string;
  amount_cents: number;
  placed_at: string;
}

export interface AuctionListResponse {
  data: AuctionDetail[];
  server_time_ms: number;
}

export interface BidListResponse {
  data: Bid[];
  server_time_ms: number;
}

export interface PlaceBidResponse {
  bid: Bid;
  auction: AuctionDetail;
}

export interface User {
  id: string;
  username: string;
  email: string;
}

export interface AuthResponse {
  token: string;
  expires_at: string;
  user: User;
}

export interface ApiErrorBody {
  error: {
    code: string;
    message: string;
    details?: Record<string, unknown>;
  };
  request_id?: string;
}

// --- WebSocket envelope ---

export type WsEventType =
  | 'snapshot'
  | 'auction.started'
  | 'bid.placed'
  | 'bidder.outbid'
  | 'auction.closed';

export interface WsEnvelope<T = unknown> {
  type: WsEventType;
  auction_id: string;
  ts: number;
  data: T;
}

export interface WsSnapshotData {
  auction: {
    id: string;
    title: string;
    status: AuctionStatus;
    current_price_cents: number;
    min_next_bid_cents: number;
    bid_count: number;
    current_winner: UserRef | null;
    starts_at: string;
    ends_at: string;
    closed_at: string | null;
  };
  bids: Array<{
    id: string;
    seq: number;
    bidder_id: string;
    bidder_name: string;
    amount_cents: number;
    placed_at: string;
  }>;
}

export interface WsBidPlacedData {
  bid: {
    id: string;
    seq: number;
    amount_cents: number;
    bidder_id: string;
    bidder_name: string;
    placed_at: string;
  };
  auction: {
    current_price_cents: number;
    min_next_bid_cents: number;
    current_winner: UserRef | null;
    bid_count: number;
  };
}

export interface WsBidderOutbidData {
  outbid_user_id: string;
  new_price_cents: number;
  by_username: string;
}

export interface WsAuctionStartedData {
  auction_id: string;
  started_at: string;
  ends_at: string;
}

export interface WsAuctionClosedData {
  auction_id: string;
  final_price_cents: number;
  winner: UserRef | null;
  bid_count: number;
  closed_at: string;
}
