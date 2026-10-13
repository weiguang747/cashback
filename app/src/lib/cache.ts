/**
 * Server data shared between screens, by key. A screen that needs data
 * another screen already loaded shows it at once instead of a spinner, and
 * the server's response to a payment or redemption updates every screen
 * (see `wallet.ts`). Memory only; cleared on sign-in and sign-out.
 *
 * Everything here is a copy of what the server said. Never compute money
 * into it: the server stays the authority.
 */

type Entry = { data: unknown; fetchedAt: number };

const entries = new Map<string, Entry>();
const listeners = new Map<string, Set<() => void>>();
// Bumped by clearCache, so a response for the previous user that arrives
// after sign-out can't land in the new session's cache.
let generation = 0;

/** How long data counts as fresh: within this, screens reuse it without a request. */
export const FRESH_MS = 30_000;

export function getCached<T>(key: string): T | undefined {
  return entries.get(key)?.data as T | undefined;
}

/** Whether `key` has data that is recent and hasn't been invalidated. */
export function isFresh(key: string): boolean {
  const entry = entries.get(key);
  return !!entry && Date.now() - entry.fetchedAt < FRESH_MS;
}

export function cacheGeneration(): number {
  return generation;
}

/** Stores the server's data for `key`. Ignored if the cache was cleared since `gen`. */
export function setCached<T>(key: string, data: T, gen = generation) {
  if (gen !== generation) return;
  entries.set(key, { data, fetchedAt: Date.now() });
  notify(key);
}

/**
 * Applies data from a server response to what's cached under `key`, if
 * anything is; the result counts as fresh.
 */
export function updateCached<T>(key: string, update: (prev: T) => T) {
  const entry = entries.get(key);
  if (!entry) return;
  setCached(key, update(entry.data as T));
}

/** Keys currently cached that start with `prefix`. */
export function cachedKeys(prefix: string): string[] {
  return [...entries.keys()].filter((k) => k.startsWith(prefix));
}

/** Keeps the data on screen but makes the next refresh fetch it again. */
export function invalidate(key: string) {
  const entry = entries.get(key);
  if (entry) entry.fetchedAt = 0;
}

export function clearCache() {
  generation += 1;
  const keys = [...entries.keys()];
  entries.clear();
  keys.forEach(notify);
}

export function subscribe(key: string, listener: () => void): () => void {
  let set = listeners.get(key);
  if (!set) listeners.set(key, (set = new Set()));
  set.add(listener);
  return () => {
    set.delete(listener);
    if (!set.size) listeners.delete(key);
  };
}

function notify(key: string) {
  listeners.get(key)?.forEach((l) => l());
}
