// Package money handles USD amounts as whole cents. Amounts travel as strings with two decimals ("1250.50")
// and are never floating-point numbers anywhere in the application.
package money

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// MaxCents is the largest amount numeric(12,2) holds: 9,999,999,999.99.
const MaxCents int64 = 999_999_999_999

var pattern = regexp.MustCompile(`^[0-9]{1,10}(\.[0-9]{1,2})?$`)

// Parse reads "35000", "35000.5" or "35000.50" into cents. It rejects signs, commas, exponents, spaces
// inside, more than two decimals and anything beyond numeric(12,2). Zero is accepted; callers decide whether
// an amount must be positive.
func Parse(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if !pattern.MatchString(raw) {
		return 0, false
	}
	whole, frac, _ := strings.Cut(raw, ".")
	for len(frac) < 2 {
		frac += "0"
	}
	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, false
	}
	cents, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, false
	}
	total := units*100 + cents
	return total, total <= MaxCents
}

// Format writes cents as a string with two decimals.
func Format(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}
