import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../lib/api';

const RESYNC_INTERVAL_MS = 60_000;
const SAMPLE_COUNT = 3;

/**
 * SNTP-style clock offset: offset = server_time_ms + (t1-t0)/2 - t1, taking
 * the median of a few samples to smooth out one unlucky slow request.
 * Every render should compute "now" as Date.now() + offsetMs rather than
 * trusting the browser clock alone -- see useCountdown for why this
 * matters (a client clock that's off by even a few seconds would make the
 * countdown disagree with when the server actually closes the auction).
 */
export function useServerTime() {
  const [offsetMs, setOffsetMs] = useState(0);
  const [synced, setSynced] = useState(false);
  const inFlight = useRef(false);

  const resync = useCallback(async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    try {
      const samples: number[] = [];
      for (let i = 0; i < SAMPLE_COUNT; i++) {
        const t0 = Date.now();
        const { server_time_ms } = await api.serverTimeMs();
        const t1 = Date.now();
        samples.push(server_time_ms + (t1 - t0) / 2 - t1);
      }
      samples.sort((a, b) => a - b);
      setOffsetMs(samples[Math.floor(samples.length / 2)]);
      setSynced(true);
    } catch {
      // Keep the previous offset (or 0) if the sync request fails; the UI
      // degrades to trusting the local clock rather than breaking.
    } finally {
      inFlight.current = false;
    }
  }, []);

  useEffect(() => {
    resync();
    const id = setInterval(resync, RESYNC_INTERVAL_MS);
    return () => clearInterval(id);
  }, [resync]);

  return { offsetMs, synced, resync };
}
