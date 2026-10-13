/**
 * Client for the cashback server. Set EXPO_PUBLIC_API_URL in `.env` to point
 * somewhere other than localhost (see `.env.example`).
 */
export const API_URL = process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:8080';

export type User = { email: string };
export type SignInResponse = { token: string; user: User };

/** Signs in with an email. The server accepts any valid email for now. */
export function signIn(email: string): Promise<SignInResponse> {
  return post<SignInResponse>('/auth/sign-in', { email });
}

export type TransactionKind = 'in' | 'out' | 'cashback';

export type Transaction = {
  id: string;
  name: string;
  /** e.g. "Payment", "Received", "Bill" */
  description: string;
  /** Whole rupiah. Negative for money out. */
  amount: number;
  kind: TransactionKind;
  /** ISO 8601 timestamp. */
  createdAt: string;
};

/** Transactions, newest first. Pass `limit` to get only the most recent ones. */
export async function getTransactions(opts: { limit?: number; signal?: AbortSignal } = {}): Promise<Transaction[]> {
  const query = opts.limit ? `?limit=${opts.limit}` : '';
  const data = await request<{ transactions: Transaction[] }>(`/transactions${query}`, { signal: opts.signal });
  return data.transactions;
}

function post<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${API_URL}${path}`, init);
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
