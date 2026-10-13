/** Groups digits with dots, the Indonesian convention: 12450000 → "12.450.000". */
export function formatNumber(value: number | string): string {
  const digits = String(value).replace(/\D/g, '');
  if (digits === '') return '0';
  return digits.replace(/^0+(?=\d)/, '').replace(/\B(?=(\d{3})+(?!\d))/g, '.');
}

/** "Rp 12.450.000". Pass `sign` to prefix + or -. */
export function formatIDR(value: number, opts: { sign?: boolean } = {}): string {
  const abs = Math.abs(Math.round(value));
  const prefix = opts.sign ? (value < 0 ? '-' : '+') : value < 0 ? '-' : '';
  return `${prefix}Rp ${formatNumber(abs)}`;
}

/** 5% cash back, rounded down to whole rupiah. */
export function cashbackFor(amount: number, rate = 0.05): number {
  return Math.floor(amount * rate);
}
