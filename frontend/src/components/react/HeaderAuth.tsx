import { useAuth } from '../../hooks/useAuth';

/** Global, always-visible session indicator + login entry point. Mounted
 * once inside Header.astro (client:load) so every page -- including the
 * public SSR landing and catalog -- shows who, if anyone, is logged in. */
export default function HeaderAuth() {
  const { user, loading, logout } = useAuth();

  if (loading) {
    return <span className="text-sm text-slate-400">…</span>;
  }

  if (!user) {
    return (
      <a
        href="/login"
        className="rounded-full bg-indigo-600 px-4 py-1.5 text-sm font-semibold text-white hover:bg-indigo-500"
      >
        Iniciar sesión
      </a>
    );
  }

  return (
    <div className="flex items-center gap-3 text-sm">
      <span className="text-slate-600 dark:text-slate-300">
        Sesión: <strong className="text-slate-900 dark:text-slate-50">{user.username}</strong>
      </span>
      <button
        onClick={() => {
          logout();
          window.location.reload();
        }}
        className="text-indigo-600 hover:underline dark:text-indigo-400"
      >
        Salir
      </button>
    </div>
  );
}
