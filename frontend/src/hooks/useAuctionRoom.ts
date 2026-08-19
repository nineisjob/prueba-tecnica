import { useCallback, useReducer, useRef } from 'react';
import { useWebSocket, type ConnectionStatus } from './useWebSocket';
import { getWsUrl } from '../lib/runtimeConfig';
import type {
  AuctionDetail,
  Bid,
  WsAuctionClosedData,
  WsBidderOutbidData,
  WsBidPlacedData,
  WsEnvelope,
  WsSnapshotData,
} from '../lib/types';

interface RoomState {
  auction: AuctionDetail;
  bids: Bid[];
  lastSeq: number;
}

type RoomAction =
  | { type: 'snapshot'; data: WsSnapshotData }
  | { type: 'bid.placed'; data: WsBidPlacedData }
  | { type: 'bidder.outbid'; data: WsBidderOutbidData }
  | { type: 'auction.closed'; data: WsAuctionClosedData }
  | { type: 'auction.started' }
  | { type: 'hydrate'; auction: AuctionDetail };

const MAX_FEED = 100;

function reducer(state: RoomState, action: RoomAction): RoomState {
  switch (action.type) {
    case 'snapshot': {
      const { auction, bids } = action.data;
      const lastSeq = bids.length > 0 ? bids[bids.length - 1].seq : 0;
      return {
        auction: { ...state.auction, ...auction, current_winner: auction.current_winner },
        bids: bids.map((b) => ({ ...b, auction_id: state.auction.id })),
        lastSeq: Math.max(state.lastSeq, lastSeq),
      };
    }
    case 'bid.placed': {
      const { bid, auction } = action.data;
      // Dedupe against the bidder's own optimistic update from the POST
      // response, which can arrive before or after this WS event.
      if (bid.seq <= state.lastSeq) return state;
      const nextBids = [...state.bids, { ...bid, auction_id: state.auction.id }].slice(-MAX_FEED);
      return {
        auction: {
          ...state.auction,
          current_price_cents: auction.current_price_cents,
          min_next_bid_cents: auction.min_next_bid_cents,
          current_winner: auction.current_winner,
          bid_count: auction.bid_count,
        },
        bids: nextBids,
        lastSeq: bid.seq,
      };
    }
    case 'auction.started':
      return { ...state, auction: { ...state.auction, status: 'active' } };
    case 'auction.closed':
      return {
        ...state,
        auction: {
          ...state.auction,
          status: 'finished',
          current_price_cents: action.data.final_price_cents,
          current_winner: action.data.winner,
          bid_count: action.data.bid_count,
          closed_at: action.data.closed_at,
        },
      };
    case 'bidder.outbid':
      return state; // no state change; consumers hook into onEvent for toasts
    case 'hydrate':
      // Fallback path when the WS is down at the exact moment the auction
      // expires: LiveBiddingRoom re-fetches via REST after a short grace
      // period rather than trusting a local countdown to declare a winner
      // -- the server is the only authority on who won.
      return { ...state, auction: { ...state.auction, ...action.auction } };
    default:
      return state;
  }
}

export interface UseAuctionRoomResult {
  auction: AuctionDetail;
  bids: Bid[];
  lastSeq: number;
  connection: ConnectionStatus;
  applyOptimisticBid: (res: { bid: Bid; auction: AuctionDetail }) => void;
  hydrate: (auction: AuctionDetail) => void;
}

export function useAuctionRoom(
  initial: AuctionDetail,
  initialBids: Bid[],
  onEvent?: (ev: WsEnvelope) => void,
): UseAuctionRoomResult {
  const initialLastSeq = initialBids.length > 0 ? initialBids[initialBids.length - 1].seq : 0;
  const [state, dispatch] = useReducer(reducer, {
    auction: initial,
    bids: initialBids,
    lastSeq: initialLastSeq,
  });

  const onEventRef = useRef(onEvent);
  onEventRef.current = onEvent;

  const handleEvent = useCallback((ev: WsEnvelope) => {
    onEventRef.current?.(ev);
    switch (ev.type) {
      case 'snapshot':
        dispatch({ type: 'snapshot', data: ev.data as WsSnapshotData });
        break;
      case 'bid.placed':
        dispatch({ type: 'bid.placed', data: ev.data as WsBidPlacedData });
        break;
      case 'bidder.outbid':
        dispatch({ type: 'bidder.outbid', data: ev.data as WsBidderOutbidData });
        break;
      case 'auction.started':
        dispatch({ type: 'auction.started' });
        break;
      case 'auction.closed':
        dispatch({ type: 'auction.closed', data: ev.data as WsAuctionClosedData });
        break;
    }
  }, []);

  const { status } = useWebSocket<WsEnvelope>(getWsUrl() + `/api/v1/auctions/${initial.id}/ws`, {
    onEvent: handleEvent,
  });

  // Lets BidForm reflect its own successful POST instantly, without
  // waiting for the WS round-trip. The subsequent bid.placed WS event for
  // the same bid is deduped by seq in the reducer above.
  const applyOptimisticBid = useCallback((res: { bid: Bid; auction: AuctionDetail }) => {
    dispatch({
      type: 'bid.placed',
      data: {
        bid: res.bid,
        auction: {
          current_price_cents: res.auction.current_price_cents,
          min_next_bid_cents: res.auction.min_next_bid_cents,
          current_winner: res.auction.current_winner,
          bid_count: res.auction.bid_count,
        },
      },
    });
  }, []);

  const hydrate = useCallback((a: AuctionDetail) => dispatch({ type: 'hydrate', auction: a }), []);

  return { auction: state.auction, bids: state.bids, lastSeq: state.lastSeq, connection: status, applyOptimisticBid, hydrate };
}
