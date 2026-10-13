import type { Tone } from '@/theme';

/** Simulated data for the demo. Amounts are whole rupiah. */

export const account = {
  name: 'Andi Pratama',
  initials: 'AP',
  currency: 'IDR',
  balance: 12_450_000,
  cashback: 86_400,
};

export type TransactionKind = 'in' | 'out' | 'cashback';

export type Transaction = {
  id: string;
  name: string;
  /** e.g. "8:42 AM · Payment" */
  meta: string;
  amount: number;
  kind: TransactionKind;
};

export type TransactionGroup = { label: string; items: Transaction[] };

export const transactionGroups: TransactionGroup[] = [
  {
    label: 'Today',
    items: [
      { id: 't1', name: 'Kopi Kenangan', meta: '8:42 AM · Payment', amount: -35_000, kind: 'out' },
      { id: 't2', name: 'Sarah Wijaya', meta: '7:30 AM · Received', amount: 500_000, kind: 'in' },
    ],
  },
  {
    label: 'Yesterday',
    items: [
      { id: 't3', name: 'Indomaret', meta: '7:15 PM · Payment', amount: -248_500, kind: 'out' },
      { id: 't4', name: 'Gojek Ride', meta: '6:02 PM · Payment', amount: -42_000, kind: 'out' },
      { id: 't5', name: 'Cash back redeemed', meta: '9:10 AM · To available balance', amount: 64_000, kind: 'cashback' },
    ],
  },
  {
    label: '10 Oct',
    items: [
      { id: 't6', name: 'Salary · PT Acme', meta: '9:00 AM · Received', amount: 9_500_000, kind: 'in' },
      { id: 't7', name: 'PLN Electricity', meta: '8:15 AM · Bill', amount: -350_000, kind: 'out' },
    ],
  },
  {
    label: '9 Oct',
    items: [
      { id: 't8', name: 'Netflix', meta: '11:00 PM · Subscription', amount: -186_000, kind: 'out' },
      { id: 't9', name: 'Tokopedia', meta: '2:40 PM · Payment', amount: -129_000, kind: 'out' },
      { id: 't10', name: 'Budi Santoso', meta: '12:05 PM · Received', amount: 75_000, kind: 'in' },
    ],
  },
  {
    label: '8 Oct',
    items: [
      { id: 't11', name: 'Shell Fuel', meta: '6:20 PM · Payment', amount: -200_000, kind: 'out' },
      { id: 't12', name: 'Kopi Kenangan', meta: '8:35 AM · Payment', amount: -35_000, kind: 'out' },
    ],
  },
];

/** The short list shown on Home. */
export const recentTransactions: Transaction[] = [
  { id: 'r1', name: 'Kopi Kenangan', meta: 'Today · 8:42 AM', amount: -35_000, kind: 'out' },
  { id: 'r2', name: 'Sarah Wijaya', meta: 'Today · Received', amount: 500_000, kind: 'in' },
  { id: 'r3', name: 'Indomaret', meta: 'Yesterday · 7:15 PM', amount: -248_500, kind: 'out' },
  { id: 'r4', name: 'Gojek Ride', meta: 'Yesterday · 6:02 PM', amount: -42_000, kind: 'out' },
  { id: 'r5', name: 'Salary · PT Acme', meta: '10 Oct · Received', amount: 9_500_000, kind: 'in' },
  { id: 'r6', name: 'Netflix', meta: '9 Oct · Subscription', amount: -186_000, kind: 'out' },
];

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
