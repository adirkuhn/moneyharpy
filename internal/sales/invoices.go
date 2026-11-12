package sales

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

var (
	periodPattern   = regexp.MustCompile(`^\d{8}\.\.\d{8}$`)
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
)

// DetailInput changes, adds or removes a line. A line without ID is new.
type DetailInput struct {
	ID              *resource.ID
	Description     *string
	Period          *string
	Price           *string
	Amount          *string
	TaxRateID       *resource.ID
	LedgerAccountID *resource.ID
	ProjectID       *resource.ID
	ProductID       *resource.ID
	RowOrder        *int
	Optional        *bool
	Selected        *bool
	Destroy         bool
}

// Changes holds the fields to set; nil fields stay as they are. A zero id
// clears an optional one.
type Changes struct {
	ContactID         *resource.ID
	ContactPersonID   *resource.ID
	DocumentStyleID   *resource.ID
	WorkflowID        *resource.ID
	IdentityID        *resource.ID
	Reference         *string
	InvoiceDate       *string
	FirstDueInterval  *int
	Currency          *string
	PricesAreInclTax  *bool
	PaymentConditions *string
	Discount          *string
	SequenceID        *string
	Details           []DetailInput
	CustomFields      []FieldValue
}

// NextDraftID returns the draft number the next new invoice gets, which
// Moneybird shows on an invoice being added ("Concept #12").
func (s *Service) NextDraftID(ctx context.Context, admin, user resource.ID) (int, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return 0, err
	}
	n, err := s.store.NextDraftID(ctx, admin)
	if err != nil {
		return 0, wrap("number draft", err)
	}
	return n, nil
}

// Create adds a draft invoice, filled from the administration's default
// workflow, document style and identity where the changes leave them out.
func (s *Service) Create(ctx context.Context, admin, user resource.ID, c Changes) (Invoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Invoice{}, err
	}
	now := s.now()
	inv := Invoice{ID: s.ids.Next(), AdministrationID: admin, State: Draft, Currency: a.Currency, Language: a.Language, CreatedAt: now, UpdatedAt: now}
	if err := s.defaults(ctx, a, user, &inv, c); err != nil {
		return Invoice{}, err
	}
	if inv.DraftID, err = s.store.NextDraftID(ctx, admin); err != nil {
		return Invoice{}, wrap("number draft", err)
	}
	if err := s.apply(ctx, a, user, &inv, c); err != nil {
		return Invoice{}, err
	}
	if err := s.save(ctx, admin, user, &inv, nil, true); err != nil {
		return Invoice{}, err
	}
	if err := s.record(ctx, user, on(inv, "sales_invoice_created")); err != nil {
		return Invoice{}, err
	}
	return inv, nil
}

// defaults fills the workflow, document style and identity of a new invoice.
func (s *Service) defaults(ctx context.Context, a administration.Listed, user resource.ID, inv *Invoice, c Changes) error {
	if c.WorkflowID == nil {
		flows, err := s.invoicing.Workflows(ctx, a.ID, user)
		if err != nil {
			return wrap("load workflows", err)
		}
		if i := slices.IndexFunc(flows, func(w invoicing.Workflow) bool { return w.Type == workflowType(*inv) && w.Default && w.Active }); i >= 0 {
			s.followWorkflow(inv, flows[i])
		}
	}
	if c.DocumentStyleID == nil {
		styles, err := s.invoicing.DocumentStyles(ctx, a.ID, user)
		if err != nil {
			return wrap("load document styles", err)
		}
		if i := slices.IndexFunc(styles, func(d invoicing.DocumentStyle) bool { return d.Default }); i >= 0 {
			inv.DocumentStyleID = styles[i].ID
		}
	}
	if c.IdentityID == nil {
		id, err := s.admins.DefaultIdentity(ctx, a.ID, user)
		if err != nil {
			return wrap("load identity", err)
		}
		inv.IdentityID = id.ID
	}
	return nil
}

