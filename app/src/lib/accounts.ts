import { useCallback } from 'react';

import { getAccounts } from './api';
import { useApi } from './useApi';

/** The signed-in user's main and cash back accounts. */
export function useAccounts() {
  const fetcher = useCallback((signal: AbortSignal) => getAccounts({ signal }), []);
  const { data, ...rest } = useApi(fetcher);
  return {
    ...rest,
    main: data?.find((a) => a.kind === 'main') ?? null,
    cashback: data?.find((a) => a.kind === 'cashback') ?? null,
  };
}
