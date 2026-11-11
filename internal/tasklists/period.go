package tasklists

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Duration is an ISO 8601 duration in whole days, weeks, months and years,
// such as P1M or P10D.
type Duration struct{ Years, Months, Days int }

var durationPattern = regexp.MustCompile(`^P(?:(\d+)Y)?(?:(\d+)M)?(?:(\d+)W)?(?:(\d+)D)?$`)

var errDuration = errors.New("is not an ISO 8601 duration such as P1M, P3M, P1Y or P10D")

// ParseDuration reads an ISO 8601 duration of days, weeks, months and
// years.
func ParseDuration(s string) (Duration, error) {
	m := durationPattern.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(s)))
	if m == nil || s == "P" || strings.TrimSpace(s) == "P" {
		return Duration{}, errDuration
	}
	n := func(i int) int {
		v, _ := strconv.Atoi(m[i])
		return v
	}
	d := Duration{Years: n(1), Months: n(2), Days: n(3)*7 + n(4)}
	if d == (Duration{}) || d.Years > 100 || d.Months > 1200 || d.Days > 36600 {
		return Duration{}, errDuration
	}
	return d, nil
}

// After returns the day this long after day. Months keep the day within
// the month, so a month after March 31 is April 30.
func (d Duration) After(day time.Time) time.Time {
	first := time.Date(day.Year()+d.Years, day.Month()+time.Month(d.Months), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(day.Day(), last), 0, 0, 0, 0, time.UTC).AddDate(0, 0, d.Days)
}

// friendly are the names Moneybird takes for common periods.
var friendly = map[string]string{"monthly": "P1M", "quarterly": "P3M", "yearly": "P1Y", "none": ""}

// NormalizePeriod reads a template's period: an ISO 8601 duration or
// monthly, quarterly, yearly or none; it returns the duration, empty for
// none.
func NormalizePeriod(s string) (string, error) {
	s = strings.TrimSpace(s)
	if v, ok := friendly[strings.ToLower(s)]; ok || s == "" {
		return v, nil
	}
	s = strings.ToUpper(s)
	if _, err := ParseDuration(s); err != nil {
		return "", errors.New("is not an ISO 8601 duration (such as P1M) or one of monthly, quarterly, yearly, none")
	}
	return s, nil
}

// previous returns the whole period of this length before the one today
// falls in. Periods of months align to the year, such as quarters; others
// end yesterday.
func previous(today time.Time, period string) (*time.Time, *time.Time) {
	d, err := ParseDuration(period)
	if err != nil {
		return nil, nil
	}
	months := d.Years*12 + d.Months
	if d.Days == 0 && months > 0 && (months <= 12 && 12%months == 0 || months%12 == 0) {
		year := today.Year()
		var start time.Time
		if months <= 12 {
			block := (int(today.Month()) - 1) / months * months
			start = time.Date(year, time.Month(block+1), 1, 0, 0, 0, 0, time.UTC)
		} else {
			start = time.Date(year-year%(months/12), 1, 1, 0, 0, 0, 0, time.UTC)
		}
		begin := start.AddDate(0, -months, 0)
		end := start.AddDate(0, 0, -1)
		return &begin, &end
	}
	end := today.AddDate(0, 0, -1)
	begin := today.AddDate(-d.Years, -d.Months, -d.Days)
	return &begin, &end
}
