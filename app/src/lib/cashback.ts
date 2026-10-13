import { useCallback } from 'react';

import { getCampaign, getCashbackHistory } from './api';
import { useApi } from './useApi';
import { usePages } from './usePages';

/** The cash back campaign to show: live, ended, or null if there's never been one. */
export function useCampaign() {
  const fetcher = useCallback((signal: AbortSignal) => getCampaign({ signal }), []);
  return useApi(fetcher);
}

/**
 * The cash back activity, a page at a time (see `usePages`), with the
 * balance from the first page as `meta.balance`.
 */
export function useCashbackHistory() {
  const fetchPage = useCallback(
    (cursor: string | null, signal: AbortSignal) =>
      getCashbackHistory({ cursor, signal }).then((page) => ({
        items: page.transactions,
        nextCursor: page.nextCursor,
        meta: { balance: page.balance },
      })),
    [],
  );
  return usePages(fetchPage);
}

/** "5%" from basis points; keeps decimals only when needed ("2.5%"). */
export function ratePercent(rateBps: number): string {
  return `${rateBps / 100}%`;
}
