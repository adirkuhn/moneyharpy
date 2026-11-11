package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

type salesDetailJSON struct {
	ID                          string    `json:"id"`
	AdministrationID            string    `json:"administration_id"`
	TaxRateID                   string    `json:"tax_rate_id"`
	LedgerAccountID             string    `json:"ledger_account_id"`
	ProjectID                   *string   `json:"project_id"`
	ProductID                   *string   `json:"product_id"`
	Amount                      string    `json:"amount"`
	AmountDecimal               string    `json:"amount_decimal"`
	Description                 string    `json:"description"`
	Price                       string    `json:"price"`
	Period                      *string   `json:"period"`
	RowOrder                    int       `json:"row_order"`
	TotalPriceExclTaxWithDisc   string    `json:"total_price_excl_tax_with_discount"`
	TotalPriceExclTaxWithDiscBs string    `json:"total_price_excl_tax_with_discount_base"`
	TaxReportReference          []string  `json:"tax_report_reference"`
	MandatoryTaxText            *string   `json:"mandatory_tax_text"`
	IsOptional                  bool      `json:"is_optional"`
	IsSelected                  bool      `json:"is_selected"`
	CreatedAt                   time.Time `json:"created_at"`
	UpdatedAt                   time.Time `json:"updated_at"`
}

type taxTotalJSON struct {
	TaxRateID         string `json:"tax_rate_id"`
	TaxableAmount     string `json:"taxable_amount"`
	TaxableAmountBase string `json:"taxable_amount_base"`
	TaxAmount         string `json:"tax_amount"`
	TaxAmountBase     string `json:"tax_amount_base"`
}

type salesInvoiceJSON struct {
	ID                      string                 `json:"id"`
	AdministrationID        string                 `json:"administration_id"`
	ContactID               string                 `json:"contact_id"`
	Contact                 *contactJSON           `json:"contact,omitempty"`
	ContactPersonID         *string                `json:"contact_person_id"`
	ContactPerson           *contactPersonJSON     `json:"contact_person"`
	InvoiceID               *string                `json:"invoice_id"`
	RecurringSalesInvoiceID *string                `json:"recurring_sales_invoice_id"`
	SubscriptionID          *string                `json:"subscription_id"`
	WorkflowID              string                 `json:"workflow_id"`
	DocumentStyleID         string                 `json:"document_style_id"`
	IdentityID              string                 `json:"identity_id"`
	DraftID                 *int                   `json:"draft_id"`
	State                   string                 `json:"state"`
	InvoiceDate             *string                `json:"invoice_date"`
	DueDate                 string                 `json:"due_date"`
	PaymentConditions       *string                `json:"payment_conditions"`
	PaymentReference        *string                `json:"payment_reference"`
	ShortPaymentReference   *string                `json:"short_payment_reference"`
	Reference               *string                `json:"reference"`
	Language                string                 `json:"language"`
	Currency                string                 `json:"currency"`
	Discount                string                 `json:"discount"`
	OriginalSalesInvoiceID  *string                `json:"original_sales_invoice_id"`
	OriginalEstimateID      *string                `json:"original_estimate_id"`
	Paused                  bool                   `json:"paused"`
	PaidAt                  *string                `json:"paid_at"`
	SentAt                  *string                `json:"sent_at"`
	CreatedAt               time.Time              `json:"created_at"`
	UpdatedAt               time.Time              `json:"updated_at"`
	PublicViewCode          *string                `json:"public_view_code"`
	PublicViewCodeExpiresAt *time.Time             `json:"public_view_code_expires_at"`
	Version                 int64                  `json:"version"`
	Details                 []salesDetailJSON      `json:"details"`
	Payments                []paymentJSON          `json:"payments"`
	TotalPaid               string                 `json:"total_paid"`
	TotalUnpaid             string                 `json:"total_unpaid"`
	TotalUnpaidBase         string                 `json:"total_unpaid_base"`
	PricesAreInclTax        bool                   `json:"prices_are_incl_tax"`
	TotalPriceExclTax       string                 `json:"total_price_excl_tax"`
	TotalPriceExclTaxBase   string                 `json:"total_price_excl_tax_base"`
	TotalPriceInclTax       string                 `json:"total_price_incl_tax"`
	TotalPriceInclTaxBase   string                 `json:"total_price_incl_tax_base"`
	TotalDiscount           string                 `json:"total_discount"`
	TotalTax                string                 `json:"total_tax"`
	MarkedDubiousOn         *string                `json:"marked_dubious_on"`
	MarkedUncollectibleOn   *string                `json:"marked_uncollectible_on"`
	ReminderCount           int                    `json:"reminder_count"`
	NextReminder            *string                `json:"next_reminder"`
	URL                     string                 `json:"url"`
	PaymentURL              string                 `json:"payment_url"`
	CustomFields            []customFieldValueJSON `json:"custom_fields"`
	Notes                   []noteJSON             `json:"notes"`
	Attachments             []attachmentJSON       `json:"attachments"`
	Events                  []any                  `json:"events"`
	TaxTotals               []taxTotalJSON         `json:"tax_totals"`
	TimeEntries             []any                  `json:"time_entries"`
	FirstDueInterval        int                    `json:"first_due_interval"`
	DeliveryMethod          *string                `json:"delivery_method"`
	URI                     string                 `json:"uri"`
}

