import { getApiUrl } from './runtimeConfig';
import type {
  ApiErrorBody,
  AuctionDetail,
  AuctionListResponse,
  AuthResponse,
  BidListResponse,
  PlaceBidResponse,
  User,
} from './types';

// sessionStorage, NOT localStorage: two tabs on the same origin share
// localStorage, so logging in as bob in tab B would silently switch tab A
// to bob too -- destroying the two-tabs-competing demo the spec asks for.
// sessionStorage is per-tab, so alice and bob can coexist.
const TOKEN_KEY = 'bidcraft_token';

export function getToken(): string | null {
  if (typeof window === 'undefined') return null;
  return window.sessionStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  window.sessionStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  window.sessionStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  code: string;
  status: number;
  details?: Record<string, unknown>;

  constructor(status: number, body: ApiErrorBody) {
    super(body.error.message);
    this.name = 'ApiError';
    this.status = status;
    this.code = body.error.code;
    this.details = body.error.details;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getToken();
  const headers = new Headers(init?.headers);
  headers.set('Content-Type', 'application/json');
  if (token) headers.set('Authorization', `Bearer ${token}`);

  const res = await fetch(`${getApiUrl()}${path}`, { ...init, headers });

  if (!res.ok) {
    let body: ApiErrorBody;
    try {
      body = await res.json();
    } catch {
      body = { error: { code: 'UNKNOWN', message: res.statusText } };
    }
    throw new ApiError(res.status, body);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export const api = {
  listAuctions: (status?: string) =>
    request<AuctionListResponse>(`/api/v1/auctions${status ? `?status=${status}` : ''}`),

  getAuction: (id: string) => request<AuctionDetail>(`/api/v1/auctions/${id}`),

  listBids: (id: string, limit = 50) =>
    request<BidListResponse>(`/api/v1/auctions/${id}/bids?limit=${limit}`),

  createAuction: (input: {
    title: string;
    description: string;
    image_url: string;
    base_price_cents: number;
    min_increment_cents: number;
    duration_seconds: number;
    starts_in_seconds?: number;
  }) =>
    request<AuctionDetail>('/api/v1/auctions', {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  placeBid: (auctionId: string, amountCents: number) =>
    request<PlaceBidResponse>(`/api/v1/auctions/${auctionId}/bids`, {
      method: 'POST',
      body: JSON.stringify({ amount_cents: amountCents }),
    }),

  register: (email: string, username: string, password: string) =>
    request<AuthResponse>('/api/v1/auth/register', {
      method: 'POST',
      body: JSON.stringify({ email, username, password }),
    }),

  login: (email: string, password: string) =>
    request<AuthResponse>('/api/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    }),

  me: () => request<{ user: User }>('/api/v1/auth/me'),

  serverTimeMs: () => request<{ server_time_ms: number }>('/api/v1/time'),
};
