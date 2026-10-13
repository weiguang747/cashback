import type { Tone } from '@/theme';

/**
 * The merchants the user can pay. Deliberately kept in the app: the server
 * has no merchant table and records the id and name sent with each payment.
 */

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

/** The merchant with this id, or undefined (e.g. a stale deep link). */
export function findMerchant(id: string | undefined): Merchant | undefined {
  return merchants.find((m) => m.id === id);
}
