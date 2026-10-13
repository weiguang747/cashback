// Package money represents amounts exactly, as an integer count of a
// currency's minor unit (sen, cents). Floats never touch an amount.
package money

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Money is an exact amount in one currency.
type Money struct {
	// Minor is the amount in the currency's smallest unit, e.g. 3500000
	// for Rp 35.000,00. Negative for money out.
	Minor int64
	// Currency is the ISO 4217 code, e.g. "IDR".
	Currency string
	// Exponent is the number of decimal places of the currency's minor
	// unit (ISO 4217): 2 for IDR and USD, 0 for JPY.
	Exponent int
}

// Decimal returns the amount in major units as an exact decimal string,
// e.g. "-35000.00" for Minor -3500000 with Exponent 2.
func (m Money) Decimal() string {
	// Work on the magnitude as uint64 so math.MinInt64 doesn't overflow.
	neg := m.Minor < 0
	abs := uint64(m.Minor)
	if neg {
		abs = -abs
	}

	digits := strconv.FormatUint(abs, 10)
	if m.Exponent > 0 {
		if pad := m.Exponent + 1 - len(digits); pad > 0 {
			digits = strings.Repeat("0", pad) + digits
		}
		cut := len(digits) - m.Exponent
		digits = digits[:cut] + "." + digits[cut:]
	}
	if neg {
		return "-" + digits
	}
	return digits
}

type jsonMoney struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

// MarshalJSON writes {"value":"-35000.00","currency":"IDR"}. The value is a
// string so JSON parsers never round it through a float.
func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(jsonMoney{Value: m.Decimal(), Currency: m.Currency})
}

var decimalPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// Parse converts a positive decimal string in major units, e.g. "35000" or
// "10.99", to minor units for a currency with the given exponent. It is the
// exact inverse of Decimal and never goes through a float. It rejects signs,
// exponents, more fraction digits than the currency has, zero, and values
// that don't fit in int64.
func Parse(value string, exponent int) (int64, error) {
	if !decimalPattern.MatchString(value) {
		return 0, errors.New("amount must be a positive decimal number")
	}
	whole, frac, _ := strings.Cut(value, ".")
	if len(frac) > exponent {
		return 0, errors.New("amount has too many decimal places for its currency")
	}
	digits := strings.TrimLeft(whole+frac+strings.Repeat("0", exponent-len(frac)), "0")
	if digits == "" {
		return 0, errors.New("amount must be greater than zero")
	}
	minor, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, errors.New("amount is too large")
	}
	return minor, nil
}
