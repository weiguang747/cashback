import type { Money } from './api';
import { deviceLocale } from './locale';

// Hermes on iOS implements only part of Intl.NumberFormat: no
// formatToParts, and signDisplay and currencyDisplay: 'narrowSymbol' are
// ignored. Everything here works from plain format() output instead.

/**
 * Currencies shown without decimals when the amount is whole. ISO gives
 * IDR two decimals (sen), but rupiah prices are written as "Rp 35.000".
 */
const WHOLE_UNIT_DISPLAY = new Set(['IDR']);

const formatters = new Map<string, Intl.NumberFormat>();

function cached(key: string, make: () => Intl.NumberFormat) {
  let f = formatters.get(key);
  if (!f) {
    f = make();
    formatters.set(key, f);
  }
  return f;
}

function currencyFormat(locale: string, currency: string, digits: number) {
  return cached(`c|${locale}|${currency}|${digits}`, () =>
    new Intl.NumberFormat(locale, {
      style: 'currency',
      currency,
      minimumFractionDigits: digits,
      maximumFractionDigits: digits,
    }),
  );
}

/** Plain number with a fixed number of decimals and no grouping, e.g. "0.10". */
function plainFormat(locale: string, digits: number) {
  return cached(`p|${locale}|${digits}`, () =>
    new Intl.NumberFormat(locale, { useGrouping: false, minimumFractionDigits: digits, maximumFractionDigits: digits }),
  );
}

/**
 * How the locale writes a currency's symbol. `intl` is what Intl puts in
 * the text; `display` is what we show instead. The device region's own
 * currency gets its local symbol ("Rp" rather than "IDR"); foreign ones keep
 * the unambiguous form ("US$", "IDR").
 */
function currencySymbol(locale: string, currency: string) {
  const sample = currencyFormat(locale, currency, 0).format(1);
  const one = plainFormat(locale, 0).format(1);
  // Drop spaces and bidi marks (Arabic locales add U+200F/U+061C around the number).
  const intl = sample.replace(one, '').replace(/[\s؜‎‏]/g, '');
  const device = deviceLocale();
  const display = currency === device.currencyCode && device.currencySymbol ? device.currencySymbol : intl;
  return { intl, display, symbolFirst: sample.indexOf(intl) < sample.indexOf(one) };
}

/**
 * "Rp 35.000", "-US$11.99", "35.000,00 €" in the device locale. Pass `sign`
 * to prefix + to money in as well.
 *
 * Exact for any decimal string: only the integer part goes through a JS
 * number, and the fraction digits are copied from `money.value`.
 */
export function formatMoney(money: Money, opts: { sign?: boolean } = {}): string {
  const locale = deviceLocale().languageTag;
  const negative = money.value.startsWith('-');
  const [int = '0', frac = ''] = money.value.replace(/^[-+]/, '').split('.');
  const hasFraction = /[1-9]/.test(frac);
  const digits = WHOLE_UNIT_DISPLAY.has(money.currency) && !hasFraction ? 0 : frac.length;
  const nf = currencyFormat(locale, money.currency, digits);

  // For "+", format as negative and swap the locale's minus sign, so the
  // sign lands where the locale puts it.
  const plus = !!opts.sign && !negative && (hasFraction || Number(int) !== 0);
  const sign = negative || plus ? -1 : 1;

  let text: string;
  if (!hasFraction) {
    text = nf.format(sign * Number(int));
  } else {
    // Format int + 0.1 so the sign survives even for "-0.05", then swap
    // the placeholder fraction (".10" or ",10") for the real digits.
    const approx = nf.format(sign * (Number(int) + 0.1));
    const pf = plainFormat(locale, digits);
    const placeholder = pf.format(0.1).slice(-(digits + 1));
    const exact = pf.format(Number(`0.${frac}`)).slice(-(digits + 1));
    const at = approx.lastIndexOf(placeholder);
    text =
      at === -1
        ? nf.format(sign * Math.abs(Number(money.value)))
        : approx.slice(0, at) + exact + approx.slice(at + placeholder.length);
  }
  if (plus) text = text.replace(/[-−]/, '+');

  const symbol = currencySymbol(locale, money.currency);
  return symbol.intl && symbol.display !== symbol.intl ? text.replace(symbol.intl, symbol.display) : text;
}

/**
 * Splits formatted money into the currency symbol and the number, for
 * layouts that style them differently. `symbolFirst` is false in locales
 * that write "35.000,00 €".
 */
export function splitMoney(money: Money): { symbol: string; figure: string; symbolFirst: boolean } {
  const { display, symbolFirst } = currencySymbol(deviceLocale().languageTag, money.currency);
  const text = formatMoney(money);
  const escaped = display.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  return {
    symbol: display,
    figure: display ? text.replace(new RegExp(`\\s*${escaped}\\s*`), '') : text,
    symbolFirst,
  };
}

/** Wraps a whole-rupiah amount from the mock data as Money. */
export function idr(amount: number | string): Money {
  return { value: String(amount || '0'), currency: 'IDR' };
}

/** 5% cash back, rounded down to whole rupiah. */
export function cashbackFor(amount: number, rate = 0.05): number {
  return Math.floor(amount * rate);
}
