import { useCallback } from 'react';

import { getAccounts } from './api';
import { useApi } from './useApi';

/** Cache key for the signed-in user's accounts. */
export const ACCOUNTS_KEY = 'accounts';

/** The signed-in user's main and cash back accounts. */
export function useAccounts() {
  const fetcher = useCallback((signal: AbortSignal) => getAccounts({ signal }), []);
  const { data, ...rest } = useApi(ACCOUNTS_KEY, fetcher);
  return {
    ...rest,
    main: data?.find((a) => a.kind === 'main') ?? null,
    cashback: data?.find((a) => a.kind === 'cashback') ?? null,
  };
}
