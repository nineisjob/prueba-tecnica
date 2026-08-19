import { useEffect, useRef } from 'react';
import { useAuth } from '../../hooks/useAuth';
import { useServerTime } from '../../hooks/useServerTime';
import { useCountdown } from '../../hooks/useCountdown';
import { useAuctionRoom } from '../../hooks/useAuctionRoom';
import { useToasts } from '../../hooks/useToasts';
import { api } from '../../lib/api';
import { formatCents } from '../../lib/money';
import type { AuctionDetail, Bid, WsBidderOutbidData, WsEnvelope } from '../../lib/types';
import Countdown from './Countdown';
import PricePanel from './PricePanel';
import BidForm from './BidForm';
import BidFeed from './BidFeed';
import ConnectionBadge from './ConnectionBadge';
import ToastHost from './ToastHost';
import DemoUserSwitcher from './DemoUserSwitcher';

interface Props {
  auction: AuctionDetail;
  initialBids: Bid[];
}

const CLOSE_FALLBACK_DELAY_MS = 3000;

export default function LiveBiddingRoom({ auction: initialAuction, initialBids }: Props) {
  const { user, isAuthenticated, login, logout } = useAuth();
  const { offsetMs } = useServerTime();
  const toasts = useToasts();

  const handleWsEvent = (ev: WsEnvelope) => {
    if (ev.type === 'bidder.outbid') {
      const data = ev.data as WsBidderOutbidData;
      if (user && data.outbid_user_id === user.id) {
        toasts.info(`${data.by_username} te superó — nuevo precio ${formatCents(data.new_price_cents)}`);
      }
    }
  };

  const { auction, bids, lastSeq, connection, applyOptimisticBid, hydrate } = useAuctionRoom(
    initialAuction,
    initialBids,
    handleWsEvent,
  );

  const { expired } = useCountdown(auction.ends_at, offsetMs);
  const closing = expired && auction.status === 'active';

  // The server is the only authority on who won -- the countdown reaching
  // zero is never treated as "closed" by itself. If the WS is down at the
  // exact moment of expiry, this fallback re-fetches once via REST rather
  // than leaving the UI stuck on "Cerrando…" forever.
  const fallbackFired = useRef(false);
  useEffect(() => {
    if (!closing) {
      fallbackFired.current = false;
      return;
    }
    if (fallbackFired.current) return;
    fallbackFired.current = true;
    const t = setTimeout(() => {
      api.getAuction(auction.id).then(hydrate).catch(() => {});
    }, CLOSE_FALLBACK_DELAY_MS);
    return () => clearTimeout(t);
  }, [closing, auction.id, hydrate]);

  return (
    <div className="grid gap-8 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
      <div className="space-y-6">
        <img
          src={auction.image_url}
          alt={auction.title}
          className="aspect-video w-full rounded-xl border border-slate-200 object-cover dark:border-slate-800"
        />
        <div>
          <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-50">{auction.title}</h1>
          <p className="mt-2 text-slate-600 dark:text-slate-400">{auction.description}</p>
          <p className="mt-1 text-xs text-slate-400">Vendedor: {auction.seller.username}</p>
        </div>
        <div>
          <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-slate-500">Feed en vivo</h2>
          <BidFeed bids={bids} currentUserId={user?.id ?? null} />
        </div>
      </div>

      <div className="space-y-4 lg:sticky lg:top-6 lg:self-start">
        <div className="flex items-center justify-between">
          <ConnectionBadge status={connection} />
          <DemoUserSwitcher currentUsername={user?.username ?? null} onLogin={login} onLogout={logout} />
        </div>

        {auction.status === 'finished' ? (
          <div className="rounded-xl border border-slate-200 bg-slate-50 p-5 dark:border-slate-800 dark:bg-slate-900">
            <div className="text-sm font-semibold text-slate-500">Subasta finalizada</div>
            <div className="mt-1 text-lg">
              {auction.current_winner ? (
                <>
                  Ganador: <strong>{auction.current_winner.username}</strong>
                </>
              ) : (
                'Sin pujas — sin ganador'
              )}
            </div>
          </div>
        ) : (
          <Countdown endsAt={auction.ends_at} offsetMs={offsetMs} closing={closing} />
        )}

        <PricePanel auction={auction} lastSeq={lastSeq} currentUserId={user?.id ?? null} />

        <BidForm
          auction={{ ...auction, status: closing ? 'finished' : auction.status }}
          isAuthenticated={isAuthenticated}
          onPlaced={applyOptimisticBid}
          onToastSuccess={toasts.success}
          onToastError={toasts.error}
        />
      </div>

      <ToastHost toasts={toasts.toasts} dismiss={toasts.dismiss} />
    </div>
  );
}
