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
  // SSR: prefer the internal Docker DNS name; fall back to localhost for
  // `astro dev` outside a container.
  return import.meta.env.API_INTERNAL_URL || 'http://localhost:8080';
}

export function getWsUrl(): string {
  if (typeof window !== 'undefined' && window.__BIDCRAFT__) {
    return window.__BIDCRAFT__.wsUrl;
  }
  return getApiUrl().replace(/^http/, 'ws');
}

export function getPublicApiUrl(): string {
  return import.meta.env.PUBLIC_API_URL || 'http://localhost:8080';
}
