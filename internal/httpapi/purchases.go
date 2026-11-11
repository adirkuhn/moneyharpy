package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// Kinds of incoming document attachments a download link names.
const (
	filePurchaseInvoiceAttachment = "purchase_invoice_attachment"
	fileReceiptAttachment         = "receipt_attachment"
	fileGeneralDocumentAttachment = "general_document_attachment"
	fileTypelessAttachment        = "typeless_document_attachment"
)

// openIncomingAttachment opens an attachment of an incoming document, as
// the user a link names.
func (s *server) openIncomingAttachment(r *http.Request, l fileLink) (attachments.Attachment, io.ReadCloser, error) {
	ctx := r.Context()
	switch l.Kind {
	case filePurchaseInvoiceAttachment:
		return s.deps.Purchases.OpenAttachment(ctx, l.Admin, l.User, purchases.PurchaseInvoice, l.ID, l.Attachment)
	case fileReceiptAttachment:
		return s.deps.Purchases.OpenAttachment(ctx, l.Admin, l.User, purchases.Receipt, l.ID, l.Attachment)
	case fileGeneralDocumentAttachment:
		return s.deps.Purchases.OpenGeneralAttachment(ctx, l.Admin, l.ID, l.Attachment, l.User)
	default:
		return s.deps.Purchases.OpenTypelessAttachment(ctx, l.Admin, l.ID, l.Attachment, l.User)
	}
}

type fiscalAllocationJSON struct {
	ID                     string    `json:"id"`
	AdministrationID       string    `json:"administration_id"`
	LedgerAccountID        string    `json:"ledger_account_id"`
	PrivateLedgerAccountID string    `json:"private_ledger_account_id"`
	Percentage             string    `json:"percentage"`
	Price                  string    `json:"price"`
	PriceBase              string    `json:"price_base"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type purchaseDocumentJSON struct {
	ID                    string                 `json:"id"`
	AdministrationID      string                 `json:"administration_id"`
	ContactID             *string                `json:"contact_id"`
	Contact               *contactJSON           `json:"contact"`
	Reference             *string                `json:"reference"`
	Date                  string                 `json:"date"`
	DueDate               *string                `json:"due_date"`
	EntryNumber           int                    `json:"entry_number"`
	State                 string                 `json:"state"`
	Currency              string                 `json:"currency"`
	ExchangeRate          string                 `json:"exchange_rate"`
	RevenueInvoice        bool                   `json:"revenue_invoice"`
	PricesAreInclTax      bool                   `json:"prices_are_incl_tax"`
	Origin                *string                `json:"origin"`
	PaidAt                *string                `json:"paid_at"`
	TaxNumber             *string                `json:"tax_number"`
	TotalPriceExclTax     string                 `json:"total_price_excl_tax"`
	TotalPriceExclTaxBase string                 `json:"total_price_excl_tax_base"`
	TotalPriceInclTax     string                 `json:"total_price_incl_tax"`
	TotalPriceInclTaxBase string                 `json:"total_price_incl_tax_base"`
	TotalTax              string                 `json:"total_tax"`
	TotalPaid             string                 `json:"total_paid"`
	TotalUnpaid           string                 `json:"total_unpaid"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
	Version               int64                  `json:"version"`
	Details               []salesDetailJSON      `json:"details"`
	FiscalAllocations     []fiscalAllocationJSON `json:"fiscal_allocations"`
	Payments              []paymentJSON          `json:"payments"`
	Notes                 []noteJSON             `json:"notes"`
	Attachments           []attachmentJSON       `json:"attachments"`
	Events                []any                  `json:"events"`
	TaxTotals             []taxTotalJSON         `json:"tax_totals"`
	URI                   string                 `json:"uri"`
}