// percentText writes a percentage kept in hundredths, such as "2.5".
func percentText(p int64) string {
	whole, frac := p/100, p%100
	if frac == 0 {
		return strconv.FormatInt(whole, 10) + ".0"
	}
	return strings.TrimRight(strconv.FormatInt(whole, 10)+"."+strconv.FormatInt(frac+100, 10)[1:], "0")
}

// quantityText writes a quantity kept in thousandths, such as "2.5".
func quantityText(q int64) string {
	sign := ""
	if q < 0 {
		sign, q = "-", -q
	}
	s := strconv.FormatInt(q/1000, 10)
	if frac := q % 1000; frac != 0 {
		s += "." + strings.TrimRight(strconv.FormatInt(frac+1000, 10)[1:], "0")
	} else {
		s += ".0"
	}
	return sign + s
}

func (s *server) toSalesInvoice(inv sales.Invoice, contact *contacts.Contact, today time.Time) salesInvoiceJSON {
	date := func(t *time.Time) *string {
		if t == nil {
			return nil
		}
		v := t.Format(time.DateOnly)
		return &v
	}
	due := inv.DueDate
	if due == nil {
		from := today
		if inv.InvoiceDate != nil {
			from = *inv.InvoiceDate
		}
		d := from.AddDate(0, 0, inv.FirstDueInterval)
		due = &d
	}
	paid := inv.Paid()
	unpaid := inv.Totals.InclTax - paid
	out := salesInvoiceJSON{
		ID: inv.ID.String(), AdministrationID: inv.AdministrationID.String(), ContactID: inv.ContactID.String(), ContactPersonID: idString(inv.ContactPersonID),
		InvoiceID: nz(inv.InvoiceID), RecurringSalesInvoiceID: idString(inv.RecurringSalesInvoiceID), WorkflowID: inv.WorkflowID.String(),
		DocumentStyleID: inv.DocumentStyleID.String(), IdentityID: inv.IdentityID.String(), State: inv.State, InvoiceDate: date(inv.InvoiceDate),
		DueDate: due.Format(time.DateOnly), PaymentConditions: nz(inv.PaymentConditions), PaymentReference: nz(inv.PaymentReference),
		Reference: nz(inv.Reference), Language: inv.Language, Currency: inv.Currency, Discount: percentText(inv.Discount),
		OriginalSalesInvoiceID: idString(inv.OriginalID), Paused: inv.Paused, PaidAt: date(inv.PaidAt), SentAt: date(inv.SentAt),
		CreatedAt: inv.CreatedAt, UpdatedAt: inv.UpdatedAt, Version: inv.Version(), Details: []salesDetailJSON{}, Payments: []paymentJSON{},
		TotalPaid: paid.String(), TotalUnpaid: unpaid.String(), TotalUnpaidBase: unpaid.String(), PricesAreInclTax: inv.PricesAreInclTax,
		TotalPriceExclTax: inv.Totals.ExclTax.String(), TotalPriceExclTaxBase: inv.Totals.ExclTax.String(), TotalPriceInclTax: inv.Totals.InclTax.String(),
		TotalPriceInclTaxBase: inv.Totals.InclTax.String(), TotalDiscount: inv.Totals.Discount.String(), TotalTax: inv.Totals.Tax.String(),
		MarkedDubiousOn: date(inv.MarkedDubiousOn), MarkedUncollectibleOn: date(inv.MarkedUncollectibleOn), ReminderCount: inv.ReminderCount,
		NextReminder: date(inv.NextReminder),
		URL:          strings.TrimSuffix(s.deps.BaseURL, "/") + inv.Ref().URI(), CustomFields: []customFieldValueJSON{}, Notes: []noteJSON{},
		Attachments: []attachmentJSON{}, Events: []any{}, TaxTotals: []taxTotalJSON{}, TimeEntries: []any{}, FirstDueInterval: inv.FirstDueInterval,
		URI: inv.Ref().URI(),
	}
	if inv.DraftID > 0 {
		out.DraftID = &inv.DraftID
	}
	if inv.Delivery != nil {
		out.DeliveryMethod = &inv.Delivery.Method
	}
	if contact != nil {
		c := s.toContact(*contact)
		out.Contact = &c
		if inv.ContactPersonID != nil {
			for _, p := range contact.People {
				if p.ID == *inv.ContactPersonID {
					cp := toContactPerson(p)
					out.ContactPerson = &cp
				}
			}
		}
	}
	for i, d := range inv.Lines {
		var base string
		if i < len(inv.Totals.LineBases) {
			base = inv.Totals.LineBases[i].String()
		} else {
			base = "0.0"
		}
		out.Details = append(out.Details, salesDetailJSON{
			ID: d.ID.String(), AdministrationID: inv.AdministrationID.String(), TaxRateID: d.TaxRateID.String(), LedgerAccountID: deref(idString(d.LedgerAccountID)),
			ProjectID: idString(d.ProjectID), ProductID: idString(d.ProductID), Amount: d.Amount, AmountDecimal: quantityText(d.Quantity),
			Description: d.Description, Price: d.Price.String(), Period: nz(d.Period), RowOrder: d.RowOrder, TotalPriceExclTaxWithDisc: base,
			TotalPriceExclTaxWithDiscBs: base, TaxReportReference: []string{}, IsOptional: d.Optional, IsSelected: d.Selected,
			CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
		})
	}
	for _, p := range inv.Payments {
		out.Payments = append(out.Payments, toPayment(p))
	}
	for _, r := range inv.Totals.ByRate {
		out.TaxTotals = append(out.TaxTotals, taxTotalJSON{TaxRateID: r.TaxRateID.String(), TaxableAmount: r.Taxable.String(), TaxableAmountBase: r.Taxable.String(), TaxAmount: r.Tax.String(), TaxAmountBase: r.Tax.String()})
	}
	for _, f := range inv.CustomFields {
		out.CustomFields = append(out.CustomFields, customFieldValueJSON{ID: f.FieldID.String(), Name: f.Name, Value: f.Value})
	}
	for _, n := range inv.Notes {
		out.Notes = append(out.Notes, toNote(n))
	}
	for _, a := range inv.Attachments {
		out.Attachments = append(out.Attachments, toAttachment(a))
	}
	return out
}

