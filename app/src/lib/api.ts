import { getUser } from './session';

/**
 * Client for the cashback server. Set EXPO_PUBLIC_API_URL in `.env` to point
 * somewhere other than localhost (see `.env.example`).
 */
export const API_URL = process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:8080';

export type User = { id: string; email: string; name: string };
export type SignInResponse = { user: User };

/** Signs in with the email of an existing user. Demo only: there is no password. */
export function signIn(email: string): Promise<SignInResponse> {
  return post<SignInResponse>('/v1/auth/sign-in', { email });
}

/**
 * An exact amount. `value` is a decimal string in major units ("-35000.00"),
 * never a JS number, so large amounts and decimals keep full precision.
 */
export type Money = {
  value: string;
  /** ISO 4217 code, e.g. "IDR". */
  currency: string;
};

export type TransactionKind = 'in' | 'out' | 'cashback';

export type Transaction = {
  id: string;
  name: string;
  /** e.g. "Payment", "Received", "Bill" */
  description: string;
  /** Negative for money out. */
  amount: Money;
  kind: TransactionKind;
  /** ISO 8601 timestamp in UTC; show it in the device's time zone. */
  createdAt: string;
};

/** Transactions, newest first. Pass `limit` to get only the most recent ones. */
export async function getTransactions(opts: { limit?: number; signal?: AbortSignal } = {}): Promise<Transaction[]> {
  const query = opts.limit ? `?limit=${opts.limit}` : '';
  const data = await request<{ transactions: Transaction[] }>(`/v1/transactions${query}`, { signal: opts.signal });
  return data.transactions;
}

export type Account = {
  id: string;
  kind: 'main' | 'cashback';
  balance: Money;
};

/** The signed-in user's accounts, main first. */
export async function getAccounts(opts: { signal?: AbortSignal } = {}): Promise<Account[]> {
  const data = await request<{ accounts: Account[] }>('/v1/accounts', { signal: opts.signal });
  return data.accounts;
}

function post<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  // Demo stand-in for auth: the server trusts this header (see requireUser).
  const user = getUser();
  const headers = new Headers(init.headers);
  if (user) headers.set('X-User-ID', user.id);

  let res: Response;
  try {
    res = await fetch(`${API_URL}${path}`, { ...init, headers });
  } catch (e) {
    if (e instanceof Error && e.name === 'AbortError') throw e;
    throw new Error("Can't reach the server. Check your connection and try again.");
  }

  const data = await res.json().catch(() => null);
  if (!res.ok) {
    throw new Error(data?.error ?? 'Something went wrong. Please try again.');
  }
  return data as T;
}
