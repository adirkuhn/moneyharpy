package httpapi

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

type linkSuggestionJSON struct {
	BookingType      string  `json:"booking_type"`
	BookingID        string  `json:"booking_id"`
	Label            string  `json:"label"`
	Reference        string  `json:"reference"`
	ContactName      string  `json:"contact_name"`
	Date             *string `json:"date"`
	Total            string  `json:"total"`
	Open             string  `json:"open"`
	Currency         string  `json:"currency"`
	State            string  `json:"state"`
	PaymentReference *string `json:"payment_reference"`
	Details          string  `json:"details"`
	Price            string  `json:"price"`
}

// maxSuggestions caps the suggestions, as Moneybird's list is short.
const maxSuggestions = 25

// shown is what a suggestion shows besides what ranks it.
type shown struct {
	label, state, currency string
	total                  sales.Totals
	details                string
}

// ListFinancialMutationLinkSuggestions implements ServerInterface.
func (s *server) ListFinancialMutationLinkSuggestions(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, params ListFinancialMutationLinkSuggestionsParams) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	ctx, me := r.Context(), user(r).ID
	m, err := s.deps.Banking.Mutation(ctx, admin, rid, me)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	scope := "all"
	if params.Scope != nil {
		scope = string(*params.Scope)
	}
	query := strings.TrimSpace(deref(params.Query))
	candidates, extra, err := s.linkCandidates(ctx, admin, me, scope, query)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []linkSuggestionJSON{}
	for _, sg := range banking.Suggest(m, candidates, query, maxSuggestions) {
		e := extra[sg.Type+sg.ID.String()]
		reference := cmpOr(sg.Number, sg.Reference)
		if sg.Type == "LedgerAccount" {
			reference = sg.Text
		}
		out = append(out, linkSuggestionJSON{
			BookingType: sg.Type, BookingID: sg.ID.String(), Label: e.label, Reference: reference, ContactName: sg.ContactName, Date: day(sg.Date),
			Total: e.total.InclTax.String(), Open: sg.Open.String(), Currency: cmpOr(e.currency, m.Currency), State: e.state,
			PaymentReference: nz(sg.PaymentReference), Details: e.details, Price: sg.Price.String(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// linkCandidates gathers what a transaction can be linked to in the scope:
// the newest unpaid sales invoices, external invoices, incoming documents
// and VAT returns, and the categories transactions may be booked on, which only
// a query or the categories scope brings.
func (s *server) linkCandidates(ctx context.Context, admin, user resource.ID, scope, query string) ([]banking.Candidate, map[string]shown, error) {
	var out []banking.Candidate
	extra := map[string]shown{}
	var contactIDs []resource.ID
	add := func(c banking.Candidate, contact resource.ID, e shown) {
		out = append(out, c)
		extra[c.Type+c.ID.String()] = e
		if contact != 0 {
			contactIDs = append(contactIDs, contact)
		}
	}
	contactOf := map[string]resource.ID{}
	if scope == "all" || scope == "sales_invoices" {
		invs, _, err := s.deps.Sales.List(ctx, admin, user, sales.Query{States: []string{sales.Open, sales.Late, sales.Reminded, sales.PendingPayment}, PerPage: sales.MaxPerPage})
		if err != nil {
			return nil, nil, err
		}
		for _, inv := range invs {
			c := banking.Candidate{Type: "SalesInvoice", ID: inv.ID, Open: inv.Unpaid(), Number: inv.InvoiceID, Reference: inv.Reference, PaymentReference: inv.PaymentReference, Date: inv.InvoiceDate, Text: lineSummary(inv.Lines)}
			contactOf[c.Type+c.ID.String()] = inv.ContactID
			add(c, inv.ContactID, shown{label: "INV", state: inv.State, currency: inv.Currency, total: inv.Totals, details: c.Text})
		}
		exts, _, err := s.deps.Sales.ExternalList(ctx, admin, user, sales.Query{States: []string{sales.Open, sales.Late}, PerPage: sales.MaxPerPage})
		if err != nil {
			return nil, nil, err
		}
		for _, e := range exts {
			date := e.Date
			c := banking.Candidate{Type: "ExternalSalesInvoice", ID: e.ID, Open: e.Unpaid(), Reference: e.Reference, Date: &date, Text: lineSummary(e.Lines)}
			contactOf[c.Type+c.ID.String()] = e.ContactID
			add(c, e.ContactID, shown{label: "EXT", state: e.State, currency: e.Currency, total: e.Totals, details: c.Text})
		}
	}
	if scope == "all" || scope == "documents" {
		for _, kind := range []string{purchases.PurchaseInvoice, purchases.Receipt} {
			docs, _, err := s.deps.Purchases.List(ctx, admin, user, kind, purchases.Query{States: []string{purchases.Open, purchases.Late, purchases.PendingPayment}, PerPage: purchases.MaxPerPage})
			if err != nil {
				return nil, nil, err
			}
			label := "PUR"
			if kind == purchases.Receipt {
				label = "REC"
			}
			for _, d := range docs {
				date := d.Date
				c := banking.Candidate{Type: banking.DocumentType, ID: d.ID, Open: d.Unpaid(), Reference: d.Reference, Date: &date, Text: lineSummary(d.Lines)}
				contactOf[c.Type+c.ID.String()] = d.ContactID
				add(c, d.ContactID, shown{label: label, state: d.State, currency: d.Currency, total: d.Totals, details: c.Text})
			}
		}
	}
	if (scope == "all" || scope == "documents") && s.deps.VatReturns != nil {
		returns, err := s.deps.VatReturns.Returns(ctx, admin, user)
		if err != nil {
			return nil, nil, err
		}
		for _, v := range returns {
			if v.Unpaid() == 0 {
				continue
			}
			deadline := v.Deadline()
			period := v.Period.From.Format("20060102") + ".." + v.Period.To.Format("20060102")
			c := banking.Candidate{Type: banking.VatDocumentType, ID: v.ID, Open: v.Unpaid(), Reference: period, Date: &deadline, Text: "Belastingdienst omzetbelasting btw VAT"}
			add(c, 0, shown{label: "VAT", state: v.State(), total: sales.Totals{InclTax: v.Totals().Total}, details: period})
		}
	}
	if scope == "ledger_accounts" || (scope == "all" && query != "") {
		accounts, err := s.deps.Bookkeeping.LedgerAccounts(ctx, admin, user)
		if err != nil {
			return nil, nil, err
		}
		for _, a := range accounts {
			if !a.Active || !slices.Contains(a.AllowedDocumentTypes, "financial_mutation") {
				continue
			}
			add(banking.Candidate{Type: "LedgerAccount", ID: a.ID, Text: a.Name + " " + a.Code}, 0, shown{details: a.Description})
		}
	}
	names := map[resource.ID]string{}
	for start := 0; start < len(contactIDs); start += contacts.MaxFetch {
		found, err := s.deps.Contacts.ContactsByID(ctx, admin, user, contactIDs[start:min(start+contacts.MaxFetch, len(contactIDs))])
		if err != nil {
			return nil, nil, err
		}
		for _, c := range found {
			names[c.ID] = c.Name()
		}
	}
	for i := range out {
		out[i].ContactName = names[contactOf[out[i].Type+out[i].ID.String()]]
	}
	return out, extra, nil
}
