export interface DurationParts {
  h: number;
  m: number;
  s: number;
  ms: number;
}

export function splitDuration(remainingMs: number): DurationParts {
  const clamped = Math.max(0, remainingMs);
  const h = Math.floor(clamped / 3_600_000);
  const m = Math.floor((clamped % 3_600_000) / 60_000);
  const s = Math.floor((clamped % 60_000) / 1000);
  const ms = Math.floor(clamped % 1000);
  return { h, m, s, ms };
}

export function formatCountdown(remainingMs: number): string {
  const { h, m, s, ms } = splitDuration(remainingMs);
  const pad = (n: number) => String(n).padStart(2, '0');
  if (h > 0) return `${h}:${pad(m)}:${pad(s)}`;
  if (remainingMs < 60_000) return `${m}:${pad(s)}.${String(Math.floor(ms / 100))}`;
  return `${m}:${pad(s)}`;
}
