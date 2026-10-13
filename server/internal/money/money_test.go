package money

import (
	"encoding/json"
	"math"
	"testing"
)

func TestDecimal(t *testing.T) {
	tests := []struct {
		minor    int64
		exponent int
		want     string
	}{
		{-3_500_000, 2, "-35000.00"},
		{3_500_000, 2, "35000.00"},
		{-5, 2, "-0.05"},
		{5, 2, "0.05"},
		{50, 2, "0.50"},
		{0, 2, "0.00"},
		{1200, 0, "1200"},
		{-1200, 0, "-1200"},
		{12345, 3, "12.345"},
		{9_007_199_254_740_993, 2, "90071992547409.93"},
		{math.MaxInt64, 2, "92233720368547758.07"},
		{math.MinInt64, 2, "-92233720368547758.08"},
	}
	for _, tt := range tests {
		got := Money{Minor: tt.minor, Currency: "XXX", Exponent: tt.exponent}.Decimal()
		if got != tt.want {
			t.Errorf("Decimal(%d, exp %d) = %q, want %q", tt.minor, tt.exponent, got, tt.want)
		}
	}
}

func TestMarshalJSON(t *testing.T) {
	b, err := json.Marshal(Money{Minor: -3_500_000, Currency: "IDR", Exponent: 2})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"value":"-35000.00","currency":"IDR"}`; string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
}
