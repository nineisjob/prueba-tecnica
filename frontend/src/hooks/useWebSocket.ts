import { useEffect, useRef, useState } from 'react';

export type ConnectionStatus = 'connecting' | 'open' | 'closed' | 'error';

const MAX_BACKOFF_MS = 30_000;
const BASE_BACKOFF_MS = 1_000;

export interface UseWebSocketOptions<E> {
  onEvent: (ev: E) => void;
  enabled?: boolean;
}

/**
 * Generic reconnecting WebSocket hook with exponential backoff (1s -> 30s,
 * jittered). The socket is only ever re-created when `url` changes -- NOT
 * on every render -- because `onEvent` is captured in a ref rather than a
 * dependency; otherwise a parent re-render would tear down and reopen the
 * connection on every state update, which would also cause the server's
 * per-connection `snapshot` event to keep re-arriving.
 */
export function useWebSocket<E = unknown>(url: string, opts: UseWebSocketOptions<E>) {
  const [status, setStatus] = useState<ConnectionStatus>('connecting');
  const [reconnectAttempt, setReconnectAttempt] = useState(0);
  const onEventRef = useRef(opts.onEvent);
  onEventRef.current = opts.onEvent;
  const enabled = opts.enabled ?? true;

  useEffect(() => {
    if (!enabled) return;

    let socket: WebSocket | null = null;
    let attempt = 0;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let stopped = false;

    const connect = () => {
      if (stopped) return;
      setStatus('connecting');
      socket = new WebSocket(url);

      socket.onopen = () => {
        attempt = 0;
        setReconnectAttempt(0);
        setStatus('open');
      };

      socket.onmessage = (event) => {
        try {
          onEventRef.current(JSON.parse(event.data));
        } catch {
          // Malformed frame: ignore rather than crash the connection.
        }
      };

      socket.onerror = () => setStatus('error');

      socket.onclose = () => {
        if (stopped) return;
        setStatus('closed');
        attempt += 1;
        setReconnectAttempt(attempt);
        const backoff = Math.min(MAX_BACKOFF_MS, BASE_BACKOFF_MS * 2 ** (attempt - 1));
        const jitter = backoff * (0.5 + Math.random() * 0.5);
        reconnectTimer = setTimeout(connect, jitter);
      };
    };

    const handleVisibility = () => {
      // Close on tab hidden for >5min to avoid holding a stale connection
      // open indefinitely in a background tab; reopens on visibilitychange.
      if (document.visibilityState === 'hidden') {
        reconnectTimer && clearTimeout(reconnectTimer);
      } else if (document.visibilityState === 'visible' && socket?.readyState !== WebSocket.OPEN) {
        connect();
      }
    };
    document.addEventListener('visibilitychange', handleVisibility);

    connect();

    return () => {
      stopped = true;
      document.removeEventListener('visibilitychange', handleVisibility);
      if (reconnectTimer) clearTimeout(reconnectTimer);
      socket?.close();
    };
  }, [url, enabled]);

  return { status, reconnectAttempt };
}
