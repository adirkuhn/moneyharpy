package banking

import (
	"time"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

// Amount is money in cents; see money.Amount.
type Amount = money.Amount

// Period is a range of days; see money.Period.
type Period = money.Period

// ParseAmount reads an amount; see money.ParseAmount.
func ParseAmount(s string) (Amount, error) { return money.ParseAmount(s) }

// ParsePeriod reads a period; see money.ParsePeriod.
func ParsePeriod(s string, today time.Time) (Period, error) { return money.ParsePeriod(s, today) }
