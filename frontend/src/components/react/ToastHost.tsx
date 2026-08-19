import type { Toast } from '../../hooks/useToasts';

const STYLES: Record<Toast['kind'], string> = {
  success: 'bg-emerald-600',
  error: 'bg-red-600',
  info: 'bg-slate-800',
};

export default function ToastHost({ toasts, dismiss }: { toasts: Toast[]; dismiss: (id: number) => void }) {
  if (toasts.length === 0) return null;
  return (
    <div className="pointer-events-none fixed bottom-4 right-4 z-50 flex flex-col gap-2">
      {toasts.map((t) => (
        <div
          key={t.id}
          role="status"
          className={`pointer-events-auto flex items-center gap-3 rounded-lg px-4 py-3 text-sm text-white shadow-lg ${STYLES[t.kind]}`}
        >
          <span>{t.message}</span>
          <button onClick={() => dismiss(t.id)} className="opacity-70 hover:opacity-100" aria-label="Cerrar">
            &times;
          </button>
        </div>
      ))}
    </div>
  );
}
