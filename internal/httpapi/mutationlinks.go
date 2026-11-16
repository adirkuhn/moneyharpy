package httpapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// mutationLinkJSON is MoneyHarpy's summary of a document a transaction pays,
// as Moneybird's transaction list shows it: "REC 23-09-2026 · Supplier",
// its date and total, and its lines.
type mutationLinkJSON struct {
	PaymentID        string  `json:"payment_id"`
	DocumentType     string  `json:"document_type"`
	DocumentID       string  `json:"document_id"`
	Label            string  `json:"label"`
	Reference        string  `json:"reference"`
	ContactName      string  `json:"contact_name"`
	Date             *string `json:"date"`
	Total            string  `json:"total"`
	Currency         string  `json:"currency"`
	State            string  `json:"state"`
	PaymentReference *string `json:"payment_reference"`
	Details          string  `json:"details"`
	Price            string  `json:"price"`
}

// lineSummary sums up a document's lines as Moneybird does: "2 Coxinha,
// 1 Massa", cut off when long.
func lineSummary(lines []sales.Detail) string {
	parts := make([]string, 0, len(lines))
	for _, l := range lines {
		d := strings.Join(strings.Fields(l.Description), " ")
		if d == "" {
			continue
		}
		if a := strings.TrimSpace(l.Amount); a != "" && a != "1" {
			d = strings.TrimSuffix(strings.TrimSpace(a), " x") + " " + d
		}
		parts = append(parts, d)
	}
	out := strings.Join(parts, ", ")
	if r := []rune(out); len(r) > 200 {
		out = string(r[:199]) + "…"
	}
	return out
}

func day(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.DateOnly)
	return &s
}

// mutationLinks sums up, for each transaction, the documents its payments
// pay, loading each kind of document and the contacts at once. A document
// that cannot be read is left out.
func (s *server) mutationLinks(ctx context.Context, admin, user resource.ID, list []banking.Mutation) (map[resource.ID][]mutationLinkJSON, error) {
	var salesIDs, externalIDs, documentIDs []resource.ID
	for _, m := range list {
		for _, p := range m.Payments {
			switch p.InvoiceType {
			case "SalesInvoice":
				salesIDs = append(salesIDs, p.InvoiceID)
			case "ExternalSalesInvoice":
				externalIDs = append(externalIDs, p.InvoiceID)
			case "Document":
				documentIDs = append(documentIDs, p.InvoiceID)
			}
		}
	}
	type summary struct {
		kind, label, reference, state, currency string
		contact                                 resource.ID
		date                                    *time.Time
		total                                   sales.Totals
		paymentReference                        string
		lines                                   []sales.Detail
	}
	docs := map[resource.ID]summary{}
	var contactIDs []resource.ID
	if len(salesIDs) > 0 {
		invs, err := s.deps.Sales.InvoicesByID(ctx, admin, user, salesIDs)
		if err != nil {
			return nil, err
		}
		for _, inv := range invs {
			docs[inv.ID] = summary{kind: "SalesInvoice", label: "INV", reference: inv.InvoiceID, state: inv.State, currency: inv.Currency, contact: inv.ContactID,
				date: inv.InvoiceDate, total: inv.Totals, paymentReference: inv.PaymentReference, lines: inv.Lines}
			contactIDs = append(contactIDs, inv.ContactID)
		}
	}
	if len(externalIDs) > 0 {
		exts, err := s.deps.Sales.ExternalInvoicesByID(ctx, admin, user, externalIDs)
		if err != nil {
			return nil, err
		}
		for _, e := range exts {
			date := e.Date
			docs[e.ID] = summary{kind: "ExternalSalesInvoice", label: "EXT", reference: e.Reference, state: e.State, currency: e.Currency, contact: e.ContactID,
				date: &date, total: e.Totals, lines: e.Lines}
			contactIDs = append(contactIDs, e.ContactID)
		}
	}
	if len(documentIDs) > 0 {
		for _, kind := range []string{purchases.PurchaseInvoice, purchases.Receipt} {
			found, err := s.deps.Purchases.DocumentsByID(ctx, admin, user, kind, documentIDs)
			if err != nil {
				return nil, err
			}
			for _, d := range found {
				label := "PUR"
				if kind == purchases.Receipt {
					label = "REC"
				}
				date := d.Date
				docs[d.ID] = summary{kind: kind, label: label, reference: d.Reference, state: d.State, currency: d.Currency, contact: d.ContactID,
					date: &date, total: d.Totals, lines: d.Lines}
				if d.ContactID != 0 {
					contactIDs = append(contactIDs, d.ContactID)
				}
			}
		}
	}
	names := map[resource.ID]string{}
	for start := 0; start < len(contactIDs); start += contacts.MaxFetch {
		found, err := s.deps.Contacts.ContactsByID(ctx, admin, user, contactIDs[start:min(start+contacts.MaxFetch, len(contactIDs))])
		if err != nil {
			return nil, err
		}
		for _, c := range found {
			names[c.ID] = c.Name()
		}
	}
	out := map[resource.ID][]mutationLinkJSON{}
	for _, m := range list {
		for _, p := range m.Payments {
			d, ok := docs[p.InvoiceID]
			if !ok {
				continue
			}
			link := mutationLinkJSON{
				PaymentID: p.ID.String(), DocumentType: d.kind, DocumentID: p.InvoiceID.String(), Label: d.label, Reference: d.reference,
				ContactName: names[d.contact], Date: day(d.date), Total: d.total.InclTax.String(), Currency: d.currency, State: d.state,
				PaymentReference: nz(d.paymentReference), Details: lineSummary(d.lines), Price: p.Price.String(),
			}
			out[m.ID] = append(out[m.ID], link)
		}
	}
	return out, nil
}

// withLinks answers transactions with the documents they pay.
func (s *server) withLinks(ctx context.Context, admin, user resource.ID, list []banking.Mutation) ([]financialMutationJSON, error) {
	links, err := s.mutationLinks(ctx, admin, user, list)
	if err != nil {
		return nil, fmt.Errorf("sum up linked documents: %w", err)
	}
	out := toFinancialMutations(list)
	for i := range out {
		out[i].Links = links[list[i].ID]
		if out[i].Links == nil {
			out[i].Links = []mutationLinkJSON{}
		}
	}
	return out, nil
}
