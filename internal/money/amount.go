package money

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Amount is money in cents. Positive amounts come in, negative ones go out.
type Amount int64

// maxAmount is Moneybird's bound on amounts: one billion.
const maxAmount Amount = 1_000_000_000_00

var errAmount = errors.New("is not an amount")

// ParseAmount reads "12.50", "-12,5" or "1000". At most two decimals.
func ParseAmount(s string) (Amount, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" && frac == "" || len(frac) > 2 || strings.ContainsAny(whole+frac, "+-") {
		return 0, errAmount
	}
	frac += strings.Repeat("0", 2-len(frac))
	if whole == "" {
		whole = "0"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w > int64(maxAmount/100) {
		return 0, errAmount
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, errAmount
	}
	a := Amount(w*100 + f)
	if a > maxAmount {
		return 0, errAmount
	}
	if neg {
		a = -a
	}
	return a, nil
}

// String formats the amount as Moneybird does: "12.5", "-3.0".
func (a Amount) String() string {
	sign := ""
	if a < 0 {
		sign, a = "-", -a
	}
	cents := a % 100
	switch {
	case cents == 0:
		return fmt.Sprintf("%s%d.0", sign, a/100)
	case cents%10 == 0:
		return fmt.Sprintf("%s%d.%d", sign, a/100, cents/10)
	default:
		return fmt.Sprintf("%s%d.%02d", sign, a/100, cents)
	}
}
