package banking

import (
	"errors"
	"strings"
	"time"
)

// Period is a range of days, both ends included.
type Period struct{ From, To time.Time }

// Contains reports whether day falls in the period.
func (p Period) Contains(day time.Time) bool { return !day.Before(p.From) && !day.After(p.To) }

var errPeriod = errors.New("is not a period")

// ParsePeriod reads Moneybird's periods: a name such as this_month or
// prev_year relative to today, or a range like 202401..202403 or
// 20240101..20240131.
func ParsePeriod(s string, today time.Time) (Period, error) {
	y, m, d := today.Date()
	today = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	month := func(t time.Time, add int) time.Time {
		return time.Date(t.Year(), t.Month()+time.Month(add), 1, 0, 0, 0, 0, time.UTC)
	}
	span := func(from, next time.Time) Period { return Period{From: from, To: next.AddDate(0, 0, -1)} }
	if rel, unit, ok := strings.Cut(s, "_"); ok {
		shift := map[string]int{"this": 0, "prev": -1, "next": 1}
		n, known := shift[rel]
		if known {
			switch unit {
			case "week":
				weekday := (int(today.Weekday()) + 6) % 7
				from := today.AddDate(0, 0, -weekday+7*n)
				return Period{From: from, To: from.AddDate(0, 0, 6)}, nil
			case "month":
				return span(month(today, n), month(today, n+1)), nil
			case "quarter":
				q := month(today, -int(today.Month()-1)%3)
				return span(month(q, 3*n), month(q, 3*n+3)), nil
			case "year":
				from := time.Date(y+n, 1, 1, 0, 0, 0, 0, time.UTC)
				return span(from, from.AddDate(1, 0, 0)), nil
			}
		}
	}
	a, b, ok := strings.Cut(s, "..")
	if !ok || len(a) != len(b) {
		return Period{}, errPeriod
	}
	var p Period
	switch len(a) {
	case 6:
		from, err1 := time.Parse("200601", a)
		to, err2 := time.Parse("200601", b)
		if err1 != nil || err2 != nil {
			return Period{}, errPeriod
		}
		p = span(from, month(to, 1))
	case 8:
		from, err1 := time.Parse("20060102", a)
		to, err2 := time.Parse("20060102", b)
		if err1 != nil || err2 != nil {
			return Period{}, errPeriod
		}
		p = Period{From: from, To: to}
	default:
		return Period{}, errPeriod
	}
	low, high := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	if p.To.Before(p.From) || p.From.Before(low) || p.To.After(high) || p.To.After(p.From.AddDate(10, 0, 0)) {
		return Period{}, errPeriod
	}
	return p, nil
}
