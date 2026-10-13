import { useCallback, useEffect, useRef, useState } from 'react';

/** One page from the server: its rows, the cursor for the next, and anything else the first page carries (e.g. a balance). */
export type Page<T, M> = { items: T[]; nextCursor: string | null; meta: M };

/**
 * Loads a cursor-paged list: the first page on mount, more with `loadMore`
 * (for `onEndReached`). `fetchPage` must be stable (wrap it in useCallback);
 * when it changes (e.g. a new filter), the list starts again from the first
 * page.
 *
 * Like `useApi`, rows stay on screen when a reload fails, and state is only
 * set in promise callbacks. `error` is for the first page, `moreError` for
 * the page after the last one loaded.
 */
export function usePages<T, M = undefined>(
  fetchPage: (cursor: string | null, signal: AbortSignal) => Promise<Page<T, M>>,
) {
  const [items, setItems] = useState<T[] | null>(null);
  const [meta, setMeta] = useState<M | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reloading, setReloading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreError, setMoreError] = useState<string | null>(null);
  // The fetchPage the rows on screen came from. When it differs from the
  // current one, a new first page is on its way.
  const [loadedFrom, setLoadedFrom] = useState<typeof fetchPage | null>(null);
  const loadedFromRef = useRef<typeof fetchPage | null>(null);

  const first = useRef<AbortController | null>(null);
  const more = useRef<AbortController | null>(null);

  const run = useCallback(() => {
    first.current?.abort();
    // A page in flight belongs to the list being replaced.
    more.current?.abort();
    more.current = null;
    const c = new AbortController();
    first.current = c;
    return fetchPage(null, c.signal).then(
      (page) => {
        if (first.current === c) first.current = null;
        setItems(page.items);
        setMeta(page.meta);
        setNextCursor(page.nextCursor);
        loadedFromRef.current = fetchPage;
        setLoadedFrom(() => fetchPage);
        setError(null);
        setMoreError(null);
        setReloading(false);
        setLoadingMore(false);
      },
      (e) => {
        if (c.signal.aborted) return;
        first.current = null;
        setError(message(e));
        setReloading(false);
        setLoadingMore(false);
        // Rows from a different query (an old filter) would be misleading;
        // rows from this one are just stale, so they stay.
        if (loadedFromRef.current !== fetchPage) {
          setItems(null);
          setMeta(null);
          setNextCursor(null);
        }
        loadedFromRef.current = fetchPage;
        setLoadedFrom(() => fetchPage);
      },
    );
  }, [fetchPage]);

  const reload = useCallback(() => {
    setReloading(true);
    setError(null);
    return run();
  }, [run]);

  const loadMore = useCallback(() => {
    // Refs, not state, so two quick onEndReached calls can't both fetch.
    if (!nextCursor || first.current || more.current || loadedFrom !== fetchPage) return;
    const c = new AbortController();
    more.current = c;
    setLoadingMore(true);
    setMoreError(null);
    fetchPage(nextCursor, c.signal).then(
      (page) => {
        if (more.current !== c) return;
        more.current = null;
        setItems((prev) => [...(prev ?? []), ...page.items]);
        setNextCursor(page.nextCursor);
        setLoadingMore(false);
      },
      (e) => {
        if (c.signal.aborted || more.current !== c) return;
        more.current = null;
        setMoreError(message(e));
        setLoadingMore(false);
      },
    );
  }, [fetchPage, loadedFrom, nextCursor]);

  useEffect(() => {
    run();
    return () => {
      first.current?.abort();
      more.current?.abort();
    };
  }, [run]);

  return {
    items,
    meta,
    error,
    /** The first page is loading: initially, on reload, or after fetchPage changed. */
    loading: reloading || loadedFrom !== fetchPage,
    loadingMore,
    moreError,
    hasMore: nextCursor !== null,
    reload,
    loadMore,
  };
}

function message(e: unknown): string {
  return e instanceof Error ? e.message : 'Something went wrong. Please try again.';
}
