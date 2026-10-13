import { useCallback } from 'react';

import { getCampaign, getCashbackHistory } from './api';
import { useApi } from './useApi';

/** The cash back campaign to show: live, ended, or null if there's never been one. */
export function useCampaign() {
  const fetcher = useCallback((signal: AbortSignal) => getCampaign({ signal }), []);
  return useApi(fetcher);
}

/** The cash back balance and the activity behind it. */
export function useCashbackHistory() {
  const fetcher = useCallback((signal: AbortSignal) => getCashbackHistory({ signal }), []);
  return useApi(fetcher);
}

/** "5%" from basis points; keeps decimals only when needed ("2.5%"). */
export function ratePercent(rateBps: number): string {
  return `${rateBps / 100}%`;
}
