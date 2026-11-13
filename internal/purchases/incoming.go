package purchases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Incoming is one incoming document of any kind, as a row of the Incoming
// page: a purchase invoice, receipt, general document, journal entry
// (general journal document) or typeless document.
type Incoming struct {
	ID               resource.ID
	AdministrationID resource.ID
	// Kind is PurchaseInvoice, Receipt, GeneralDocument,
	// journal.GeneralJournalDocument or TypelessDocument.
	Kind      string
	Reference string
	// ContactID is zero without a contact; ContactName is then empty.
	ContactID   resource.ID
	ContactName string
	Date        time.Time
	// State is the state to show: new, saved, open, late, pending_payment
	// or paid.
	State string
	// Currency is the administration's for documents without one.
	Currency string
	// TotalInclTax is zero but for purchase invoices and receipts.
	TotalInclTax     money.Amount
	AttachmentsCount int
	CreatedAt        time.Time
}

// Ref locates the typed resource the document is.
func (d Incoming) Ref() resource.Ref {
	t := map[string]resource.Type{
		PurchaseInvoice: resource.TypePurchaseInvoice, Receipt: resource.TypeReceipt, GeneralDocument: resource.TypeGeneralDocument,
		journal.GeneralJournalDocument: resource.TypeGeneralJournalDocument, TypelessDocument: resource.TypeTypelessDocument,
	}[d.Kind]
	return resource.Ref{ID: d.ID, Type: t, AdministrationID: d.AdministrationID}
}

// IncomingQuery selects incoming documents of every kind.
type IncomingQuery struct {
	Query
	// Kinds keeps documents of these kinds; empty keeps every kind,
	// typeless documents too, which are of no type a filter can name.
	Kinds []string
}

// IncomingStatistics are the totals of the incoming documents a query
// selects.
type IncomingStatistics struct {
	Currency string
	// CostsExclTax is what the selected purchase invoices and receipts
	// book as costs: those that are not new, excluding VAT.
	CostsExclTax money.Amount
	// AmountPayable is what is unpaid on the open, late and pending ones,
	// including VAT.
	AmountPayable money.Amount
	// Categories are the costs per category, largest first.
	Categories []CostCategory
}

// CostCategory is what documents book on one category; a zero
// LedgerAccountID holds the lines without one.
type CostCategory struct {
	LedgerAccountID resource.ID
	Name            string
	Amount          money.Amount
}

// incomingTypes are the kinds a type filter names.
var incomingTypes = map[string]string{
	"purchase_invoice": PurchaseInvoice, "receipt": Receipt, "general_document": GeneralDocument,
	"general_journal_document": journal.GeneralJournalDocument,
}

// ParseIncomingFilter reads the filter of the Incoming page: the document
// filter's period, state, contact_id, attachment, recurring and
// automatic_saving, and type (all, purchase_invoice, receipt,
// general_document, general_journal_document). Without a filter the list
// holds this year's documents.
func ParseIncomingFilter(filter string, today time.Time) (IncomingQuery, error) {
	var verr resource.ValidationError
	var rest, kinds []string
	all := false
	for term := range strings.SplitSeq(filter, ",") {
		term = strings.TrimSpace(term)
		key, value, ok := strings.Cut(term, ":")
		switch {
		case ok && key == "type":
			for v := range strings.SplitSeq(value, "|") {
				if v == "all" {
					all = true
					continue
				}
				kind, known := incomingTypes[v]
				if !known {
					verr.Add("filter", "type "+v+" is not one of all, purchase_invoice, receipt, general_document, general_journal_document")
					continue
				}
				kinds = append(kinds, kind)
			}
		case ok && (key == "reference" || key == "ledger_account_id" || key == "updated_after"):
			verr.Add("filter", "has an unknown key: "+key)
		case term != "":
			rest = append(rest, term)
		}
	}
	q := Query{Today: today}
	if len(rest) > 0 || strings.TrimSpace(filter) == "" {
		var err error
		if q, err = ParseFilter(strings.Join(rest, ","), today); err != nil {
			var perr *resource.ValidationError
			if !errors.As(err, &perr) {
				return IncomingQuery{}, err
			}
			for _, problem := range perr.Fields["filter"] {
				verr.Add("filter", problem)
			}
		}
	}
	if all {
		kinds = nil
	}
	return IncomingQuery{Query: q, Kinds: kinds}, verr.OrNil()
}

// IncomingList returns the page of incoming documents of every kind the
// query selects, newest first unless q.Ascending.
func (s *Service) IncomingList(ctx context.Context, admin, user resource.ID, q IncomingQuery) ([]Incoming, int, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, 0, err
	}
	q.Today = s.today(a)
	pageOf(&q.Query)
	list, total, err := s.store.IncomingDocuments(ctx, admin, q)
	if err != nil {
		return nil, 0, fmt.Errorf("list incoming documents: %w", err)
	}
	for i := range list {
		list[i].Currency = cmpString(list[i].Currency, a.Currency)
	}
	return list, total, nil
}

// IncomingDocument returns the incoming document of any kind with the id.
func (s *Service) IncomingDocument(ctx context.Context, admin, id, user resource.ID) (Incoming, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Incoming{}, err
	}
	d, err := s.store.IncomingDocument(ctx, admin, id, s.today(a))
	if err != nil {
		return Incoming{}, err
	}
	d.Currency = cmpString(d.Currency, a.Currency)
	return d, nil
}

// IncomingStatistics sums up the incoming documents the query selects, in
// the administration's currency.
func (s *Service) IncomingStatistics(ctx context.Context, admin, user resource.ID, q IncomingQuery) (IncomingStatistics, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return IncomingStatistics{}, err
	}
	q.Today, q.Page, q.PerPage = s.today(a), 0, 0
	st, err := s.store.IncomingStatistics(ctx, admin, q)
	if err != nil {
		return IncomingStatistics{}, fmt.Errorf("sum up incoming documents: %w", err)
	}
	st.Currency = a.Currency
	return st, nil
}
