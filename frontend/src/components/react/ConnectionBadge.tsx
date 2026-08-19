import type { ConnectionStatus } from '../../hooks/useWebSocket';

const LABELS: Record<ConnectionStatus, string> = {
  open: 'En vivo',
  connecting: 'Conectando…',
  closed: 'Reconectando…',
  error: 'Reconectando…',
};

const DOT: Record<ConnectionStatus, string> = {
  open: 'bg-emerald-500',
  connecting: 'bg-amber-400 animate-pulse',
  closed: 'bg-amber-400 animate-pulse',
  error: 'bg-red-500 animate-pulse',
};

export default function ConnectionBadge({ status }: { status: ConnectionStatus }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs font-medium text-slate-500 dark:text-slate-400">
      <span className={`h-2 w-2 rounded-full ${DOT[status]}`} />
      {LABELS[status]}
    </span>
  );
}
