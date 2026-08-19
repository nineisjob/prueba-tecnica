import { useEffect, useState } from 'react';
import { api, ApiError } from '../../lib/api';
import { formatCents, parseToCents } from '../../lib/money';
import type { AuctionDetail, Bid } from '../../lib/types';

interface Props {
  auction: AuctionDetail;
  isAuthenticated: boolean;
  onPlaced: (res: { bid: Bid; auction: AuctionDetail }) => void;
  onToastSuccess: (msg: string) => void;
  onToastError: (msg: string) => void;
}

export default function BidForm({ auction, isAuthenticated, onPlaced, onToastSuccess, onToastError }: Props) {
  const [value, setValue] = useState(() => (auction.min_next_bid_cents / 100).toFixed(2));
  const [pending, setPending] = useState(false);

  useEffect(() => {
    setValue((auction.min_next_bid_cents / 100).toFixed(2));
  }, [auction.min_next_bid_cents]);

  const disabled = pending || auction.status !== 'active' || !isAuthenticated;

  const bumpBy = (increments: number) => {
    const next = auction.min_next_bid_cents + increments * auction.min_increment_cents - auction.min_increment_cents;
    setValue((next / 100).toFixed(2));
  };

  const submit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const cents = parseToCents(value);
    if (cents === null) {
      onToastError('Ingresa un importe válido');
      return;
    }
    setPending(true);
    try {
      const res = await api.placeBid(auction.id, cents);
      onPlaced(res);
      onToastSuccess(`Puja de ${formatCents(cents)} aceptada`);
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.code === 'BID_TOO_LOW' && err.details) {
          const min = err.details.min_next_bid_cents as number;
          setValue((min / 100).toFixed(2));
          onToastError(`Puja muy baja. Mínimo: ${formatCents(min)}`);
        } else if (err.code === 'ALREADY_HIGHEST_BIDDER') {
          onToastError('Ya vas ganando esta subasta');
        } else if (err.code === 'AUCTION_ENDED') {
          onToastError('La subasta ya finalizó');
        } else {
          onToastError(err.message);
        }
      } else {
        onToastError('Error de red al pujar');
      }
    } finally {
      setPending(false);
    }
  };

  return (
    <form onSubmit={submit} className="rounded-xl border border-slate-200 p-5 dark:border-slate-800">
      <label htmlFor="bid-amount" className="text-sm font-medium text-slate-700 dark:text-slate-300">
        Tu puja (USD)
      </label>
      <div className="mt-2 flex gap-2">
        <input
          id="bid-amount"
          type="text"
          inputMode="decimal"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          disabled={disabled}
          className="w-full rounded-lg border border-slate-300 px-3 py-2 text-lg tabular-nums disabled:opacity-50 dark:border-slate-700 dark:bg-slate-900"
        />
        <button
          type="submit"
          disabled={disabled}
          className="shrink-0 rounded-lg bg-indigo-600 px-5 py-2 font-semibold text-white transition hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {pending ? 'Pujando…' : 'Pujar'}
        </button>
      </div>

      <div className="mt-3 flex gap-2 text-xs">
        {[1, 5, 10].map((n) => (
          <button
            key={n}
            type="button"
            disabled={disabled}
            onClick={() => bumpBy(n)}
            className="rounded-full border border-slate-300 px-3 py-1 text-slate-600 hover:bg-slate-100 disabled:opacity-40 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
          >
            +{n} incremento{n > 1 ? 's' : ''}
          </button>
        ))}
      </div>

      {!isAuthenticated && (
        <p className="mt-3 text-sm text-amber-600 dark:text-amber-400">Inicia sesi&oacute;n para pujar.</p>
      )}
      {auction.status !== 'active' && (
        <p className="mt-3 text-sm text-slate-500">
          {auction.status === 'created' ? 'La subasta aún no comienza.' : 'Esta subasta ya finalizó.'}
        </p>
      )}
    </form>
  );
}