// toSalesInvoices answers with invoices and their contacts, loaded at once.
func (s *server) toSalesInvoices(r *http.Request, admin resource.ID, list []sales.Invoice) ([]salesInvoiceJSON, error) {
	ids := make([]resource.ID, 0, len(list))
	for _, inv := range list {
		ids = append(ids, inv.ContactID)
	}
	byID := map[resource.ID]*contacts.Contact{}
	for start := 0; start < len(ids); start += contacts.MaxFetch {
		found, err := s.deps.Contacts.ContactsByID(r.Context(), admin, user(r).ID, ids[start:min(start+contacts.MaxFetch, len(ids))])
		if err != nil {
			return nil, err
		}
		for i := range found {
			byID[found[i].ID] = &found[i]
		}
	}
	today, err := s.today(r, admin)
	if err != nil {
		return nil, err
	}
	out := make([]salesInvoiceJSON, len(list))
	for i, inv := range list {
		out[i] = s.toSalesInvoice(inv, byID[inv.ContactID], today)
	}
	return out, nil
}

func (s *server) writeSalesInvoice(w http.ResponseWriter, r *http.Request, admin resource.ID, status int, inv sales.Invoice) {
	out, err := s.toSalesInvoices(r, admin, []sales.Invoice{inv})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, out[0])
}

