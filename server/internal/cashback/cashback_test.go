package cashback

import (
	"math"
	"testing"
)

// The launch campaign in sen: 5%, Rp 20.000 minimum, Rp 50.000 daily cap,
// whole-rupiah awards.
var launch = Rules{RateBps: 500, MinSpend: 2_000_000, DailyCap: 5_000_000, AwardIncrement: 100}

func TestAward(t *testing.T) {
	plenty := State{BudgetRemaining: 1_000_000_000}
	tests := []struct {
		name       string
		amount     int64
		state      State
		wantAward  int64
		wantReason Reason
	}{
		{"just below minimum", 1_999_900, plenty, 0, BelowMinimum},
		{"at minimum", 2_000_000, plenty, 100_000, Awarded},
		{"5% of Rp 35.000", 3_500_000, plenty, 175_000, Awarded},
		{"rounds down to whole rupiah", 2_123_456, plenty, 106_100, Awarded}, // 5% = Rp 1.061,7280
		{"partial at the daily cap", 3_500_000, State{AwardedToday: 4_900_000, BudgetRemaining: 1e9}, 100_000, Awarded},
		{"exactly fills the daily cap", 3_500_000, State{AwardedToday: 4_825_000, BudgetRemaining: 1e9}, 175_000, Awarded},
		{"daily cap already reached", 3_500_000, State{AwardedToday: 5_000_000, BudgetRemaining: 1e9}, 0, DailyCapReached},
		{"pays what's left of the budget", 5_000_000, State{BudgetRemaining: 120_000}, 120_000, Awarded}, // due Rp 2.500, Rp 1.200 left
		{"budget empty", 5_000_000, State{BudgetRemaining: 0}, 0, BudgetExhausted},
		{"budget left is less than a rupiah", 5_000_000, State{BudgetRemaining: 50}, 0, BudgetExhausted},
		{"tighter of cap and budget wins", 5_000_000, State{AwardedToday: 4_900_000, BudgetRemaining: 30_000}, 30_000, Awarded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := Award(tt.amount, launch, tt.state)
			if got != tt.wantAward || reason != tt.wantReason {
				t.Errorf("Award(%d) = %d, %s; want %d, %s", tt.amount, got, reason, tt.wantAward, tt.wantReason)
			}
		})
	}
}

func TestAwardNeverExceedsLimits(t *testing.T) {
	for amount := int64(0); amount <= 20_000_000; amount += 12_345 {
		for _, s := range []State{
			{AwardedToday: 0, BudgetRemaining: 1e9},
			{AwardedToday: 4_999_900, BudgetRemaining: 1e9},
			{AwardedToday: 0, BudgetRemaining: 77_700},
		} {
			got, _ := Award(amount, launch, s)
			if got < 0 || got > launch.DailyCap-s.AwardedToday || got > s.BudgetRemaining || got%launch.AwardIncrement != 0 {
				t.Fatalf("Award(%d, %+v) = %d breaks a limit", amount, s, got)
			}
		}
	}
}

func TestPercentOfDoesNotOverflow(t *testing.T) {
	if got, want := percentOf(math.MaxInt64, 500), int64(math.MaxInt64/20); got != want {
		t.Errorf("percentOf(MaxInt64, 5%%) = %d, want %d", got, want)
	}
	if got := percentOf(math.MaxInt64, 10000); got != math.MaxInt64 {
		t.Errorf("percentOf(MaxInt64, 100%%) = %d, want MaxInt64", got)
	}
}
