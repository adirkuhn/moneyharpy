package purchases

import (
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Query selects documents, after Moneybird's document filter.
type Query struct {
	// Kind is PurchaseInvoice or Receipt for booked documents.
	Kind string
	// States keeps documents in one of them; late and pending_payment are
	// worked out, so an open document past its due date counts as late.
	States          []string
	Period          *money.Period
	References      []string
	ContactID       *resource.ID
	LedgerAccountID *resource.ID
	UpdatedAfter    *time.Time
	// Attachment is with or without; empty means all.
	Attachment string
	// None selects nothing, for terms no document matches, such as
	// recurring:enabled.
	None          bool
	Ascending     bool
	Page, PerPage int
	// Today decides which open documents are late.
	Today time.Time
}

// ParseFilter reads Moneybird's document filter: period, state, recurring,
// attachment, automatic_saving, reference, contact_id, ledger_account_id
// and updated_after. Without a filter the list holds this year's documents.
// Documents never recur and MoneyHarpy saves none by itself, so
// recurring:enabled and automatic_saving:automatic select nothing.
func ParseFilter(filter string, today time.Time) (Query, error) {
	q := Query{Today: today}
	if strings.TrimSpace(filter) == "" {
		filter = "period:this_year"
	}
	var verr resource.ValidationError
	for term := range strings.SplitSeq(filter, ",") {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		key, value, ok := strings.Cut(term, ":")
		if !ok {
			verr.Add("filter", "has a term without a value: "+term)
			continue
		}
		switch key {
		case "period":
			p, err := money.ParsePeriod(value, today)
			if err != nil {
				verr.Add("filter", "period "+err.Error())
				continue
			}
			q.Period = &p
		case "state":
			for st := range strings.SplitSeq(value, "|") {
				switch {
				case st == "all":
				case slices.Contains(States, st):
					q.States = append(q.States, st)
				default:
					verr.Add("filter", "state "+st+" is not one of all, "+strings.Join(States, ", "))
				}
			}
		case "recurring", "attachment", "automatic_saving":
			parseChoice(&q, &verr, key, value)
		case "reference":
			q.References = strings.Split(value, "|")
		case "contact_id", "ledger_account_id":
			id, err := resource.ParseID(value)
			if err != nil {
				verr.Add("filter", key+" is not an id")
				continue
			}
			if key == "contact_id" {
				q.ContactID = &id
			} else {
				q.LedgerAccountID = &id
			}
		case "updated_after":
			t, err := time.Parse(time.RFC3339, value)
			if err != nil {
				verr.Add("filter", "updated_after is not an ISO 8601 time")
				continue
			}
			q.UpdatedAfter = &t
		default:
			verr.Add("filter", "has an unknown key: "+key)
		}
	}
	return q, verr.OrNil()
}

// parseChoice reads the terms that choose between a few values, which may
// be combined with a pipe.
func parseChoice(q *Query, verr *resource.ValidationError, key, value string) {
	allowed := map[string][]string{
		"recurring": {"all", "enabled", "disabled"}, "attachment": {"all", "with", "without"}, "automatic_saving": {"all", "automatic", "manual"},
	}[key]
	values := strings.Split(value, "|")
	for _, v := range values {
		if !slices.Contains(allowed, v) {
			verr.Add("filter", key+" is not one of "+strings.Join(allowed, ", "))
			return
		}
	}
	if slices.Contains(values, "all") {
		return
	}
	switch key {
	case "recurring":
		q.None = q.None || !slices.Contains(values, "disabled")
	case "automatic_saving":
		q.None = q.None || !slices.Contains(values, "manual")
	case "attachment":
		if len(values) == 1 {
			q.Attachment = values[0]
		}
	}
}

// effectiveState is the state to show: an open document a batched purchase
// transaction pays is pending payment, and one past its due date is late.
func effectiveState(state string, due *time.Time, pending bool, today time.Time) string {
	switch {
	case state != Open:
		return state
	case pending:
		return PendingPayment
	case due != nil && due.Before(today):
		return Late
	}
	return Open
}
