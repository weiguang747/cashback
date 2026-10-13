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

func TestParse(t *testing.T) {
	tests := []struct {
		value    string
		exponent int
		want     int64
		wantErr  bool
	}{
		{"35000", 2, 3_500_000, false},
		{"35000.5", 2, 3_500_050, false},
		{"35000.50", 2, 3_500_050, false},
		{"0.05", 2, 5, false},
		{"0035000", 2, 3_500_000, false},
		{"3990", 0, 3990, false},
		{"92233720368547758.07", 2, math.MaxInt64, false},
		{"1.234", 2, 0, true},
		{"1.5", 0, 0, true},
		{"0", 2, 0, true},
		{"0.00", 2, 0, true},
		{"-5", 2, 0, true},
		{"+5", 2, 0, true},
		{"1e3", 2, 0, true},
		{"1,000", 2, 0, true},
		{" 5", 2, 0, true},
		{".5", 2, 0, true},
		{"5.", 2, 0, true},
		{"", 2, 0, true},
		{"92233720368547758.08", 2, 0, true},
	}
	for _, tt := range tests {
		got, err := Parse(tt.value, tt.exponent)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("Parse(%q, %d) = %d, %v; want %d, err=%v", tt.value, tt.exponent, got, err, tt.want, tt.wantErr)
		}
		// Round trip: what Parse accepts, Decimal gives back (normalized).
		if err == nil {
			if back, _ := Parse(Money{Minor: got, Exponent: tt.exponent}.Decimal(), tt.exponent); back != got {
				t.Errorf("round trip of %q = %d, want %d", tt.value, back, got)
			}
		}
	}
}
