// Package cashback decides how much cash back a payment earns under a
// campaign's rules. It is pure arithmetic on minor units; the database layer
// supplies the numbers and records the result.
package cashback

// Reason explains an award, so the app can tell users why they got what
// they got.
type Reason string

const (
	Awarded         Reason = "awarded"
	BelowMinimum    Reason = "below_minimum"
	DailyCapReached Reason = "daily_cap_reached"
	BudgetExhausted Reason = "budget_exhausted"
	// Unavailable: the budget couldn't be checked (the Redis gate failed or
	// is briefly closed), so the payment went ahead without cash back.
	Unavailable Reason = "unavailable"
)

// Rules are a campaign's terms, in minor units.
type Rules struct {
	RateBps        int64 // 500 = 5%
	MinSpend       int64 // per payment
	DailyCap       int64 // per user per day
	AwardIncrement int64 // awards are rounded down to a multiple of this (100 = whole rupiah)
}

// State is what's already been used when the payment arrives.
type State struct {
	AwardedToday    int64 // this user, this campaign day
	BudgetRemaining int64 // the campaign account's balance
}

// Award returns the cash back for a payment of amount (minor units): the
// rate applied and rounded down, then limited by what's left of the user's
// daily cap and of the campaign budget. When the budget can't cover the full
// award, it pays what's left.
func Award(amount int64, r Rules, s State) (int64, Reason) {
	if amount < r.MinSpend {
		return 0, BelowMinimum
	}
	award := roundDown(percentOf(amount, r.RateBps), r.AwardIncrement)
	if award == 0 {
		return 0, BelowMinimum // too small to earn even one increment
	}

	reason := Awarded
	if left := r.DailyCap - s.AwardedToday; award >= left {
		award, reason = roundDown(max(left, 0), r.AwardIncrement), DailyCapReached
	}
	if award >= s.BudgetRemaining {
		award, reason = roundDown(max(s.BudgetRemaining, 0), r.AwardIncrement), BudgetExhausted
	}
	if award > 0 {
		// A partial award at a limit still counts as awarded; the limit
		// matters to the user only when it leaves them with nothing.
		return award, Awarded
	}
	return 0, reason
}

// percentOf is floor(amount * bps / 10000) without overflowing int64.
func percentOf(amount, bps int64) int64 {
	return amount/10000*bps + amount%10000*bps/10000
}

func roundDown(v, increment int64) int64 {
	if increment <= 1 {
		return v
	}
	return v - v%increment
}