type salesDetailBody struct {
	ID              *flexString `json:"id"`
	Description     *string     `json:"description"`
	Period          *string     `json:"period"`
	Price           *flexString `json:"price"`
	Amount          *flexString `json:"amount"`
	TaxRateID       *flexString `json:"tax_rate_id"`
	LedgerAccountID *flexString `json:"ledger_account_id"`
	ProjectID       *flexString `json:"project_id"`
	ProductID       *flexString `json:"product_id"`
	RowOrder        *flexString `json:"row_order"`
	Optional        *flexBool   `json:"is_optional"`
	Selected        *flexBool   `json:"is_selected"`
	Destroy         *flexBool   `json:"_destroy"`
}

type salesDetails []salesDetailBody

// UnmarshalJSON implements json.Unmarshaler: lines come as an array or as
// an object keyed by position.
func (d *salesDetails) UnmarshalJSON(b []byte) error {
	type plain salesDetails
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte("[")) {
		return json.Unmarshal(b, (*plain)(d))
	}
	var keyed map[string]salesDetailBody
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
	for _, k := range keys {
		*d = append(*d, keyed[k])
	}
	return nil
}

type salesInvoiceBody struct {
	Invoice struct {
		ContactID         *flexString   `json:"contact_id"`
		ContactPersonID   *flexString   `json:"contact_person_id"`
		DocumentStyleID   *flexString   `json:"document_style_id"`
		WorkflowID        *flexString   `json:"workflow_id"`
		IdentityID        *flexString   `json:"identity_id"`
		Reference         *string       `json:"reference"`
		SequenceID        *string       `json:"invoice_sequence_id"`
		RemoveSequence    *flexBool     `json:"remove_invoice_sequence_id"`
		InvoiceDate       *string       `json:"invoice_date"`
		FirstDueInterval  *flexString   `json:"first_due_interval"`
		Currency          *string       `json:"currency"`
		PricesAreInclTax  *flexBool     `json:"prices_are_incl_tax"`
		PaymentConditions *string       `json:"payment_conditions"`
		Discount          *flexString   `json:"discount"`
		Details           *salesDetails `json:"details_attributes"`
		CustomFields      *fieldValues  `json:"custom_fields_attributes"`
	} `json:"sales_invoice"`
}

func flexInt(w http.ResponseWriter, f *flexString, field string) (*int, bool) {
	if f == nil {
		return nil, true
	}
	n, err := strconv.Atoi(string(*f))
	if err != nil {
		writeError(w, http.StatusBadRequest, field+" is not a number")
		return nil, false
	}
	return &n, true
}