// followWorkflow takes the workflow's language, currency, price mode and
// payment term.
func (s *Service) followWorkflow(inv *Invoice, w invoicing.Workflow) {
	inv.WorkflowID = w.ID
	inv.Language, inv.Currency, inv.PricesAreInclTax = w.Language, w.Currency, w.PricesAreInclTax
	if len(w.Steps) > 0 {
		inv.FirstDueInterval = w.Steps[0].DueInterval
	}
}

// Update changes an invoice. A sent invoice, even a paid one, can change as
// at Moneybird and is booked again; its state follows what is still to be
// paid. An invoice written off as uncollectible, or one in the locked
// period, stays as it is.
func (s *Service) Update(ctx context.Context, admin, id, user resource.ID, c Changes) (Invoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Invoice{}, err
	}
	inv, err := s.store.Invoice(ctx, admin, id)
	if err != nil {
		return Invoice{}, err
	}
	if inv.State == Uncollectible {
		return Invoice{}, ErrNotEditable
	}
	before := inv
	before.Lines = slices.Clone(inv.Lines)
	today := s.today(a)
	was := inv.InvoiceDate
	if inv.Sent() {
		if err := s.checkUnlocked(ctx, admin, user, was); err != nil {
			return Invoice{}, err
		}
	}
	if err := s.apply(ctx, a, user, &inv, c); err != nil {
		return Invoice{}, err
	}
	inv.UpdatedAt = s.now()
	var entries []journal.Entry
	if inv.Sent() {
		if inv.InvoiceDate == nil {
			return Invoice{}, invalid("invoice_date", "can't be blank on a sent invoice")
		}
		if inv.DueDate == nil || c.InvoiceDate != nil || c.FirstDueInterval != nil {
			due := inv.InvoiceDate.AddDate(0, 0, inv.FirstDueInterval)
			inv.DueDate = &due
		}
		if inv.State == Late {
			inv.State = reopened(inv, today)
		}
		if err := s.checkUnlocked(ctx, admin, user, inv.InvoiceDate); err != nil {
			return Invoice{}, err
		}
		if inv.Totals, err = s.Totals(ctx, admin, user, inv); err != nil {
			return Invoice{}, err
		}
		settleState(&inv, today)
		if entries, err = s.post(ctx, admin, user, inv); err != nil {
			return Invoice{}, err
		}
	}
	if err := s.save(ctx, admin, user, &inv, entries, false); err != nil {
		return Invoice{}, err
	}
	var events []event
	// A draft is still being written; only changes to a sent invoice are
	// history.
	if changed := changes(before, inv); len(changed) > 0 && before.Sent() {
		events = append(events, on(inv, "sales_invoice_updated").with(map[string]any{"changes": changed}))
	}
	if state := effective(inv, today); state != effective(before, today) {
		events = append(events, stateChanged(inv, state))
		s.notify(ctx, admin, inv.events()+"_state_changed_to_"+state, inv)
	}
	if err := s.record(ctx, user, events...); err != nil {
		return Invoice{}, err
	}
	return s.Invoice(ctx, admin, id, user)
}

// Delete removes an invoice with what it booked. As at Moneybird a sent
// invoice can go too, though crediting it is the regular way; one with
// payments, or in the locked period, cannot.
func (s *Service) Delete(ctx context.Context, admin, id, user resource.ID) error {
	inv, err := s.Invoice(ctx, admin, id, user)
	if err != nil {
		return err
	}
	if len(inv.Payments) > 0 {
		return invalid("base", "Remove the payments of this invoice first")
	}
	if inv.Sent() {
		if err := s.checkUnlocked(ctx, admin, user, inv.InvoiceDate); err != nil {
			return err
		}
	}
	if err := s.attachments.DeleteAll(ctx, admin, EntityType, id); err != nil {
		return wrap("delete attachments", err)
	}
	return s.store.DeleteInvoice(ctx, admin, id)
}

