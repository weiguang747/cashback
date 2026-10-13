import { useCallback } from 'react';

import { getTransactions, type Transaction } from './api';
import { deviceLocale } from './locale';
import { useApi } from './useApi';

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

/** Loads the signed-in user's transactions, newest first. */
export function useTransactions(limit?: number) {
  const fetcher = useCallback((signal: AbortSignal) => getTransactions({ limit, signal }), [limit]);
  return useApi(fetcher);
}