func toSalesChanges(w http.ResponseWriter, body salesInvoiceBody) (sales.Changes, bool) {
	in := body.Invoice
	c := sales.Changes{Reference: in.Reference, InvoiceDate: in.InvoiceDate, Currency: in.Currency, PricesAreInclTax: in.PricesAreInclTax.ptr(),
		PaymentConditions: in.PaymentConditions, Discount: in.Discount.ptr(), SequenceID: in.SequenceID}
	if deref(in.RemoveSequence.ptr()) {
		c.SequenceID = ptr("")
	}
	var ok bool
	for _, f := range []struct {
		src *flexString
		dst **resource.ID
	}{{in.ContactID, &c.ContactID}, {in.ContactPersonID, &c.ContactPersonID}, {in.DocumentStyleID, &c.DocumentStyleID}, {in.WorkflowID, &c.WorkflowID}, {in.IdentityID, &c.IdentityID}} {
		if *f.dst, ok = flexOptionalID(w, f.src); !ok {
			return c, false
		}
	}
	if c.FirstDueInterval, ok = flexInt(w, in.FirstDueInterval, "first_due_interval"); !ok {
		return c, false
	}
	if c.Details, ok = toDetailInputs(w, in.Details); !ok {
		return c, false
	}
	if in.CustomFields != nil {
		c.CustomFields = []sales.FieldValue{}
		for _, v := range *in.CustomFields {
			id, ok := flexID(w, &v.ID)
			if !ok {
				return c, false
			}
			c.CustomFields = append(c.CustomFields, sales.FieldValue{FieldID: *id, Value: v.Value})
		}
	}
	return c, true
}

// toDetailInputs reads details_attributes; nil when they are left out.
func toDetailInputs(w http.ResponseWriter, details *salesDetails) ([]sales.DetailInput, bool) {
	if details == nil {
		return nil, true
	}
	out := []sales.DetailInput{}
	var ok bool
	for _, d := range *details {
		line := sales.DetailInput{Description: d.Description, Period: d.Period, Price: d.Price.ptr(), Amount: d.Amount.ptr(),
			Optional: d.Optional.ptr(), Selected: d.Selected.ptr(), Destroy: deref(d.Destroy.ptr())}
		if d.ID != nil && *d.ID != "" {
			if line.ID, ok = flexID(w, d.ID); !ok {
				return nil, false
			}
		}
		for _, f := range []struct {
			src *flexString
			dst **resource.ID
		}{{d.TaxRateID, &line.TaxRateID}, {d.LedgerAccountID, &line.LedgerAccountID}, {d.ProjectID, &line.ProjectID}, {d.ProductID, &line.ProductID}} {
			if *f.dst, ok = flexOptionalID(w, f.src); !ok {
				return nil, false
			}
		}
		if line.RowOrder, ok = flexInt(w, d.RowOrder, "row_order"); !ok {
			return nil, false
		}
		out = append(out, line)
	}
	return out, true
}

func (s *server) salesQuery(w http.ResponseWriter, r *http.Request, admin resource.ID, filter *DocumentFilter) (sales.Query, bool) {
	now, err := s.today(r, admin)
	if err != nil {
		s.fail(w, r, err)
		return sales.Query{}, false
	}
	q, err := sales.ParseFilter(deref((*string)(filter)), now)
	if err != nil {
		writeError(w, http.StatusBadRequest, filterProblem(err))
		return q, false
	}
	return q, true
}