func (s *server) toPurchaseDocument(d purchases.Document, contact *contacts.Contact, today time.Time) purchaseDocumentJSON {
	out := purchaseDocumentJSON{
		ID: d.ID.String(), AdministrationID: d.AdministrationID.String(), Reference: nz(d.Reference), Date: d.Date.Format(time.DateOnly),
		DueDate: dateText(d.DueDate), EntryNumber: d.EntryNumber, State: d.EffectiveState(today), Currency: d.Currency, ExchangeRate: "1.0",
		RevenueInvoice: d.RevenueInvoice, PricesAreInclTax: d.PricesAreInclTax, Origin: nz(d.Origin), PaidAt: dateText(d.PaidAt),
		TotalPriceExclTax: d.Totals.ExclTax.String(), TotalPriceExclTaxBase: d.Totals.ExclTax.String(), TotalPriceInclTax: d.Totals.InclTax.String(),
		TotalPriceInclTaxBase: d.Totals.InclTax.String(), TotalTax: d.Totals.Tax.String(), TotalPaid: d.Paid().String(), TotalUnpaid: d.Unpaid().String(),
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, Version: d.Version(), Details: []salesDetailJSON{}, FiscalAllocations: []fiscalAllocationJSON{},
		Payments: []paymentJSON{}, Notes: []noteJSON{}, Attachments: []attachmentJSON{}, Events: []any{}, TaxTotals: []taxTotalJSON{}, URI: d.Ref().URI(),
	}
	if d.ContactID != 0 {
		out.ContactID = ptr(d.ContactID.String())
	}
	if contact != nil {
		c := s.toContact(*contact)
		out.Contact = &c
	}
	for i, l := range d.Lines {
		base := "0.0"
		if i < len(d.Totals.LineBases) {
			base = d.Totals.LineBases[i].String()
		}
		out.Details = append(out.Details, salesDetailJSON{
			ID: l.ID.String(), AdministrationID: d.AdministrationID.String(), TaxRateID: l.TaxRateID.String(), LedgerAccountID: deref(idString(l.LedgerAccountID)),
			ProjectID: idString(l.ProjectID), ProductID: idString(l.ProductID), Amount: l.Amount, AmountDecimal: quantityText(l.Quantity),
			Description: l.Description, Price: l.Price.String(), Period: nz(l.Period), RowOrder: l.RowOrder, TotalPriceExclTaxWithDisc: base,
			TotalPriceExclTaxWithDiscBs: base, TaxReportReference: []string{}, IsSelected: true, CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
		})
	}
	for _, f := range d.FiscalAllocations {
		out.FiscalAllocations = append(out.FiscalAllocations, fiscalAllocationJSON{
			ID: f.ID.String(), AdministrationID: d.AdministrationID.String(), LedgerAccountID: f.LedgerAccountID.String(),
			PrivateLedgerAccountID: f.PrivateLedgerAccountID.String(), Percentage: percentText(f.Percentage), Price: f.Price.String(),
			PriceBase: f.Price.String(), CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
		})
	}
	for _, p := range d.Payments {
		out.Payments = append(out.Payments, toPayment(p))
	}
	for _, r := range d.Totals.ByRate {
		out.TaxTotals = append(out.TaxTotals, taxTotalJSON{TaxRateID: r.TaxRateID.String(), TaxableAmount: r.Taxable.String(), TaxableAmountBase: r.Taxable.String(), TaxAmount: r.Tax.String(), TaxAmountBase: r.Tax.String()})
	}
	for _, n := range d.Notes {
		out.Notes = append(out.Notes, toNote(n))
	}
	for _, a := range d.Attachments {
		out.Attachments = append(out.Attachments, toAttachment(a))
	}
	return out
}

// contactsOf loads the contacts with these ids at once.
func (s *server) contactsOf(r *http.Request, admin resource.ID, ids []resource.ID) (map[resource.ID]*contacts.Contact, error) {
	byID := map[resource.ID]*contacts.Contact{}
	ids = nonZeroIDs(ids)
	for start := 0; start < len(ids); start += contacts.MaxFetch {
		found, err := s.deps.Contacts.ContactsByID(r.Context(), admin, user(r).ID, ids[start:min(start+contacts.MaxFetch, len(ids))])
		if err != nil {
			return nil, err
		}
		for i := range found {
			byID[found[i].ID] = &found[i]
		}
	}
	return byID, nil
}