func (s *Service) checkUnlocked(ctx context.Context, admin, user resource.ID, day *time.Time) error {
	if day == nil {
		return nil
	}
	until, err := s.ledger.LockedUntil(ctx, admin, user)
	if err != nil || until == "" {
		return err
	}
	lock, err := time.Parse(time.DateOnly, until)
	if err != nil {
		return wrap("read locked period", err)
	}
	if !day.After(lock) {
		return invalid("invoice_date", "is in the locked period, which ends on "+until)
	}
	return nil
}

func optional(id resource.ID) *resource.ID {
	if id == 0 {
		return nil
	}
	return &id
}

func (s *Service) apply(ctx context.Context, a administration.Listed, user resource.ID, inv *Invoice, c Changes) error {
	admin := a.ID
	var verr resource.ValidationError
	if c.ContactID != nil {
		inv.ContactID = *c.ContactID
	}
	if c.ContactPersonID != nil {
		inv.ContactPersonID = optional(*c.ContactPersonID)
	}
	if c.WorkflowID != nil {
		w, err := s.invoicing.Workflow(ctx, admin, *c.WorkflowID, user)
		switch {
		case err != nil || w.Type != workflowType(*inv):
			verr.Add("workflow_id", "does not exist")
		case !w.Active:
			verr.Add("workflow_id", "is deactivated")
		default:
			s.followWorkflow(inv, w)
		}
	}
	if c.DocumentStyleID != nil {
		if _, err := s.invoicing.DocumentStyle(ctx, admin, *c.DocumentStyleID, user); err != nil {
			verr.Add("document_style_id", "does not exist")
		} else {
			inv.DocumentStyleID = *c.DocumentStyleID
		}
	}
	if c.IdentityID != nil {
		if _, err := s.admins.Identity(ctx, admin, *c.IdentityID, user); err != nil {
			verr.Add("identity_id", "does not exist")
		} else {
			inv.IdentityID = *c.IdentityID
		}
	}
	if c.Reference != nil {
		inv.Reference = strings.TrimSpace(*c.Reference)
	}
	if c.PaymentConditions != nil {
		inv.PaymentConditions = strings.TrimSpace(*c.PaymentConditions)
	}
	if c.Currency != nil {
		inv.Currency = strings.ToUpper(strings.TrimSpace(*c.Currency))
	}
	if c.PricesAreInclTax != nil {
		inv.PricesAreInclTax = *c.PricesAreInclTax
	}
	if c.FirstDueInterval != nil {
		inv.FirstDueInterval = *c.FirstDueInterval
	}
	if c.SequenceID != nil {
		inv.SequenceID = strings.TrimSpace(*c.SequenceID)
	}
	if c.Discount != nil {
		switch d := strings.TrimSpace(*c.Discount); d {
		case "":
			inv.Discount = 0
		default:
			p, err := ParsePercentage(d)
			if err != nil {
				verr.Add("discount", "is not a percentage from 0 to 100")
			} else {
				inv.Discount = p
			}
		}
	}
	if c.InvoiceDate != nil {
		inv.InvoiceDate = nil
		if v := strings.TrimSpace(*c.InvoiceDate); v != "" {
			d, err := time.Parse(time.DateOnly, v)
			if err != nil {
				verr.Add("invoice_date", "is not a date")
			} else {
				inv.InvoiceDate = &d
			}
		}
	}
	if inv.ContactID == 0 {
		verr.Add("contact_id", "can't be blank")
	} else if contact, err := s.contacts.Contact(ctx, admin, inv.ContactID, user); err != nil {
		verr.Add("contact_id", "does not exist")
	} else if inv.ContactPersonID != nil && !slices.ContainsFunc(contact.People, func(p contacts.Person) bool { return p.ID == *inv.ContactPersonID }) {
		verr.Add("contact_person_id", "is not a person of this contact")
	}
	if !currencyPattern.MatchString(inv.Currency) {
		verr.Add("currency", "is not a currency")
	}
	if inv.FirstDueInterval < 0 || inv.FirstDueInterval > 365 {
		verr.Add("first_due_interval", "must be from 0 to 365 days")
	}
	for field, v := range map[string]string{"reference": inv.Reference, "payment_conditions": inv.PaymentConditions} {
		if len(v) > 1000 {
			verr.Add(field, "is too long (maximum is 1000 characters)")
		}
	}
	if inv.SequenceID != "" && !regexp.MustCompile(`^\d{1,10}$`).MatchString(inv.SequenceID) {
		verr.Add("invoice_sequence_id", "is not a number")
	}
	if c.Details != nil {
		s.applyDetails(ctx, admin, user, inv, c.Details, &verr)
	}
	if c.CustomFields != nil {
		s.applyCustomFields(ctx, admin, user, inv, c.CustomFields, &verr)
	}
	return verr.OrNil()
}

