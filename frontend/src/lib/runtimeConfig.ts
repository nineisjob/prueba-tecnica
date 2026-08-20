// Astro SSR must reach the backend via Docker's internal DNS
// (http://backend:8080); the browser must reach it via the host-published
// port (http://localhost:8080). PUBLIC_* env vars are inlined by Vite at
// BUILD time, so baking one into the image would make it non-portable
// across hosts. Instead the server injects the browser-facing URL at
// RUNTIME via a small inline script in BaseLayout.astro, and this module
// picks the right source depending on which side it's running on.

declare global {
  interface Window {
    __BIDCRAFT__?: { apiUrl: string; wsUrl: string };
  }
}

export function getApiUrl(): string {
  if (typeof window !== 'undefined' && window.__BIDCRAFT__) {
    return window.__BIDCRAFT__.apiUrl;
  }
  // SSR: read dynamically from process.env at runtime.
  // In Docker, API_INTERNAL_URL is 'http://backend:8080'.
  // In local dev outside Docker, falls back to 'http://localhost:8080'.
  if (typeof process !== 'undefined' && process.env) {
    if (process.env.API_INTERNAL_URL) return process.env.API_INTERNAL_URL;
    if (process.env.PUBLIC_API_URL) return process.env.PUBLIC_API_URL;
  }
  return 'http://localhost:8080';
}

export function getWsUrl(): string {
  if (typeof window !== 'undefined' && window.__BIDCRAFT__) {
    return window.__BIDCRAFT__.wsUrl;
  }
  return getApiUrl().replace(/^http/, 'ws');
}

export function getPublicApiUrl(): string {
  if (typeof process !== 'undefined' && process.env?.PUBLIC_API_URL) {
    return process.env.PUBLIC_API_URL;
  }
  return 'http://localhost:8080';
}
