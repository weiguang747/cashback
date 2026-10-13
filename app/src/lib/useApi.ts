import { useCallback, useEffect, useRef, useState } from 'react';

/**
 * Loads data from the server on mount and exposes `reload`. `data` stays
 * populated when a reload fails, so screens can keep showing it alongside
 * the error. `fetcher` must be stable (wrap it in useCallback).
 */
export function useApi<T>(fetcher: (signal: AbortSignal) => Promise<T>) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const controller = useRef<AbortController | null>(null);

  // State is only set in the promise callbacks, so the mount effect below
  // doesn't trigger a synchronous re-render.
  const run = useCallback(() => {
    controller.current?.abort();
    const c = new AbortController();
    controller.current = c;
    return fetcher(c.signal).then(
      (result) => {
        setData(result);
        setError(null);
        setLoading(false);
      },
      (e) => {
        if (c.signal.aborted) return;
        setError(e instanceof Error ? e.message : 'Something went wrong. Please try again.');
        setLoading(false);
      },
    );
  }, [fetcher]);

  const reload = useCallback(() => {
    setLoading(true);
    setError(null);
    return run();
  }, [run]);

  useEffect(() => {
    run();
    return () => controller.current?.abort();
  }, [run]);

  return { data, error, loading, reload };
}