func nonZeroIDs(ids []resource.ID) []resource.ID {
	out := make([]resource.ID, 0, len(ids))
	for _, id := range ids {
		if id != 0 {
			out = append(out, id)
		}
	}
	return out
}

func (s *server) toPurchaseDocuments(r *http.Request, admin resource.ID, list []purchases.Document) ([]purchaseDocumentJSON, error) {
	ids := make([]resource.ID, len(list))
	for i, d := range list {
		ids[i] = d.ContactID
	}
	byID, err := s.contactsOf(r, admin, ids)
	if err != nil {
		return nil, err
	}
	today, err := s.today(r, admin)
	if err != nil {
		return nil, err
	}
	day := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	out := make([]purchaseDocumentJSON, len(list))
	for i, d := range list {
		out[i] = s.toPurchaseDocument(d, byID[d.ContactID], day)
	}
	return out, nil
}

func (s *server) writePurchaseDocument(w http.ResponseWriter, r *http.Request, admin resource.ID, status int, d purchases.Document, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toPurchaseDocuments(r, admin, []purchases.Document{d})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, out[0])
}

// keyedList reads Moneybird's nested attributes: an array, or an object
// keyed by position.
type keyedList[T any] []T

// UnmarshalJSON implements json.Unmarshaler.
func (l *keyedList[T]) UnmarshalJSON(b []byte) error {
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte("[")) {
		var list []T
		if err := json.Unmarshal(b, &list); err != nil {
			return err
		}
		*l = list
		return nil
	}
	var keyed map[string]T
	if err := json.Unmarshal(b, &keyed); err != nil {
		return err
	}
	keys := make([]string, 0, len(keyed))
	for k := range keyed {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, c int) bool {
		x, errX := strconv.Atoi(keys[a])
		y, errY := strconv.Atoi(keys[c])
		if errX == nil && errY == nil {
			return x < y
		}
		return keys[a] < keys[c]
	})
	out := make([]T, 0, len(keys))
	for _, k := range keys {
		out = append(out, keyed[k])
	}
	*l = out
	return nil
}

type fiscalAllocationBody struct {
	ID                     *flexString `json:"id"`
	LedgerAccountID        *flexString `json:"ledger_account_id"`
	PrivateLedgerAccountID *flexString `json:"private_ledger_account_id"`
	Percentage             *flexString `json:"percentage"`
	Destroy                *flexBool   `json:"_destroy"`
}

type purchaseDocumentFields struct {
	ContactID         *flexString                      `json:"contact_id"`
	Reference         *string                          `json:"reference"`
	Date              *string                          `json:"date"`
	DueDate           *string                          `json:"due_date"`
	Currency          *string                          `json:"currency"`
	PricesAreInclTax  *flexBool                        `json:"prices_are_incl_tax"`
	RevenueInvoice    *flexBool                        `json:"revenue_invoice"`
	Details           *salesDetails                    `json:"details_attributes"`
	FiscalAllocations *keyedList[fiscalAllocationBody] `json:"fiscal_allocations_attributes"`
}

