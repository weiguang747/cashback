import type { Tone } from '@/theme';

/** Simulated data for the demo. Amounts are whole rupiah. */

export const account = {
  name: 'Andi Pratama',
  initials: 'AP',
  currency: 'IDR',
  balance: 12_450_000,
  cashback: 86_400,
};

export type Merchant = { id: string; name: string; category: string; tone: Tone };

export const merchants: Merchant[] = [
  { id: 'kopi-senja', name: 'Kopi Senja', category: 'Coffee shop', tone: 'reward' },
  { id: 'warung-bu-tini', name: 'Warung Bu Tini', category: 'Restaurant', tone: 'reward' },
  { id: 'martabak-pak-udin', name: 'Martabak Pak Udin', category: 'Street food', tone: 'reward' },
  { id: 'toko-maju-jaya', name: 'Toko Maju Jaya', category: 'Convenience store', tone: 'positive' },
  { id: 'pasar-segar', name: 'Pasar Segar', category: 'Grocery', tone: 'positive' },
  { id: 'laundry-bersih', name: 'Laundry Bersih', category: 'Laundry', tone: 'navy' },
  { id: 'bengkel-motor-jaya', name: 'Bengkel Motor Jaya', category: 'Motorbike repair', tone: 'navy' },
  { id: 'apotek-sehat', name: 'Apotek Sehat', category: 'Pharmacy', tone: 'rose' },
];

export function findMerchant(id: string | undefined): Merchant {
  return merchants.find((m) => m.id === id) ?? merchants[0];
}

export const payment = {
  date: '13 Oct 2026, 14:32',
  source: 'Main balance',
  reference: 'TRX 2410 1308 4421',
};
