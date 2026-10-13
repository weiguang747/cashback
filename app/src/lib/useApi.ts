import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from 'react';

import { cacheGeneration, getCached, isFresh, setCached, subscribe } from './cache';

/**
 * Loads data from the server into the shared cache under `key` (see
 * `cache.ts`) and exposes `reload` and `refresh`. Data another screen
 * loaded recently shows at once and isn't fetched again on mount. `data`
 * stays populated when a reload fails, so screens can keep showing it
 * alongside the error. `fetcher` must be stable (wrap it in useCallback).
 */
export function useApi<T>(key: string, fetcher: (signal: AbortSignal) => Promise<T>) {
  const subscribeKey = useCallback((listener: () => void) => subscribe(key, listener), [key]);
  const snapshot = useCallback(() => getCached<T>(key) ?? null, [key]);
  const data = useSyncExternalStore(subscribeKey, snapshot);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(() => !isFresh(key));
  const controller = useRef<AbortController | null>(null);

  // State is only set in the promise callbacks, so the mount effect below
  // doesn't trigger a synchronous re-render.
  const run = useCallback(() => {
    controller.current?.abort();
    const c = new AbortController();
    controller.current = c;
    const gen = cacheGeneration();
    return fetcher(c.signal).then(
      (result) => {
        setCached(key, result, gen);
        setError(null);
        setLoading(false);
      },
      (e) => {
        if (c.signal.aborted) return;
        setError(e instanceof Error ? e.message : 'Something went wrong. Please try again.');
        setLoading(false);
      },
    );
  }, [fetcher, key]);

  /** Fetches again now, showing `loading`. */
  const reload = useCallback(() => {
    setLoading(true);
    setError(null);
    return run();
  }, [run]);

  /** Fetches in the background only if the data is missing, old or invalidated. */
  const refresh = useCallback(() => (isFresh(key) ? Promise.resolve() : run()), [key, run]);

  useEffect(() => {
    if (!isFresh(key)) run();
    return () => controller.current?.abort();
  }, [key, run]);

  return { data, error, loading, reload, refresh };
}