const detailsField = "details_attributes"

func (s *Service) applyDetails(ctx context.Context, admin, user resource.ID, inv *Invoice, inputs []DetailInput, verr *resource.ValidationError) {
	now := s.now()
	lines := slices.Clone(inv.Lines)
	for i, in := range inputs {
		field := fmt.Sprintf("%s.%d", detailsField, i)
		var d *Detail
		if in.ID != nil {
			at := slices.IndexFunc(lines, func(d Detail) bool { return d.ID == *in.ID })
			if at < 0 {
				verr.Add(field+".id", "is not a line of this invoice")
				continue
			}
			if in.Destroy {
				lines = slices.Delete(lines, at, at+1)
				continue
			}
			d = &lines[at]
			d.UpdatedAt = now
		} else {
			if in.Destroy {
				continue
			}
			lines = append(lines, Detail{ID: s.ids.Next(), Amount: "1", Quantity: 1000, Selected: true, RowOrder: len(lines) + 1, CreatedAt: now, UpdatedAt: now})
			d = &lines[len(lines)-1]
		}
		s.applyProduct(ctx, admin, user, d, in, field, verr)
		s.applyProject(ctx, admin, user, d, in, field, verr)
		if in.Description != nil {
			d.Description = strings.TrimSpace(*in.Description)
		}
		if in.Period != nil {
			d.Period = strings.TrimSpace(*in.Period)
		}
		if in.Price != nil {
			p, err := money.ParseAmount(*in.Price)
			if err != nil {
				verr.Add(field+".price", "is not an amount")
			} else {
				d.Price = p
			}
		}
		if in.Amount != nil {
			q, err := ParseQuantity(*in.Amount)
			if err != nil {
				verr.Add(field+".amount", "does not start with a number")
			} else {
				d.Amount, d.Quantity = strings.TrimSpace(*in.Amount), q
			}
		}
		if in.TaxRateID != nil {
			d.TaxRateID = *in.TaxRateID
		}
		if in.LedgerAccountID != nil {
			d.LedgerAccountID = optional(*in.LedgerAccountID)
		}
		if in.RowOrder != nil {
			d.RowOrder = *in.RowOrder
		}
		if in.Optional != nil {
			d.Optional = *in.Optional
		}
		if in.Selected != nil {
			d.Selected = *in.Selected
		}
	}
	for i, d := range lines {
		field := fmt.Sprintf("%s.%d", detailsField, i)
		if len(d.Description) > 10000 {
			verr.Add(field+".description", "is too long (maximum is 10000 characters)")
		}
		if d.Period != "" && !periodPattern.MatchString(d.Period) {
			verr.Add(field+".period", "is not a period like 20250101..20250131")
		}
		if d.TaxRateID == 0 {
			verr.Add(field+".tax_rate_id", "can't be blank")
		} else if rate, err := s.ledger.TaxRate(ctx, admin, d.TaxRateID, user); err != nil || rate.Type != "sales_invoice" {
			verr.Add(field+".tax_rate_id", "is not a sales VAT rate")
		} else if !rate.Active {
			verr.Add(field+".tax_rate_id", "is deactivated")
		}
		if d.LedgerAccountID == nil {
			// Moneybird books lines without a category on revenue still to
			// categorize, so every line has one.
			id, err := s.ledger.SystemLedgerAccount(ctx, admin, user, "uncategorized_income")
			if err != nil {
				verr.Add(field+".ledger_account_id", "can't be blank")
				continue
			}
			lines[i].LedgerAccountID = &id
		} else if problem, err := s.ledger.Bookable(ctx, admin, user, *d.LedgerAccountID, "sales_invoice"); err != nil || problem != "" {
			verr.Add(field+".ledger_account_id", cmpString(problem, "could not be checked"))
		}
	}
	slices.SortStableFunc(lines, func(a, b Detail) int { return a.RowOrder - b.RowOrder })
	inv.Lines = lines
}

