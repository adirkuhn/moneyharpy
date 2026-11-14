package recognize

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// dateHit is a date printed on a line, with its byte span.
type dateHit struct {
	t          time.Time
	start, end int
}

var months = func() map[string]time.Month {
	months := map[string]time.Month{}
	for m, names := range map[time.Month][]string{
		time.January:   {"januari", "january", "januar", "jänner", "janvier", "janeiro", "enero", "jan", "janv", "ene"},
		time.February:  {"februari", "february", "februar", "février", "fevrier", "fevereiro", "febrero", "feb", "févr", "fev", "fév"},
		time.March:     {"maart", "march", "märz", "maerz", "mars", "março", "marco", "marzo", "mrt", "mar", "mär", "mrz"},
		time.April:     {"april", "avril", "abril", "apr", "avr", "abr"},
		time.May:       {"mei", "may", "mai", "maio", "mayo"},
		time.June:      {"juni", "june", "juin", "junho", "junio", "jun"},
		time.July:      {"juli", "july", "juillet", "julho", "julio", "jul", "juil"},
		time.August:    {"augustus", "august", "août", "aout", "agosto", "aug", "ago"},
		time.September: {"september", "septembre", "setembro", "septiembre", "sep", "sept", "set"},
		time.October:   {"oktober", "october", "octobre", "outubro", "octubre", "okt", "oct", "out"},
		time.November:  {"november", "novembre", "novembro", "noviembre", "nov"},
		time.December:  {"december", "dezember", "décembre", "decembre", "dezembro", "diciembre", "dec", "dez", "déc", "dic"},
	} {
		for _, n := range names {
			months[n] = m
		}
	}
	return months
}()

var (
	numericDate = regexp.MustCompile(`\b(\d{1,2})([-./])(\d{1,2})([-./])(\d{4}|\d{2})\b`)
	isoDate     = regexp.MustCompile(`\b(\d{4})-(\d{1,2})-(\d{1,2})\b`)
	monthNames  = func() string {
		var names []string
		for n := range months {
			names = append(names, regexp.QuoteMeta(n))
		}
		sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
		return strings.Join(names, "|")
	}()
	dayMonthYear = regexp.MustCompile(`(?i)\b(\d{1,2})(?:e|st|nd|rd|th|º|\.)?\s*(?:de\s+)?(` + monthNames + `)\.?,?\s*(?:de\s+)?(\d{4})\b`)
	monthDayYear = regexp.MustCompile(`(?i)\b(` + monthNames + `)\.?\s+(\d{1,2})(?:st|nd|rd|th)?,?\s+(\d{4})\b`)
)

// findDates returns the dates printed on a line. Numeric dates are read day
// first (European) unless only month first makes sense (11/13/2026).
func findDates(s string) []dateHit {
	var out []dateHit
	add := func(start, end, y, m, d int) {
		if y < 100 {
			y += 2000
		}
		t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
		if m < 1 || m > 12 || d < 1 || t.Day() != d {
			return
		}
		for _, h := range out {
			if start < h.end && h.start < end {
				return
			}
		}
		out = append(out, dateHit{t: t, start: start, end: end})
	}
	for _, m := range isoDate.FindAllStringSubmatchIndex(s, -1) {
		add(m[0], m[1], atoi(group(s, m, 1)), atoi(group(s, m, 2)), atoi(group(s, m, 3)))
	}
	for _, m := range numericDate.FindAllStringSubmatchIndex(s, -1) {
		if group(s, m, 2) != group(s, m, 4) {
			continue
		}
		a, b := atoi(group(s, m, 1)), atoi(group(s, m, 3))
		if b > 12 && a <= 12 {
			a, b = b, a
		}
		add(m[0], m[1], atoi(group(s, m, 5)), b, a)
	}
	for _, m := range dayMonthYear.FindAllStringSubmatchIndex(s, -1) {
		add(m[0], m[1], atoi(group(s, m, 3)), int(months[strings.ToLower(group(s, m, 2))]), atoi(group(s, m, 1)))
	}
	for _, m := range monthDayYear.FindAllStringSubmatchIndex(s, -1) {
		add(m[0], m[1], atoi(group(s, m, 3)), int(months[strings.ToLower(group(s, m, 1))]), atoi(group(s, m, 2)))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].start < out[j].start })
	return out
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// plausible tells whether a document date is not far from today.
func plausible(t, today time.Time) bool {
	return !t.After(today.AddDate(1, 0, 0)) && !t.Before(today.AddDate(-10, 0, 0))
}

