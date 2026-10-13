import { useCallback, useEffect, useRef, useState } from 'react';

import { getTransactions, type Transaction } from './api';

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

/** "Today", "Yesterday", or "10 Oct". Uses the device's time zone. */
export function dayLabel(date: Date, now = new Date()): string {
  const days = Math.round((startOfDay(now).getTime() - startOfDay(date).getTime()) / 86_400_000);
  if (days === 0) return 'Today';
  if (days === 1) return 'Yesterday';
  return `${date.getDate()} ${MONTHS[date.getMonth()]}`;
}

/** "8:42 AM". */
export function timeLabel(date: Date): string {
  const h = date.getHours();
  const m = String(date.getMinutes()).padStart(2, '0');
  return `${h % 12 || 12}:${m} ${h < 12 ? 'AM' : 'PM'}`;
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

  const load = useCallback(async () => {
    controller.current?.abort();
    const c = new AbortController();
    controller.current = c;
    setLoading(true);
    setError(null);
    try {
      setData(await getTransactions({ limit, signal: c.signal }));
    } catch (e) {
      if (c.signal.aborted) return;
      setError(e instanceof Error ? e.message : 'Something went wrong. Please try again.');
    } finally {
      if (!c.signal.aborted) setLoading(false);
    }
  }, [limit]);

  useEffect(() => {
    load();
    return () => controller.current?.abort();
  }, [load]);

  return { data, error, loading, reload: load };
}
