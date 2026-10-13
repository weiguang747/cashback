import { useCallback, useEffect, useRef, useState } from 'react';

import { getTransactions, type Transaction } from './api';
import { deviceLocale } from './locale';

/** "Today", "Yesterday", "10 Oct", or "10 Oct 2025" in the device locale and time zone. */
export function dayLabel(date: Date, now = new Date()): string {
  const { languageTag } = deviceLocale();
  const days = Math.round((startOfDay(now).getTime() - startOfDay(date).getTime()) / 86_400_000);
  if (days === 0 || days === 1) {
    const word = relativeDay(languageTag, -days);
    return word.charAt(0).toLocaleUpperCase(languageTag) + word.slice(1);
  }
  return new Intl.DateTimeFormat(languageTag, {
    day: 'numeric',
    month: 'short',
    year: date.getFullYear() === now.getFullYear() ? undefined : 'numeric',
  }).format(date);
}

/** "8:42 AM" or "08.42", following the device's 12/24-hour setting. */
export function timeLabel(date: Date): string {
  const { languageTag, uses24hourClock } = deviceLocale();
  return new Intl.DateTimeFormat(languageTag, {
    hour: 'numeric',
    minute: '2-digit',
    hour12: uses24hourClock === null ? undefined : !uses24hourClock,
  }).format(date);
}

/** "today" / "yesterday" (or "hari ini" / "kemarin") for offset 0 / -1. */
function relativeDay(locale: string, offset: number): string {
  if (typeof Intl.RelativeTimeFormat === 'function') {
    return new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(offset, 'day');
  }
  return offset === 0 ? 'today' : 'yesterday';
}

/** Groups newest-first transactions into day sections. */
export function groupByDay(transactions: Transaction[], now = new Date()) {
  const groups: { title: string; data: Transaction[] }[] = [];
  for (const t of transactions) {
    const title = dayLabel(new Date(t.createdAt), now);
    const last = groups[groups.length - 1];
    if (last?.title === title) last.data.push(t);
    else groups.push({ title, data: [t] });
  }
  return groups;
}

function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

/**
 * Loads transactions from the server. `data` stays populated when a
 * reload fails, so screens can keep showing it alongside the error.
 */
export function useTransactions(limit?: number) {
  const [data, setData] = useState<Transaction[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const controller = useRef<AbortController | null>(null);

  // State is only set in the promise callbacks, so the mount effect below
  // doesn't trigger a synchronous re-render.
  const fetchTransactions = useCallback(() => {
    controller.current?.abort();
    const c = new AbortController();
    controller.current = c;
    return getTransactions({ limit, signal: c.signal }).then(
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
  }, [limit]);

  const reload = useCallback(() => {
    setLoading(true);
    setError(null);
    return fetchTransactions();
  }, [fetchTransactions]);

  useEffect(() => {
    fetchTransactions();
    return () => controller.current?.abort();
  }, [fetchTransactions]);

  return { data, error, loading, reload };
}
