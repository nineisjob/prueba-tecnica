import { useEffect, useRef, useState } from 'react';
import { formatCents } from '../../lib/money';
import type { AuctionDetail } from '../../lib/types';

interface Props {
  auction: AuctionDetail;
  lastSeq: number;
  currentUserId: string | null;
}

export default function PricePanel({ auction, lastSeq, currentUserId }: Props) {
  const [flash, setFlash] = useState(false);
  const prevSeq = useRef(lastSeq);

  useEffect(() => {
    if (lastSeq !== prevSeq.current) {
      prevSeq.current = lastSeq;
      setFlash(true);
      const t = setTimeout(() => setFlash(false), 600);
      return () => clearTimeout(t);
    }
  }, [lastSeq]);

  const isYouWinning = auction.current_winner?.id === currentUserId && currentUserId !== null;

  return (
    <div
      className={`rounded-xl border p-5 transition-colors duration-500 ${
        flash ? 'border-emerald-400 bg-emerald-50 dark:bg-emerald-950/40' : 'border-slate-200 dark:border-slate-800'
      }`}
    >
      <div className="text-sm text-slate-500 dark:text-slate-400">Precio actual</div>
      <div className="mt-1 text-4xl font-bold tabular-nums text-slate-900 dark:text-slate-50">
        {formatCents(auction.current_price_cents)}
      </div>

      <div className="mt-3 text-sm">
        {auction.current_winner ? (
          <span className={isYouWinning ? 'font-semibold text-emerald-600 dark:text-emerald-400' : 'text-slate-600 dark:text-slate-300'}>
            {isYouWinning ? 'Vas ganando' : `Va ganando: ${auction.current_winner.username}`}
          </span>
        ) : (
          <span className="text-slate-500">Sin pujas todav&iacute;a</span>
        )}
      </div>

      {auction.status === 'active' && (
        <div className="mt-2 text-xs text-slate-400">
          Puja m&iacute;nima siguiente: {formatCents(auction.min_next_bid_cents)}
        </div>
      )}
    </div>
  );
}
