import type { Account, Money, Payment, Redemption, Transaction } from './api';
import { ACCOUNTS_KEY } from './accounts';
import { cachedKeys, invalidate, updateCached } from './cache';
import { CAMPAIGN_KEY } from './cashback';
import { isZeroMoney } from './format';
import { RECENT_TRANSACTIONS_KEY } from './transactions';

// The server's answer to a payment or redemption already holds the new
// balances and the new transaction. These put them in the shared cache, so
// Home is up to date when the user returns, without asking again.
// Only what the response states is written; anything it doesn't cover is
// invalidated and fetched on the next refresh.

/** Applies a completed payment's response. */
export function recordPayment(payment: Payment) {
  setBalance('main', payment.balance);
  addRecentTransaction(payment.transaction);
  if (payment.cashback) {
    // Today's earnings and the budget moved; the response doesn't say how much is left.
    invalidate(CAMPAIGN_KEY);
    // Cash back landed on an account whose new balance the response doesn't carry.
    if (!isZeroMoney(payment.cashback.amount)) invalidate(ACCOUNTS_KEY);
  }
}

/** Applies a completed redemption's response. */
export function recordRedemption(redemption: Redemption) {
  setBalance('main', redemption.balance);
  setBalance('cashback', redemption.cashbackBalance);
  addRecentTransaction(redemption.transaction);
}

function setBalance(kind: Account['kind'], balance: Money) {
  updateCached<Account[]>(ACCOUNTS_KEY, (accounts) =>
    accounts.map((a) => (a.kind === kind && a.balance.currency === balance.currency ? { ...a, balance } : a)),
  );
}

/** Puts a new main-account transaction at the top of every cached recent list. */
function addRecentTransaction(t: Transaction) {
  for (const key of cachedKeys(RECENT_TRANSACTIONS_KEY)) {
    const count = Number(key.slice(RECENT_TRANSACTIONS_KEY.length));
    updateCached<Transaction[]>(key, (list) => [t, ...list.filter((x) => x.id !== t.id)].slice(0, count));
  }
}