// applyProduct sets the line's product. A line that names a new product
// takes its description, price, VAT rate and category, except those the
// input sets itself.
func (s *Service) applyProduct(ctx context.Context, admin, user resource.ID, d *Detail, in DetailInput, field string, verr *resource.ValidationError) {
	if in.ProductID == nil {
		return
	}
	id := optional(*in.ProductID)
	if id == nil || (d.ProductID != nil && *d.ProductID == *id) {
		d.ProductID = id
		return
	}
	p, err := s.products.Product(ctx, admin, *id, user)
	switch {
	case err != nil:
		verr.Add(field+".product_id", "does not exist")
		return
	case !p.Active:
		verr.Add(field+".product_id", "is deactivated")
		return
	}
	d.ProductID = id
	if in.Description == nil {
		d.Description = p.LineDescription()
	}
	if in.Price == nil {
		d.Price = p.Price
	}
	if in.TaxRateID == nil && p.TaxRateID != nil {
		d.TaxRateID = *p.TaxRateID
	}
	if in.LedgerAccountID == nil {
		ledger := p.LedgerAccountID
		d.LedgerAccountID = &ledger
	}
}

// applyProject books the line on a project, which must be active when
// newly set.
func (s *Service) applyProject(ctx context.Context, admin, user resource.ID, d *Detail, in DetailInput, field string, verr *resource.ValidationError) {
	if in.ProjectID == nil {
		return
	}
	id := optional(*in.ProjectID)
	if id != nil && (d.ProjectID == nil || *d.ProjectID != *id) {
		p, err := s.projects.Project(ctx, admin, *id, user)
		switch {
		case err != nil:
			verr.Add(field+".project_id", "does not exist")
			return
		case !p.Active():
			verr.Add(field+".project_id", "is archived")
			return
		}
	}
	d.ProjectID = id
}