// ListSalesInvoices implements ServerInterface.
func (s *server) ListSalesInvoices(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListSalesInvoicesParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := s.salesQuery(w, r, admin, params.Filter)
	if !ok {
		return
	}
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	q.Ascending = params.Sort != nil && *params.Sort == "date_asc"
	list, total, err := s.deps.Sales.List(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toSalesInvoices(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeLinks(w, r, max(q.Page, 1), cmpPerPage(q.PerPage, sales.DefaultPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

// CreateSalesInvoice implements ServerInterface.
func (s *server) CreateSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[salesInvoiceBody](w, r)
	if !ok {
		return
	}
	c, ok := toSalesChanges(w, body)
	if !ok {
		return
	}
	inv, err := s.deps.Sales.Create(r.Context(), admin, user(r).ID, c)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeSalesInvoice(w, r, admin, http.StatusCreated, inv)
}

// ListSalesInvoiceVersions implements ServerInterface.
func (s *server) ListSalesInvoiceVersions(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListSalesInvoiceVersionsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := s.salesQuery(w, r, admin, params.Filter)
	if !ok {
		return
	}
	list, err := s.deps.Sales.Versions(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]SynchronizationItem, len(list))
	for i, inv := range list {
		out[i] = SynchronizationItem{Id: inv.ID.String(), Version: int(inv.Version())}
	}
	writeJSON(w, http.StatusOK, out)
}

// FetchSalesInvoices implements ServerInterface.
func (s *server) FetchSalesInvoices(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	ids, ok := readSyncIDs(w, r)
	if !ok {
		return
	}
	list, err := s.deps.Sales.InvoicesByID(r.Context(), admin, user(r).ID, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toSalesInvoices(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// GetSalesInvoiceByInvoiceId implements ServerInterface.
func (s *server) GetSalesInvoiceByInvoiceId(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, number string) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	inv, err := s.deps.Sales.InvoiceByNumber(r.Context(), admin, user(r).ID, number)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeSalesInvoice(w, r, admin, http.StatusOK, inv)
}

// GetSalesInvoiceByReference implements ServerInterface.
func (s *server) GetSalesInvoiceByReference(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, reference string) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	inv, err := s.deps.Sales.InvoiceByReference(r.Context(), admin, user(r).ID, reference)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeSalesInvoice(w, r, admin, http.StatusOK, inv)
}

// GetSalesInvoice implements ServerInterface.
func (s *server) GetSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	inv, err := s.deps.Sales.Invoice(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeSalesInvoice(w, r, admin, http.StatusOK, inv)
}

// UpdateSalesInvoice implements ServerInterface.
func (s *server) UpdateSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[salesInvoiceBody](w, r)
	if !ok {
		return
	}
	c, ok := toSalesChanges(w, body)
	if !ok {
		return
	}
	inv, err := s.deps.Sales.Update(r.Context(), admin, rid, user(r).ID, c)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeSalesInvoice(w, r, admin, http.StatusOK, inv)
}

// DeleteSalesInvoice implements ServerInterface.
func (s *server) DeleteSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Sales.Delete(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SendSalesInvoice implements ServerInterface.
func (s *server) SendSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		Sending struct {
			DeliveryMethod string    `json:"delivery_method"`
			Scheduled      *flexBool `json:"sending_scheduled"`
			DeliverUBL     *flexBool `json:"deliver_ubl"`
			Mergeable      *flexBool `json:"mergeable"`
			EmailAddress   string    `json:"email_address"`
			EmailMessage   string    `json:"email_message"`
			InvoiceDate    string    `json:"invoice_date"`
		} `json:"sales_invoice_sending"`
	}](w, r)
	if !ok {
		return
	}
	in := body.Sending
	inv, err := s.deps.Sales.Send(r.Context(), admin, rid, user(r).ID, sales.SendInput{
		DeliveryMethod: in.DeliveryMethod, Scheduled: deref(in.Scheduled.ptr()), DeliverUBL: in.DeliverUBL.ptr(), Mergeable: in.Mergeable.ptr(),
		EmailAddress: in.EmailAddress, EmailMessage: in.EmailMessage, InvoiceDate: in.InvoiceDate,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeSalesInvoice(w, r, admin, http.StatusOK, inv)
}

func writeDocument(w http.ResponseWriter, contentType, name string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	// The client went away if this fails; there is nobody left to tell.
	_, _ = w.Write(data)
}

func (s *server) download(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, contentType string,
	render func(admin, id resource.ID) ([]byte, string, error)) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	data, name, err := render(admin, rid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeDocument(w, contentType, name, data)
}

// DownloadSalesInvoicePdf implements ServerInterface.
func (s *server) DownloadSalesInvoicePdf(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.download(w, r, administrationID, id, "application/pdf", func(admin, rid resource.ID) ([]byte, string, error) {
		return s.deps.Sales.PDF(r.Context(), admin, rid, user(r).ID)
	})
}

// DownloadSalesInvoicePackingSlip implements ServerInterface.
func (s *server) DownloadSalesInvoicePackingSlip(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.download(w, r, administrationID, id, "application/pdf", func(admin, rid resource.ID) ([]byte, string, error) {
		return s.deps.Sales.PackingSlip(r.Context(), admin, rid, user(r).ID)
	})
}

// DownloadSalesInvoiceUbl implements ServerInterface.
func (s *server) DownloadSalesInvoiceUbl(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.download(w, r, administrationID, id, "application/xml", func(admin, rid resource.ID) ([]byte, string, error) {
		return s.deps.Sales.UBL(r.Context(), admin, rid, user(r).ID)
	})
}

// DuplicateCreditSalesInvoice implements ServerInterface.
func (s *server) DuplicateCreditSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	var lines []resource.ID
	if r.ContentLength != 0 {
		body, ok := decodeBody[struct {
			DetailIDs []flexString `json:"detail_ids"`
		}](w, r)
		if !ok {
			return
		}
		for i := range body.DetailIDs {
			lineID, ok := flexID(w, &body.DetailIDs[i])
			if !ok {
				return
			}
			lines = append(lines, *lineID)
		}
	}
	inv, err := s.deps.Sales.Credit(r.Context(), admin, rid, user(r).ID, lines)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeSalesInvoice(w, r, admin, http.StatusOK, inv)
}

func (s *server) pause(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, paused bool) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	inv, err := s.deps.Sales.Pause(r.Context(), admin, rid, user(r).ID, paused)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeSalesInvoice(w, r, admin, http.StatusCreated, inv)
}

// PauseSalesInvoice implements ServerInterface.
func (s *server) PauseSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.pause(w, r, administrationID, id, true)
}

