package money

import (
	"errors"
	"math/big"
	"strconv"
	"strings"
)

// UnitPrice is the price of one unit in millionths, as a line or product
// price may have up to six decimals, such as 0.0825 per kWh. What a line
// comes to is an Amount in cents.
type UnitPrice int64

// unitScale is how many millionths make one.
const unitScale = 1_000_000

// maxUnitPrice is Moneybird's bound on amounts: one billion.
const maxUnitPrice UnitPrice = 1_000_000_000 * unitScale

var errUnitPrice = errors.New("is not an amount")

// ParseUnitPrice reads "12.50", "-0,0825" or "1000": at most six decimals.
func ParseUnitPrice(s string) (UnitPrice, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	whole, frac, _ := strings.Cut(s, ".")
	frac = strings.TrimRight(frac, "0")
	if whole == "" && frac == "" && !strings.Contains(s, "0") || len(frac) > 6 || strings.ContainsAny(whole+frac, "+-.") {
		return 0, errUnitPrice
	}
	if whole == "" {
		whole = "0"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w > int64(maxUnitPrice/unitScale) {
		return 0, errUnitPrice
	}
	var f int64
	if frac != "" {
		if f, err = strconv.ParseInt(frac+strings.Repeat("0", 6-len(frac)), 10, 64); err != nil {
			return 0, errUnitPrice
		}
	}
	p := UnitPrice(w*unitScale + f)
	if p > maxUnitPrice {
		return 0, errUnitPrice
	}
	if neg {
		p = -p
	}
	return p, nil
}

// FromAmount is the unit price of an amount in cents.
func FromAmount(a Amount) UnitPrice { return UnitPrice(a) * unitScale / 100 }

// String formats the price as Moneybird does: "12.5", "0.0825", "3.0".
func (p UnitPrice) String() string {
	return p.format(1)
}

// Decimal formats the price for a document: at least its cents, "12.50",
// and the decimals it has beyond them, "0.0825".
func (p UnitPrice) Decimal() string {
	return p.format(2)
}

func (p UnitPrice) format(minDecimals int) string {
	sign := ""
	if p < 0 {
		sign, p = "-", -p
	}
	frac := strings.TrimRight(strconv.FormatInt(int64(p%unitScale)+unitScale, 10)[1:], "0")
	if len(frac) < minDecimals {
		frac += strings.Repeat("0", minDecimals-len(frac))
	}
	return sign + strconv.FormatInt(int64(p/unitScale), 10) + "." + frac
}

// Times is what qty thousandths of a unit come to, rounded to the cent half
// away from zero.
func (p UnitPrice) Times(qty int64) Amount {
	// price in millionths × quantity in thousandths, in cents: ÷ 10⁷.
	n := new(big.Int).Mul(big.NewInt(int64(p)), big.NewInt(qty))
	d := big.NewInt(10_000_000)
	q, r := new(big.Int).QuoRem(n, d, new(big.Int))
	// Half away from zero.
	if new(big.Int).Mul(new(big.Int).Abs(r), big.NewInt(2)).Cmp(d) >= 0 {
		if n.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return Amount(q.Int64())
}
