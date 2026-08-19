import { useEffect, useRef, useState } from 'react';
import { formatCents } from '../../lib/money';
import type { Bid } from '../../lib/types';

interface Props {
  bids: Bid[];
  currentUserId: string | null;
}

const NEAR_BOTTOM_PX = 40;

/** Chronological (oldest -> newest) feed that auto-scrolls to the newest
 * bid, but only while the viewer is already at the bottom -- scrolling up
 * to read history stops the auto-scroll and shows a "N new bids" pill
 * instead of yanking the view back down. */
export default function BidFeed({ bids, currentUserId }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [pinnedToBottom, setPinnedToBottom] = useState(true);
  const [hiddenNewCount, setHiddenNewCount] = useState(0);
  const prevLength = useRef(bids.length);

  const isNearBottom = () => {
    const el = containerRef.current;
    if (!el) return true;
    return el.scrollHeight - el.scrollTop - el.clientHeight < NEAR_BOTTOM_PX;
  };

  useEffect(() => {
    const grew = bids.length > prevLength.current;
    prevLength.current = bids.length;
    if (!grew) return;

    if (pinnedToBottom) {
      containerRef.current?.scrollTo({ top: containerRef.current.scrollHeight, behavior: 'smooth' });
    } else {
      setHiddenNewCount((n) => n + 1);
    }
  }, [bids.length, pinnedToBottom]);

  const handleScroll = () => {
    const atBottom = isNearBottom();
    setPinnedToBottom(atBottom);
    if (atBottom) setHiddenNewCount(0);
  };

  const jumpToBottom = () => {
    containerRef.current?.scrollTo({ top: containerRef.current.scrollHeight, behavior: 'smooth' });
    setPinnedToBottom(true);
    setHiddenNewCount(0);
  };

  return (
    <div className="relative">
      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="flex max-h-80 flex-col gap-1 overflow-y-auto rounded-xl border border-slate-200 p-3 dark:border-slate-800"
      >
        {bids.length === 0 && <div className="py-6 text-center text-sm text-slate-400">Sin pujas todav&iacute;a</div>}
        {bids.map((bid) => (
          <div
            key={bid.id}
            className={`flex items-center justify-between rounded-lg px-3 py-2 text-sm ${
              bid.bidder_id === currentUserId ? 'bg-indigo-50 dark:bg-indigo-950/40' : 'bg-slate-50 dark:bg-slate-900'
            }`}
          >
            <span className="font-medium text-slate-700 dark:text-slate-200">
              {bid.bidder_name}
              {bid.bidder_id === currentUserId && <span className="ml-1 text-xs text-indigo-500">(tú)</span>}
            </span>
            <span className="tabular-nums text-slate-900 dark:text-slate-50">{formatCents(bid.amount_cents)}</span>
          </div>
        ))}
      </div>

      {hiddenNewCount > 0 && (
        <button
          onClick={jumpToBottom}
          className="absolute bottom-2 left-1/2 -translate-x-1/2 rounded-full bg-indigo-600 px-3 py-1 text-xs font-medium text-white shadow-lg hover:bg-indigo-500"
        >
          {hiddenNewCount} nueva{hiddenNewCount > 1 ? 's' : ''} puja{hiddenNewCount > 1 ? 's' : ''} &darr;
        </button>
      )}
    </div>
  );
}