type labelKind int

const (
	labelDue labelKind = iota
	labelDate
	labelOther
	labelGeneric
)

var dateLabels = []struct {
	kind labelKind
	re   *regexp.Regexp
}{
	{labelDue, regexp.MustCompile(`(?i)vervaldatum|verval datum|uiterlijk|te betalen v[oó]{1,2}r|betalen v[oó]{1,2}r|due date|date due|payment due|due on|due by|pay by|f[äa]lligkeitsdatum|f[äa]llig am|f[äa]llig|zahlbar bis|data de vencimento|data limite|vencimento|date d'[ée]ch[ée]ance|[ée]ch[ée]ance|fecha de vencimiento|vencimiento`)},
	{labelOther, regexp.MustCompile(`(?i)leverdatum|leveringsdatum|orderdatum|besteldatum|delivery date|order date|leistungsdatum|lieferdatum|service date|periode|period|date de livraison|data de entrega`)},
	{labelDate, regexp.MustCompile(`(?i)factuurdatum|factuur datum|invoice date|date of issue|issue date|issued on|date issued|rechnungsdatum|data da fatura|data de emiss[aã]o|fecha de factura|fecha de emisi[oó]n|date de facture|date d'[ée]mission|bondatum|creditnotadatum|documentdatum|document date`)},
	{labelGeneric, regexp.MustCompile(`(?i)\b(?:datum|date|data|fecha|dato)\b`)},
}

type labelHit struct {
	kind       labelKind
	start, end int
}

// findDateLabels returns the date labels on a line, left to right, longer and
// more specific labels first.
func findDateLabels(s string) []labelHit {
	var out []labelHit
	for _, l := range dateLabels {
	next:
		for _, m := range l.re.FindAllStringIndex(s, -1) {
			for _, h := range out {
				if m[0] < h.end && h.start < m[1] {
					continue next
				}
			}
			out = append(out, labelHit{kind: l.kind, start: m[0], end: m[1]})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].start < out[j].start })
	return out
}

type datedLabel struct {
	kind labelKind
	t    time.Time
}

// labelledDates pairs date labels with the dates after them on the same line,
// or (for a header row of labels) with the dates on the next line.
func labelledDates(lines []string) []datedLabel {
	var out []datedLabel
	for i, s := range lines {
		labels := findDateLabels(s)
		if len(labels) == 0 {
			continue
		}
		dates := findDates(s)
		if len(dates) > 0 {
			used := map[int]bool{}
			for li, l := range labels {
				limit := len(s)
				if li+1 < len(labels) {
					limit = labels[li+1].start
				}
				for di, d := range dates {
					if !used[di] && d.start >= l.end && d.start < limit {
						used[di] = true
						out = append(out, datedLabel{l.kind, d.t})
						break
					}
				}
			}
			continue
		}
		j := nextNonEmpty(lines, i)
		if j < 0 {
			continue
		}
		next := findDates(lines[j])
		if len(next) == 0 {
			continue
		}
		if len(next) == len(labels) {
			for k, l := range labels {
				out = append(out, datedLabel{l.kind, next[k].t})
			}
			continue
		}
		for _, l := range labels {
			best, dist := -1, 1<<30
			for k, d := range next {
				if dd := absInt(d.start - l.start); dd < dist {
					best, dist = k, dd
				}
			}
			if best >= 0 && dist <= 8 {
				out = append(out, datedLabel{l.kind, next[best].t})
			}
		}
	}
	return out
}