func cmpString(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (s *Service) applyCustomFields(ctx context.Context, admin, user resource.ID, inv *Invoice, values []FieldValue, verr *resource.ValidationError) {
	fields, err := s.admins.CustomFields(ctx, admin, user)
	if err != nil {
		verr.Add("custom_fields_attributes", "could not be checked")
		return
	}
	source, kind := administration.SourceSalesInvoice, "sales invoices"
	if inv.Type == EstimateEntityType {
		source, kind = administration.SourceEstimate, "estimates"
	}
	names := map[resource.ID]string{}
	for _, f := range fields {
		if f.Source == source {
			names[f.ID] = f.Name
		}
	}
	current := slices.Clone(inv.CustomFields)
	for i, v := range values {
		name, ok := names[v.FieldID]
		if !ok {
			verr.Add(fmt.Sprintf("custom_fields_attributes.%d", i), "is not a custom field for "+kind)
			continue
		}
		current = slices.DeleteFunc(current, func(e FieldValue) bool { return e.FieldID == v.FieldID })
		if value := strings.TrimSpace(v.Value); value != "" {
			current = append(current, FieldValue{FieldID: v.FieldID, Name: name, Value: value})
		}
	}
	inv.CustomFields = current
}

// rates loads the VAT rates of the lines, as totals need them.
func (s *Service) rates(ctx context.Context, admin, user resource.ID, inv Invoice) (map[resource.ID]bookkeeping.TaxRate, error) {
	out := map[resource.ID]bookkeeping.TaxRate{}
	for _, d := range inv.Lines {
		if _, ok := out[d.TaxRateID]; ok || d.TaxRateID == 0 {
			continue
		}
		r, err := s.ledger.TaxRate(ctx, admin, d.TaxRateID, user)
		if err != nil {
			return nil, wrap("load tax rate", err)
		}
		out[d.TaxRateID] = r
	}
	return out, nil
}

// Totals works out an invoice's amounts with its VAT rates.
func (s *Service) Totals(ctx context.Context, admin, user resource.ID, inv Invoice) (Totals, error) {
	rates, err := s.rates(ctx, admin, user, inv)
	if err != nil {
		return Totals{}, err
	}
	return ComputeInvoice(inv, rates), nil
}

// ComputeInvoice works out an invoice's amounts with the given VAT rates.
func ComputeInvoice(inv Invoice, rates map[resource.ID]bookkeeping.TaxRate) Totals {
	lines := make([]Line, len(inv.Lines))
	for i, d := range inv.Lines {
		pct, _ := ParsePercentage(rates[d.TaxRateID].Percentage)
		lines[i] = Line{Price: d.Price, Quantity: d.Quantity, TaxRateID: d.TaxRateID, Rate: pct, Optional: d.Optional, Selected: d.Selected}
	}
	return Compute(lines, inv.Discount, inv.PricesAreInclTax)
}

// post works out what a sent invoice books: the total on the debtors, each
// line's amount excluding VAT on its category, and the VAT per rate on
// payable VAT. Lines without a category go to uncategorized income.
func (s *Service) post(ctx context.Context, admin, user resource.ID, inv Invoice) ([]journal.Entry, error) {
	tot, err := s.Totals(ctx, admin, user, inv)
	if err != nil {
		return nil, err
	}
	role := func(r string) (resource.ID, error) {
		id, err := s.ledger.SystemLedgerAccount(ctx, admin, user, r)
		return id, wrap("find category "+r, err)
	}
	debtors, err := role("accounts_receivable")
	if err != nil {
		return nil, err
	}
	contact := inv.ContactID
	entry := func(ledger resource.ID, amount money.Amount, description string, rate *resource.ID) journal.Entry {
		return journal.Entry{
			ID: s.ids.Next(), AdministrationID: admin, DocumentType: inv.kind(), DocumentID: inv.ID, Date: *inv.InvoiceDate,
			LedgerAccountID: ledger, ContactID: &contact, TaxRateID: rate, Amount: amount, Description: description,
		}
	}
	number := inv.label()
	entries := []journal.Entry{entry(debtors, tot.InclTax, number, nil)}
	for i, d := range inv.Lines {
		if tot.LineBases[i] == 0 {
			continue
		}
		ledger := d.LedgerAccountID
		if ledger == nil {
			id, err := role("uncategorized_income")
			if err != nil {
				return nil, err
			}
			ledger = &id
		}
		rate := d.TaxRateID
		e := entry(*ledger, -tot.LineBases[i], d.Description, &rate)
		e.ProjectID = d.ProjectID
		entries = append(entries, e)
	}
	if tot.Tax != 0 {
		vat, err := role("payable_vat")
		if err != nil {
			return nil, err
		}
		for _, r := range tot.ByRate {
			if r.Tax != 0 {
				rate := r.TaxRateID
				entries = append(entries, entry(vat, -r.Tax, number, &rate))
			}
		}
	}
	offs, err := s.writeOffs(ctx, admin, user, inv, tot, entry)
	if err != nil {
		return nil, err
	}
	entries = append(entries, offs...)
	for i := range entries {
		entries[i].RowOrder = i
	}
	return entries, journal.Balance(entries)
}