// toPurchaseChanges reads the fields of a purchase invoice or receipt.
func toPurchaseChanges(w http.ResponseWriter, in purchaseDocumentFields) (purchases.Changes, bool) {
	c := purchases.Changes{Reference: in.Reference, Date: in.Date, DueDate: in.DueDate, Currency: in.Currency, PricesAreInclTax: in.PricesAreInclTax.ptr(),
		RevenueInvoice: in.RevenueInvoice.ptr()}
	var ok bool
	if c.ContactID, ok = flexOptionalID(w, in.ContactID); !ok {
		return c, false
	}
	if c.Details, ok = toDetailInputs(w, in.Details); !ok {
		return c, false
	}
	// Every line of a document counts.
	for i := range c.Details {
		c.Details[i].Optional, c.Details[i].Selected = nil, nil
	}
	if in.FiscalAllocations != nil {
		c.FiscalAllocations = []purchases.AllocationInput{}
		for _, f := range *in.FiscalAllocations {
			a := purchases.AllocationInput{Percentage: f.Percentage.ptr(), Destroy: deref(f.Destroy.ptr())}
			if f.ID != nil && *f.ID != "" {
				if a.ID, ok = flexID(w, f.ID); !ok {
					return c, false
				}
			}
			for _, x := range []struct {
				src *flexString
				dst **resource.ID
			}{{f.LedgerAccountID, &a.LedgerAccountID}, {f.PrivateLedgerAccountID, &a.PrivateLedgerAccountID}} {
				if *x.dst, ok = flexOptionalID(w, x.src); !ok {
					return c, false
				}
			}
			c.FiscalAllocations = append(c.FiscalAllocations, a)
		}
	}
	return c, true
}

func readPurchaseChanges(w http.ResponseWriter, r *http.Request, kind string) (purchases.Changes, bool) {
	if kind == purchases.PurchaseInvoice {
		body, ok := decodeBody[struct {
			Invoice purchaseDocumentFields `json:"purchase_invoice"`
		}](w, r)
		if !ok {
			return purchases.Changes{}, false
		}
		return toPurchaseChanges(w, body.Invoice)
	}
	body, ok := decodeBody[struct {
		Receipt purchaseDocumentFields `json:"receipt"`
		Payment *struct {
			FinancialAccountID *flexString `json:"financial_account_id"`
		} `json:"payment"`
	}](w, r)
	if !ok {
		return purchases.Changes{}, false
	}
	c, ok := toPurchaseChanges(w, body.Receipt)
	if ok && body.Payment != nil {
		c.PaymentAccountID, ok = flexOptionalID(w, body.Payment.FinancialAccountID)
	}
	return c, ok
}

// purchaseQuery reads a document filter, preferring new_filter.
func (s *server) purchaseQuery(w http.ResponseWriter, r *http.Request, admin resource.ID, filter *DocumentFilter, newFilter *string) (purchases.Query, bool) {
	if newFilter != nil {
		filter = (*DocumentFilter)(newFilter)
	}
	now, err := s.today(r, admin)
	if err != nil {
		s.fail(w, r, err)
		return purchases.Query{}, false
	}
	q, err := purchases.ParseFilter(deref((*string)(filter)), now)
	if err != nil {
		writeError(w, http.StatusBadRequest, filterProblem(err))
		return q, false
	}
	return q, true
}