// ResumeSalesInvoice implements ServerInterface.
func (s *server) ResumeSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.pause(w, r, administrationID, id, false)
}

// CreateSalesInvoiceNote implements ServerInterface.
func (s *server) CreateSalesInvoiceNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	in, ok := decodeNote(w, r)
	if !ok {
		return
	}
	n, err := s.deps.Sales.AddNote(r.Context(), admin, rid, user(r).ID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toNote(n))
}

// DeleteSalesInvoiceNote implements ServerInterface.
func (s *server) DeleteSalesInvoiceNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, noteID Identifier) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	note, ok := parseID(w, noteID)
	if !ok {
		return
	}
	if err := s.deps.Sales.DeleteNote(r.Context(), admin, rid, note, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateSalesInvoiceAttachment implements ServerInterface.
func (s *server) CreateSalesInvoiceAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	name, contentType, data, ok := readUpload(w, r)
	if !ok {
		return
	}
	a, err := s.deps.Sales.AddAttachment(r.Context(), admin, rid, user(r).ID, name, contentType, data)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAttachment(a))
}

// DeleteSalesInvoiceAttachment implements ServerInterface.
func (s *server) DeleteSalesInvoiceAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	att, ok := parseID(w, attachmentID)
	if !ok {
		return
	}
	if err := s.deps.Sales.DeleteAttachment(r.Context(), admin, rid, att, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DownloadSalesInvoiceAttachment implements ServerInterface.
func (s *server) DownloadSalesInvoiceAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	att, ok := parseID(w, attachmentID)
	if !ok {
		return
	}
	a, body, err := s.deps.Sales.OpenAttachment(r.Context(), admin, rid, att, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeFile(w, a, body)
}
