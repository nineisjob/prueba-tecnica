import { useState } from 'react';
import { useAuth } from '../../hooks/useAuth';
import { ApiError } from '../../lib/api';

export default function AuthForm() {
  const { login, register } = useAuth();
  const [mode, setMode] = useState<'login' | 'register'>('login');
  const [email, setEmail] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  const submit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setError(null);
    setPending(true);
    try {
      if (mode === 'login') {
        await login(email, password);
      } else {
        await register(email, username, password);
      }
      window.location.href = '/auctions';
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Error de red');
    } finally {
      setPending(false);
    }
  };

  return (
    <form onSubmit={submit} className="mx-auto max-w-sm space-y-4">
      <div className="flex gap-4 text-sm font-medium">
        <button type="button" onClick={() => setMode('login')} className={mode === 'login' ? 'text-indigo-600' : 'text-slate-400'}>
          Iniciar sesión
        </button>
        <button type="button" onClick={() => setMode('register')} className={mode === 'register' ? 'text-indigo-600' : 'text-slate-400'}>
          Registrarse
        </button>
      </div>

      <div>
        <label className="text-sm text-slate-600 dark:text-slate-300">Email</label>
        <input
          type="email"
          required
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 dark:border-slate-700 dark:bg-slate-900"
        />
      </div>

      {mode === 'register' && (
        <div>
          <label className="text-sm text-slate-600 dark:text-slate-300">Usuario</label>
          <input
            type="text"
            required
            minLength={3}
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 dark:border-slate-700 dark:bg-slate-900"
          />
        </div>
      )}

      <div>
        <label className="text-sm text-slate-600 dark:text-slate-300">Contraseña</label>
        <input
          type="password"
          required
          minLength={8}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 dark:border-slate-700 dark:bg-slate-900"
        />
      </div>

      {error && <p className="text-sm text-red-500">{error}</p>}

      <button
        type="submit"
        disabled={pending}
        className="w-full rounded-lg bg-indigo-600 py-2 font-semibold text-white hover:bg-indigo-500 disabled:opacity-50"
      >
        {pending ? 'Enviando…' : mode === 'login' ? 'Entrar' : 'Crear cuenta'}
      </button>

      <p className="text-center text-xs text-slate-400">
        Usuarios demo: alice@bidcraft.dev / bob@bidcraft.dev / carol@bidcraft.dev — contraseña demo1234
      </p>
    </form>
  );
}
