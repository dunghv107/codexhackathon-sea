package valueobject

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrInvalidMoney is returned for amounts that are not non-negative decimals with at most 2 fraction digits.
var ErrInvalidMoney = errors.New("invalid money amount")

// maxCents keeps amounts inside numeric(18,2).
const maxCents = 999_999_999_999_999_99

// ParseCents parses "123.45" (also "123", "123.4") into cents without going through float.
func ParseCents(s string) (int64, error) {
	whole, frac, hasDot := strings.Cut(strings.TrimSpace(s), ".")
	if whole == "" || len(frac) > 2 || (hasDot && frac == "") || !isDigits(whole) || !isDigits(frac) {
		return 0, ErrInvalidMoney
	}
	frac += strings.Repeat("0", 2-len(frac))
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w > maxCents/100 {
		return 0, ErrInvalidMoney
	}
	f, _ := strconv.ParseInt(frac, 10, 64)
	return w*100 + f, nil
}

// AddLineCents returns total + qty*unit, or ErrInvalidMoney when the result would not fit numeric(18,2)
// (it also keeps int64 from wrapping).
func AddLineCents(total, unit int64, qty int) (int64, error) {
	if qty < 1 || unit > (maxCents-total)/int64(qty) {
		return 0, ErrInvalidMoney
	}
	return total + unit*int64(qty), nil
}

// FormatCents formats cents as a 2-decimal string ("129.90").
func FormatCents(c int64) string {
	sign := ""
	if c < 0 {
		sign, c = "-", -c
	}
	return fmt.Sprintf("%s%d.%02d", sign, c/100, c%100)
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