func (s *server) listPurchaseDocuments(w http.ResponseWriter, r *http.Request, kind, administrationID string, filter *DocumentFilter, newFilter *string, page *Page, perPage *PerPage, ascending bool) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := s.purchaseQuery(w, r, admin, filter, newFilter)
	if !ok {
		return
	}
	q.Page, q.PerPage = pageNumbers(page, perPage)
	q.Ascending = ascending
	list, total, err := s.deps.Purchases.List(r.Context(), admin, user(r).ID, kind, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toPurchaseDocuments(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeLinks(w, r, max(q.Page, 1), cmpPerPage(q.PerPage, purchases.DefaultPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

func (s *server) createPurchaseDocument(w http.ResponseWriter, r *http.Request, kind, administrationID string) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	c, ok := readPurchaseChanges(w, r, kind)
	if !ok {
		return
	}
	d, err := s.deps.Purchases.Create(r.Context(), admin, user(r).ID, kind, c)
	s.writePurchaseDocument(w, r, admin, http.StatusCreated, d, err)
}

func (s *server) purchaseDocumentVersions(w http.ResponseWriter, r *http.Request, kind, administrationID string, filter *DocumentFilter) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := s.purchaseQuery(w, r, admin, filter, nil)
	if !ok {
		return
	}
	list, err := s.deps.Purchases.Versions(r.Context(), admin, user(r).ID, kind, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]SynchronizationItem, len(list))
	for i, d := range list {
		out[i] = SynchronizationItem{Id: d.ID.String(), Version: int(d.Version())}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) fetchPurchaseDocuments(w http.ResponseWriter, r *http.Request, kind, administrationID string) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	ids, ok := readSyncIDs(w, r)
	if !ok {
		return
	}
	list, err := s.deps.Purchases.DocumentsByID(r.Context(), admin, user(r).ID, kind, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toPurchaseDocuments(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getPurchaseDocument(w http.ResponseWriter, r *http.Request, kind, administrationID, id string) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	d, err := s.deps.Purchases.Document(r.Context(), admin, kind, rid, user(r).ID)
	s.writePurchaseDocument(w, r, admin, http.StatusOK, d, err)
}

func (s *server) updatePurchaseDocument(w http.ResponseWriter, r *http.Request, kind, administrationID, id string) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	c, ok := readPurchaseChanges(w, r, kind)
	if !ok {
		return
	}
	d, err := s.deps.Purchases.Update(r.Context(), admin, user(r).ID, kind, rid, c)
	s.writePurchaseDocument(w, r, admin, http.StatusOK, d, err)
}

func (s *server) deletePurchaseDocument(w http.ResponseWriter, r *http.Request, kind, administrationID, id string) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	// Deleting unlinks the bank transactions that pay the document.
	d, err := s.deps.Purchases.Document(r.Context(), admin, kind, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, p := range d.Payments {
		if p.FinancialMutationID != nil {
			if !s.requireAlso(w, r, admin, administration.Bank) {
				return
			}
			break
		}
	}
	if err := s.deps.Purchases.Delete(r.Context(), admin, user(r).ID, kind, rid); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, http.StatusOK)
}

func (s *server) createPurchaseDocumentNote(w http.ResponseWriter, r *http.Request, kind, administrationID, id string) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	in, ok := decodeNote(w, r)
	if !ok {
		return
	}
	n, err := s.deps.Purchases.AddNote(r.Context(), admin, user(r).ID, kind, rid, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toNote(n))
}

func (s *server) deletePurchaseDocumentNote(w http.ResponseWriter, r *http.Request, kind, administrationID, id, noteID string) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	note, ok := parseID(w, noteID)
	if !ok {
		return
	}
	if err := s.deps.Purchases.DeleteNote(r.Context(), admin, user(r).ID, kind, rid, note); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) createPurchaseDocumentAttachment(w http.ResponseWriter, r *http.Request, kind, administrationID, id string) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	name, contentType, data, ok := readUpload(w, r)
	if !ok {
		return
	}
	a, err := s.deps.Purchases.AddAttachment(r.Context(), admin, user(r).ID, kind, rid, name, contentType, data)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAttachment(a))
}

func (s *server) deletePurchaseDocumentAttachment(w http.ResponseWriter, r *http.Request, kind, administrationID, id, attachmentID string) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	att, ok := parseID(w, attachmentID)
	if !ok {
		return
	}
	if err := s.deps.Purchases.DeleteAttachment(r.Context(), admin, user(r).ID, kind, rid, att); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// paymentsOf adapts the payment calls of a kind of document to the shared
// payment handlers.
func (s *server) paymentsOf(kind string) (
	func(ctx context.Context, admin, id, user resource.ID, in sales.PaymentInput) (sales.Payment, error),
	func(ctx context.Context, admin, id, payment, user resource.ID) error,
) {
	register := func(ctx context.Context, admin, id, user resource.ID, in sales.PaymentInput) (sales.Payment, error) {
		return s.deps.Purchases.RegisterPayment(ctx, admin, user, kind, id, in)
	}
	remove := func(ctx context.Context, admin, id, payment, user resource.ID) error {
		return s.deps.Purchases.DeletePayment(ctx, admin, user, kind, id, payment)
	}
	return register, remove
}

