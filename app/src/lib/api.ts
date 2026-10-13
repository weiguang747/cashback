import { clearUser, getUser } from './session';

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
  /** Cash back this payment earned, if any. */
  cashback?: Money;
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

export type PaymentInput = { merchantId: string; merchantName: string; amount: Money };
export type CashbackReason = 'awarded' | 'below_minimum' | 'daily_cap_reached' | 'budget_exhausted';

export type Payment = {
  transaction: Transaction;
  /** Main account balance after the payment. */
  balance: Money;
  /** Null when no cash back campaign was live. */
  cashback: { amount: Money; reason: CashbackReason } | null;
};

export type Campaign = {
  id: string;
  name: string;
  status: 'active' | 'ended';
  /** 500 = 5%. */
  rateBps: number;
  minSpend: Money;
  dailyCap: Money;
  /** What the signed-in user has earned today, against dailyCap. */
  earnedToday: Money;
  budgetRemaining: Money;
  /** When the daily cap resets (ISO 8601, UTC). */
  resetAt: string;
};

/** The live cash back campaign, the latest ended one, or null if there's never been one. */
export async function getCampaign(opts: { signal?: AbortSignal } = {}): Promise<Campaign | null> {
  const data = await request<{ campaign: Campaign | null }>('/v1/cashback/campaign', { signal: opts.signal });
  return data.campaign;
}

export type CashbackHistory = { balance: Money; transactions: Transaction[] };

/** The cash back balance and its activity, newest first. */
export function getCashbackHistory(opts: { signal?: AbortSignal } = {}): Promise<CashbackHistory> {
  return request<CashbackHistory>('/v1/cashback/history', { signal: opts.signal });
}

/**
 * Pays a merchant from the main account. `idempotencyKey` must be a fresh
 * UUID per payment, reused when retrying it: the server then returns the
 * first result instead of charging again.
 */
export async function createPayment(input: PaymentInput, idempotencyKey: string): Promise<Payment> {
  const data = await post<{ payment: Payment }>('/v1/payments', input, { 'Idempotency-Key': idempotencyKey });
  return data.payment;
}

/**
 * A failed request. `status` is 0 when the server couldn't be reached, and
 * `code` is the server's machine-readable reason, e.g. "insufficient_funds".
 */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

function post<T>(path: string, body: unknown, headers: Record<string, string> = {}): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...headers },
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
    throw new ApiError("Can't reach the server. Check your connection and try again.", 0);
  }

  const data = await res.json().catch(() => null);
  // A saved session can outlive its user (e.g. the database was re-seeded).
  // Signing out sends the app back to the sign-in screen.
  if (res.status === 401 && user) clearUser();
  if (!res.ok) {
    throw new ApiError(data?.error ?? 'Something went wrong. Please try again.', res.status, data?.code);
  }
  return data as T;
}
