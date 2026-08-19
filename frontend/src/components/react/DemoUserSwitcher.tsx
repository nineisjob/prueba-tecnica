import { useState } from 'react';

// Matches backend/internal/platform/seed/seed.go's DemoUsers -- one-click
// login lets the two-tabs demo video skip typing credentials on camera.
const DEMO_USERS = [
  { email: 'alice@bidcraft.dev', label: 'Alice' },
  { email: 'bob@bidcraft.dev', label: 'Bob' },
  { email: 'carol@bidcraft.dev', label: 'Carol' },
];
const DEMO_PASSWORD = 'demo1234';

interface Props {
  currentUsername: string | null;
  onLogin: (email: string, password: string) => Promise<unknown>;
  onLogout: () => void;
}

export default function DemoUserSwitcher({ currentUsername, onLogin, onLogout }: Props) {
  const [pending, setPending] = useState<string | null>(null);

  const switchTo = async (email: string) => {
    setPending(email);
    try {
      await onLogin(email, DEMO_PASSWORD);
    } finally {
      setPending(null);
    }
  };

  return (
    <div className="flex items-center gap-2 text-sm">
      {currentUsername ? (
        <>
          <span className="text-slate-600 dark:text-slate-300">
            Sesi&oacute;n: <strong>{currentUsername}</strong>
          </span>
          <button onClick={onLogout} className="text-indigo-600 hover:underline dark:text-indigo-400">
            Salir
          </button>
        </>
      ) : (
        <div className="flex gap-1">
          {DEMO_USERS.map((u) => (
            <button
              key={u.email}
              onClick={() => switchTo(u.email)}
              disabled={pending !== null}
              className="rounded-full border border-slate-300 px-3 py-1 text-xs hover:bg-slate-100 disabled:opacity-50 dark:border-slate-700 dark:hover:bg-slate-800"
            >
              {pending === u.email ? '…' : u.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