func (s *server) deletePurchaseDocumentPayment(w http.ResponseWriter, r *http.Request, kind, administrationID, id, paymentID string) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	pid, ok := parseID(w, paymentID)
	if !ok {
		return
	}
	// A payment made with a bank transaction is unlinked from it.
	if d, err := s.deps.Purchases.Document(r.Context(), admin, kind, rid, user(r).ID); err == nil {
		for _, p := range d.Payments {
			if p.ID == pid && p.FinancialMutationID != nil && !s.requireAlso(w, r, admin, administration.Bank) {
				return
			}
		}
	}
	_, remove := s.paymentsOf(kind)
	s.deletePayment(w, r, administrationID, id, paymentID, remove)
}

// ListPurchaseInvoices implements ServerInterface.
func (s *server) ListPurchaseInvoices(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListPurchaseInvoicesParams) {
	s.listPurchaseDocuments(w, r, purchases.PurchaseInvoice, administrationID, params.Filter, params.NewFilter, params.Page, params.PerPage,
		params.Sort != nil && *params.Sort == "date_asc")
}

// CreatePurchaseInvoice implements ServerInterface.
func (s *server) CreatePurchaseInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	s.createPurchaseDocument(w, r, purchases.PurchaseInvoice, administrationID)
}

// ListPurchaseInvoiceVersions implements ServerInterface.
func (s *server) ListPurchaseInvoiceVersions(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListPurchaseInvoiceVersionsParams) {
	s.purchaseDocumentVersions(w, r, purchases.PurchaseInvoice, administrationID, params.Filter)
}

// FetchPurchaseInvoices implements ServerInterface.
func (s *server) FetchPurchaseInvoices(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	s.fetchPurchaseDocuments(w, r, purchases.PurchaseInvoice, administrationID)
}

// GetPurchaseInvoice implements ServerInterface.
func (s *server) GetPurchaseInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.getPurchaseDocument(w, r, purchases.PurchaseInvoice, administrationID, id)
}

// UpdatePurchaseInvoice implements ServerInterface.
func (s *server) UpdatePurchaseInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.updatePurchaseDocument(w, r, purchases.PurchaseInvoice, administrationID, id)
}

// DeletePurchaseInvoice implements ServerInterface.
func (s *server) DeletePurchaseInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.deletePurchaseDocument(w, r, purchases.PurchaseInvoice, administrationID, id)
}

// CreatePurchaseInvoiceNote implements ServerInterface.
func (s *server) CreatePurchaseInvoiceNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.createPurchaseDocumentNote(w, r, purchases.PurchaseInvoice, administrationID, id)
}

// DeletePurchaseInvoiceNote implements ServerInterface.
func (s *server) DeletePurchaseInvoiceNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, noteID Identifier) {
	s.deletePurchaseDocumentNote(w, r, purchases.PurchaseInvoice, administrationID, id, noteID)
}

// CreatePurchaseInvoiceAttachment implements ServerInterface.
func (s *server) CreatePurchaseInvoiceAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.createPurchaseDocumentAttachment(w, r, purchases.PurchaseInvoice, administrationID, id)
}

// DeletePurchaseInvoiceAttachment implements ServerInterface.
func (s *server) DeletePurchaseInvoiceAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	s.deletePurchaseDocumentAttachment(w, r, purchases.PurchaseInvoice, administrationID, id, attachmentID)
}

// DownloadPurchaseInvoiceAttachment implements ServerInterface.
func (s *server) DownloadPurchaseInvoiceAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	s.redirectToAttachment(w, r, filePurchaseInvoiceAttachment, administrationID, id, attachmentID)
}

