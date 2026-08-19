import { useCountdown } from '../../hooks/useCountdown';
import { formatCountdown } from '../../lib/time';

interface Props {
  endsAt: string;
  offsetMs: number;
  closing: boolean;
}

export default function Countdown({ endsAt, offsetMs, closing }: Props) {
  const { remainingMs, expired } = useCountdown(endsAt, offsetMs);
  const urgent = remainingMs !== null && remainingMs > 0 && remainingMs < 10_000;

  if (closing) {
    return <div className="font-mono text-2xl font-bold text-amber-500 tabular-nums">Cerrando&hellip;</div>;
  }

  return (
    <div
      className={`font-mono text-3xl font-bold tabular-nums ${
        expired ? 'text-slate-400' : urgent ? 'text-red-500' : 'text-slate-900 dark:text-slate-50'
      }`}
      aria-live="polite"
    >
      {remainingMs === null ? '--:--' : expired ? '00:00' : formatCountdown(remainingMs)}
    </div>
  );
}
