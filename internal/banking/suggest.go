package banking

import (
	"cmp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Candidate is something a transaction can be linked to: an invoice or
// document that is not fully paid, or a category.
type Candidate struct {
	// Type is the booking type to link it with: SalesInvoice,
	// ExternalSalesInvoice, Document or LedgerAccount.
	Type string
	ID   resource.ID
	// Open is what is still to be received or paid; negative for a credit
	// invoice, zero for a category.
	Open Amount
	// Number, Reference and PaymentReference are what a transaction's
	// message may name it by.
	Number, Reference, PaymentReference string
	ContactName                         string
	Date                                *time.Time
	// Text is everything else a search may find it by, such as its lines or
	// a category's name.
	Text string
}

// Suggestion is a candidate with the amount linking it books, as the price
// of link_booking: positive pays the invoice or books in the transaction's
// direction.
type Suggestion struct {
	Candidate
	Price Amount
	score int
}

// compact lowers and strips what is not a letter or digit, so "RF18 0000"
// is found in "rf180000".
func compact(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

// words are the lowered words of s of at least three characters.
func words(s string) []string {
	var out []string
	for w := range strings.FieldsFuncSeq(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len([]rune(w)) >= 3 {
			out = append(out, w)
		}
	}
	return out
}

// Suggest ranks what the transaction can be linked to, after Moneybird's
// suggestions: a payment reference or number in its message first, then
// an open amount equal to what is left, the contact named like the
// counterparty, money going the way the invoice expects and a nearby date.
// With a query only candidates holding every word of it are kept.
func Suggest(m Mutation, candidates []Candidate, query string, limit int) []Suggestion {
	open := m.Open()
	message := compact(m.Message + " " + m.BatchReference)
	contra := words(m.ContraName)
	terms := strings.Fields(strings.ToLower(query))
	out := make([]Suggestion, 0, len(candidates))
	for _, c := range candidates {
		if len(terms) > 0 {
			hay := strings.ToLower(strings.Join([]string{c.Number, c.Reference, c.PaymentReference, c.ContactName, c.Text, c.Open.String()}, " "))
			missing := slices.ContainsFunc(terms, func(t string) bool { return !strings.Contains(hay, t) && !strings.Contains(compact(hay), compact(t)) })
			if missing {
				continue
			}
		}
		s := Suggestion{Candidate: c}
		if c.Type == "LedgerAccount" {
			s.Price = open * m.direction()
			out = append(out, s)
			continue
		}
		// A credit invoice's open amount is negative: money goes the other way.
		left := open * invoiceSign(c.Type) // what is left, as the price of this candidate
		size := min(abs(c.Open), abs(left))
		s.Price = size
		if left < 0 {
			s.Price = -size
		}
		if (left >= 0) == (c.Open >= 0) {
			s.score += 20
		}
		for _, ref := range []string{c.PaymentReference, c.Number, c.Reference} {
			if r := compact(ref); len(r) >= 4 && strings.Contains(message, r) {
				s.score += 100
				break
			}
		}
		if abs(c.Open) == abs(open) {
			s.score += 60
		}
		if name := words(c.ContactName); len(name) > 0 && slices.ContainsFunc(name, func(w string) bool { return slices.Contains(contra, w) }) {
			s.score += 30
		}
		if c.Date != nil {
			days := abs(Amount(m.Date.Sub(*c.Date).Hours() / 24))
			if days <= 60 {
				s.score += int(10 - days/6)
			}
		}
		out = append(out, s)
	}
	slices.SortStableFunc(out, func(a, b Suggestion) int {
		return cmp.Or(cmp.Compare(b.score, a.score), compareDates(b.Date, a.Date))
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func compareDates(a, b *time.Time) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	return a.Compare(*b)
}