// CreatePurchaseInvoicePayment implements ServerInterface.
func (s *server) CreatePurchaseInvoicePayment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	register, _ := s.paymentsOf(purchases.PurchaseInvoice)
	s.registerPayment(w, r, administrationID, id, http.StatusCreated, register)
}

// RegisterPurchaseInvoicePayment implements ServerInterface.
func (s *server) RegisterPurchaseInvoicePayment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	register, _ := s.paymentsOf(purchases.PurchaseInvoice)
	s.registerPayment(w, r, administrationID, id, http.StatusOK, register)
}

// DeletePurchaseInvoicePayment implements ServerInterface.
func (s *server) DeletePurchaseInvoicePayment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, paymentID Identifier) {
	s.deletePurchaseDocumentPayment(w, r, purchases.PurchaseInvoice, administrationID, id, paymentID)
}

// ListReceipts implements ServerInterface.
func (s *server) ListReceipts(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListReceiptsParams) {
	s.listPurchaseDocuments(w, r, purchases.Receipt, administrationID, params.Filter, params.NewFilter, params.Page, params.PerPage,
		params.Sort != nil && *params.Sort == "date_asc")
}

// CreateReceipt implements ServerInterface.
func (s *server) CreateReceipt(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	s.createPurchaseDocument(w, r, purchases.Receipt, administrationID)
}

// ListReceiptVersions implements ServerInterface.
func (s *server) ListReceiptVersions(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListReceiptVersionsParams) {
	s.purchaseDocumentVersions(w, r, purchases.Receipt, administrationID, params.Filter)
}

// FetchReceipts implements ServerInterface.
func (s *server) FetchReceipts(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	s.fetchPurchaseDocuments(w, r, purchases.Receipt, administrationID)
}

// GetReceipt implements ServerInterface.
func (s *server) GetReceipt(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.getPurchaseDocument(w, r, purchases.Receipt, administrationID, id)
}

// UpdateReceipt implements ServerInterface.
func (s *server) UpdateReceipt(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.updatePurchaseDocument(w, r, purchases.Receipt, administrationID, id)
}

// DeleteReceipt implements ServerInterface.
func (s *server) DeleteReceipt(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.deletePurchaseDocument(w, r, purchases.Receipt, administrationID, id)
}

// CreateReceiptNote implements ServerInterface.
func (s *server) CreateReceiptNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.createPurchaseDocumentNote(w, r, purchases.Receipt, administrationID, id)
}

// DeleteReceiptNote implements ServerInterface.
func (s *server) DeleteReceiptNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, noteID Identifier) {
	s.deletePurchaseDocumentNote(w, r, purchases.Receipt, administrationID, id, noteID)
}

// CreateReceiptAttachment implements ServerInterface.
func (s *server) CreateReceiptAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.createPurchaseDocumentAttachment(w, r, purchases.Receipt, administrationID, id)
}

// DeleteReceiptAttachment implements ServerInterface.
func (s *server) DeleteReceiptAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	s.deletePurchaseDocumentAttachment(w, r, purchases.Receipt, administrationID, id, attachmentID)
}

// DownloadReceiptAttachment implements ServerInterface.
func (s *server) DownloadReceiptAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	s.redirectToAttachment(w, r, fileReceiptAttachment, administrationID, id, attachmentID)
}

// CreateReceiptPayment implements ServerInterface.
func (s *server) CreateReceiptPayment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	register, _ := s.paymentsOf(purchases.Receipt)
	s.registerPayment(w, r, administrationID, id, http.StatusCreated, register)
}

// RegisterReceiptPayment implements ServerInterface.
func (s *server) RegisterReceiptPayment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	register, _ := s.paymentsOf(purchases.Receipt)
	s.registerPayment(w, r, administrationID, id, http.StatusOK, register)
}

// DeleteReceiptPayment implements ServerInterface.
func (s *server) DeleteReceiptPayment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, paymentID Identifier) {
	s.deletePurchaseDocumentPayment(w, r, purchases.Receipt, administrationID, id, paymentID)
}