func nextNonEmpty(lines []string, i int) int {
	for j := i + 1; j < len(lines) && j <= i+2; j++ {
		if strings.TrimSpace(lines[j]) != "" {
			return j
		}
	}
	return -1
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

var (
	termPattern = regexp.MustCompile(`(?i)(?:termijn|binnen|within|terms?|zahlbar|zahlungsziel|prazo|dentro|dans|payable|innerhalb|pagamento|betaling|payment)\D{0,30}?\b(\d{1,3})\s*(?:dagen|days|tagen?|dias|días|jours)\b`)
	netPattern  = regexp.MustCompile(`(?i)\bnet(?:to)?\s+(\d{1,3})\b`)
)

// termDays reads a payment term such as "betalingstermijn 14 dagen", "within
// 30 days", "zahlbar innerhalb von 14 Tagen" or "net 30".
func termDays(s string) (int, bool) {
	for _, re := range []*regexp.Regexp{termPattern, netPattern} {
		if m := re.FindStringSubmatch(s); m != nil {
			if n := atoi(m[1]); n > 0 && n <= 365 {
				return n, true
			}
		}
	}
	return 0, false
}

// dateCounts counts how often each plausible date is printed, and lists
// them in the order they first appear.
func dateCounts(lines []string, today time.Time) (map[time.Time]int, []time.Time) {
	counts := map[time.Time]int{}
	var order []time.Time
	for _, s := range lines {
		for _, d := range findDates(s) {
			if !plausible(d.t, today) {
				continue
			}
			if counts[d.t] == 0 {
				order = append(order, d.t)
			}
			counts[d.t]++
		}
	}
	return counts, order
}

// mostOften tells whether t is printed more often than any other date.
func mostOften(t time.Time, counts map[time.Time]int) bool {
	for o, n := range counts {
		if !o.Equal(t) && n >= counts[t] {
			return false
		}
	}
	return true
}

// checkMisread looks for another reading of the document date: a date
// printed elsewhere that differs in one digit, as OCR reads "20-10-2026" as
// "26-10-2026". Dates labelled as something else, such as the due date, do
// not count. When the other reading is printed more often it is taken
// instead; either way the date is then uncertain, unless it is itself
// printed more often.
func checkMisread(date Value[time.Time], counts map[time.Time]int, labelledElse map[time.Time]bool) Value[time.Time] {
	for o, n := range counts {
		if o.Equal(date.V) || labelledElse[o] || !oneDigitApart(o, date.V) {
			continue
		}
		switch {
		case n > counts[date.V]:
			return unsure(o)
		case n == counts[date.V]:
			date.Confidence = Uncertain
		}
	}
	return date
}

// oneDigitApart tells whether two dates written DDMMYYYY differ in exactly
// one digit.
func oneDigitApart(a, b time.Time) bool {
	x, y := a.Format("02012006"), b.Format("02012006")
	diff := 0
	for i := range len(x) {
		if x[i] != y[i] {
			diff++
		}
	}
	return diff == 1
}

// readDates finds the document date and due date.
func readDates(lines []string, today time.Time) (date, due Value[time.Time]) {
	set := func(v *Value[time.Time], t time.Time) {
		if plausible(t, today) {
			*v = sure(t)
		} else {
			*v = unsure(t)
		}
	}
	var generic *time.Time
	labelledElse := map[time.Time]bool{} // dates labelled as something else
	for _, d := range labelledDates(lines) {
		switch d.kind {
		case labelDate:
			if !date.Found {
				set(&date, d.t)
			}
		case labelDue:
			labelledElse[d.t] = true
			if !due.Found {
				set(&due, d.t)
			}
		case labelGeneric:
			if generic == nil {
				t := d.t
				generic = &t
			}
		case labelOther:
			labelledElse[d.t] = true
		}
	}
	counts, order := dateCounts(lines, today)
	if !date.Found && generic != nil {
		set(&date, *generic)
		date = checkMisread(date, counts, labelledElse)
	}
	if !date.Found {
		var all []time.Time
		for _, t := range order {
			if !due.Found || !t.Equal(due.V) {
				all = append(all, t)
			}
		}
		if len(all) > 0 {
			// The date printed most often; a till receipt prints it on
			// the receipt and again on the card payment slip.
			top := all[0]
			for _, t := range all[1:] {
				if counts[t] > counts[top] {
					top = t
				}
			}
			date = unsure(top)
			if len(all) == 1 || counts[top] >= 2 && mostOften(top, counts) {
				date.Confidence = Certain
			}
			date = checkMisread(date, counts, labelledElse)
		}
	}
	if !due.Found && date.Found {
		for _, s := range lines {
			if n, ok := termDays(s); ok {
				due = Value[time.Time]{V: date.V.AddDate(0, 0, n), Found: true, Confidence: date.Confidence}
				break
			}
		}
	}
	if due.Found && date.Found && due.V.Before(date.V) {
		due.Confidence = Uncertain
	}
	return date, due
}
