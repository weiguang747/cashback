/**
 * Switches for the simulated demo. These mirror the Tweaks in the design so
 * every state can be shown without a backend.
 */
export const demo = {
  /** Outcome of a payment that is within the balance. */
  paymentOutcome: 'success' as 'success' | 'error',
  /** Delay before a simulated payment or redemption resolves. */
  processingMs: 1600,

  campaign: 'active' as 'active' | 'ended',
  cashbackRate: 0.05,
  dailyCap: 25000,
  earnedToday: 18000,

  /** Set to false to see the Home empty state. */
  hasTransactions: true,
};
