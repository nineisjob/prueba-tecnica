import { useEffect, useState } from 'react';
import { splitDuration, type DurationParts } from '../lib/time';

const TICK_MS = 250;

export interface CountdownState {
  /** null until the first client-side tick has run -- see the hydration
   * note below. Render a static placeholder while null. */
  remainingMs: number | null;
  parts: DurationParts | null;
  expired: boolean;
}

/**
 * Always recomputes remaining time from the absolute target instant on
 * every tick -- it never decrements a stored counter. A backgrounded
 * browser tab throttles setInterval to >=1s (sometimes far more), so a
 * naive "subtract the tick interval" countdown would drift permanently
 * once the tab regains focus. Recomputing from `targetIso - (Date.now() +
 * offsetMs)` is self-correcting regardless of how late a tick actually
 * fires.
 *
 * The initial state is deliberately `null`, not a Date.now()-based value:
 * this component is server-rendered once (Astro's client:load renders the
 * island server-side for the initial HTML, then hydrates). If the first
 * render computed `Date.now()` directly, the server's render and the
 * client's pre-hydration render would compute it at two different
 * instants and produce different text -- a React hydration mismatch. A
 * fixed initial value is identical on both passes; the real ticking value
 * is set by this effect, which only ever runs client-side, after
 * hydration has already reconciled.
 */
export function useCountdown(targetIso: string, offsetMs: number): CountdownState {
  const targetMs = new Date(targetIso).getTime();
  const [remainingMs, setRemainingMs] = useState<number | null>(null);

  useEffect(() => {
    const tick = () => setRemainingMs(targetMs - (Date.now() + offsetMs));
    tick();
    const id = setInterval(tick, TICK_MS);
    return () => clearInterval(id);
  }, [targetMs, offsetMs]);

  return {
    remainingMs,
    parts: remainingMs === null ? null : splitDuration(remainingMs),
    expired: remainingMs !== null && remainingMs <= 0,
  };
}
