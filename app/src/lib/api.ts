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

/**
 * Lists come a page at a time, newest first. `limit` is the page size
 * (server default 20, at most 100). Pass a page's `nextCursor` as `cursor`
 * to get the next one; it is null on the last page.
 */
export type PageOptions = { limit?: number; cursor?: string | null; signal?: AbortSignal };

export type TransactionPage = { transactions: Transaction[]; nextCursor: string | null };

/**
 * One page of main account transactions. `kind` and `q` (text in the name,
 * case-insensitive) filter on the server, so paging covers every match.
 */
export function getTransactions(
  opts: PageOptions & { kind?: TransactionKind; q?: string } = {},
): Promise<TransactionPage> {
  const query = queryString({ limit: opts.limit, cursor: opts.cursor, kind: opts.kind, q: opts.q });
  return request<TransactionPage>(`/v1/transactions${query}`, { signal: opts.signal });
}

export type Account = {
  id: string;
  kind: 'main' | 'cashback';
  balance: Money;
  /** Cash back accounts only: earned but not yet in `balance` (credited shortly). Not redeemable yet. */
  pending?: Money;
};

/** The signed-in user's accounts, main first. */
export async function getAccounts(opts: { signal?: AbortSignal } = {}): Promise<Account[]> {
  const data = await request<{ accounts: Account[] }>('/v1/accounts', { signal: opts.signal });
  return data.accounts;
}

export type PaymentInput = { merchantId: string; merchantName: string; amount: Money };
/** `unavailable`: the payment went ahead without cash back (the server couldn't check the budget, or couldn't hold the award). */
export type CashbackReason = 'awarded' | 'below_minimum' | 'daily_cap_reached' | 'budget_exhausted' | 'unavailable';
/** `pending`: earned, and on its way to the cash back balance. A replayed response can still say pending. */
export type CashbackStatus = 'pending' | 'credited';

export type Payment = {
  transaction: Transaction;
  /** Main account balance after the payment. */
  balance: Money;
  /** Null when no cash back campaign was live. */
  cashback: { amount: Money; reason: CashbackReason; status: CashbackStatus } | null;
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

export type CashbackHistory = {
  /** The cash back balance: on the first page only, null on later pages. */
  balance: Money | null;
  transactions: Transaction[];
  nextCursor: string | null;
};

/** One page of cash back activity, newest first, with the balance on the first page. */
export function getCashbackHistory(opts: PageOptions = {}): Promise<CashbackHistory> {
  const query = queryString({ limit: opts.limit, cursor: opts.cursor });
  return request<CashbackHistory>(`/v1/cashback/history${query}`, { signal: opts.signal });
}

/** "?a=1&b=x" from the params that are set; "" when none are. */
function queryString(params: Record<string, string | number | null | undefined>): string {
  const parts = Object.entries(params)
    .filter(([, v]) => v !== undefined && v !== null && v !== '')
    .map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`);
  return parts.length ? `?${parts.join('&')}` : '';
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

export type Redemption = {
  /** The credit on the main account. */
  transaction: Transaction;
  /** Main account balance after redeeming. */
  balance: Money;
  /** Cash back left to redeem. */
  cashbackBalance: Money;
  /** Cash back earned but not yet credited; redeemable once it is. */
  cashbackPending: Money;
};

/**
 * Moves `amount` of cash back to the main balance. Like payments, reuse the
 * idempotency key when retrying so cash back is never moved twice.
 */
export async function redeemCashback(amount: Money, idempotencyKey: string): Promise<Redemption> {
  const data = await post<{ redemption: Redemption }>('/v1/cashback/redeem', { amount }, { 'Idempotency-Key': idempotencyKey });
  return data.redemption;
}

/**
 * A failed request. `status` is 0 when the server couldn't be reached (then
 * `code` is "timeout" if it stopped responding), otherwise `code` is the
 * server's machine-readable reason, e.g. "insufficient_funds".
 * `requestId` finds the request in the server logs.
 */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

// Development builds only. Logs method, path, status, code and request id:
// never bodies or headers, which carry user data and the user id.
function logFailure(method: string, path: string, e: ApiError) {
  if (!__DEV__) return;
  console.warn(`[api] ${method} ${path.split('?')[0]} failed: ${e.status || 'network'} ${e.code ?? ''} (request ${e.requestId ?? 'none'})`);
}

function post<T>(path: string, body: unknown, headers: Record<string, string> = {}): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...headers },
    body: JSON.stringify(body),
  });
}

/**
 * How long to wait for a response. Without a limit a stalled server leaves
 * "Processing…" on screen indefinitely. A timed-out payment may still
 * complete on the server, so callers treat `timeout` as an unknown outcome
 * and retry with the same Idempotency-Key.
 */
const TIMEOUT_MS = 15_000;

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  // Demo stand-in for auth: the server trusts this header (see requireUser).
  const user = getUser();
  const headers = new Headers(init.headers);
  if (user) headers.set('X-User-ID', user.id);

  // One signal for both the caller's cancel (e.g. leaving a screen) and the timeout.
  const caller = init.signal;
  const controller = new AbortController();
  const cancel = () => controller.abort();
  if (caller?.aborted) cancel();
  caller?.addEventListener('abort', cancel);
  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    cancel();
  }, TIMEOUT_MS);

  const method = init.method ?? 'GET';
  let res: Response;
  let data: unknown;
  try {
    res = await fetch(`${API_URL}${path}`, { ...init, headers, signal: controller.signal });
    // A body that isn't JSON is fine (the status still counts); an abort while reading it isn't.
    data = await res.json().catch((e: unknown) => {
      if (controller.signal.aborted) throw e;
      return null;
    });
  } catch (e) {
    if (!timedOut && e instanceof Error && e.name === 'AbortError') throw e;
    const err = timedOut
      ? new ApiError('The server took too long to respond. Please try again.', 0, 'timeout')
      : new ApiError("Can't reach the server. Check your connection and try again.", 0);
    logFailure(method, path, err);
    throw err;
  } finally {
    clearTimeout(timer);
    caller?.removeEventListener('abort', cancel);
  }

  // A saved session can outlive its user (e.g. the database was re-seeded).
  // Signing out sends the app back to the sign-in screen.
  if (res.status === 401 && user) clearUser();
  if (!res.ok) {
    const body = data as { error?: string; code?: string } | null;
    const err = new ApiError(
      body?.error ?? 'Something went wrong. Please try again.',
      res.status,
      body?.code,
      res.headers.get('X-Request-ID') ?? undefined,
    );
    logFailure(method, path, err);
    throw err;
  }
  return data as T;
}
