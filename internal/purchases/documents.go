package purchases

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

var (
	periodPattern   = regexp.MustCompile(`^\d{8}\.\.\d{8}$`)
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
)

// Document is a purchase invoice or a receipt: costs a supplier charged,
// booked when saved with its lines, and paid by hand or with a bank
// transaction.
type Document struct {
	ID               resource.ID
	AdministrationID resource.ID
	// Kind is PurchaseInvoice or Receipt.
	Kind string
	// ContactID is the supplier; zero while a new document has none, and
	// allowed on a receipt.
	ContactID   resource.ID
	Reference   string
	EntryNumber int
	Date        time.Time
	DueDate     *time.Time
	// State is new, open or paid; late and pending_payment are worked out
	// on reading.
	State            string
	Origin           string
	Currency         string
	PricesAreInclTax bool
	// RevenueInvoice is a purchase invoice the administration receives
	// money on, such as a self-billed invoice: it books the other way.
	RevenueInvoice    bool
	PaidAt            *time.Time
	Lines             []sales.Detail
	FiscalAllocations []FiscalAllocation
	Payments          []sales.Payment
	Notes             []notes.Note
	Attachments       []attachments.Attachment
	// Totals are the amounts worked out when the document was last saved;
	// reverse-charged VAT is not part of them.
	Totals sales.Totals
	// Pending reports a batched purchase transaction that pays the
	// document; the store fills it in.
	Pending   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// FiscalAllocation moves a percentage of what the lines book on a category,
// VAT included, to a private category.
type FiscalAllocation struct {
	ID                     resource.ID
	LedgerAccountID        resource.ID
	PrivateLedgerAccountID resource.ID
	// Percentage is in hundredths.
	Percentage int64
	// Price is what moves, worked out when the document was saved.
	Price     money.Amount
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Ref locates the document.
func (d Document) Ref() resource.Ref {
	t := resource.TypePurchaseInvoice
	if d.Kind == Receipt {
		t = resource.TypeReceipt
	}
	return resource.Ref{ID: d.ID, Type: t, AdministrationID: d.AdministrationID}
}

// Version changes whenever the document does.
func (d Document) Version() int64 { return d.UpdatedAt.Unix() }

// Booked reports a document that is in the books: anything but new.
func (d Document) Booked() bool { return d.State != New }

// sign is 1, or -1 for a revenue invoice, which books the other way.
func (d Document) sign() money.Amount {
	if d.RevenueInvoice {
		return -1
	}
	return 1
}

// Total is what the document asks to pay, signed: a revenue invoice and a
// credit note are negative.
func (d Document) Total() money.Amount { return d.sign() * d.Totals.InclTax }

// Paid is what payments settled, signed like Total.
func (d Document) Paid() money.Amount {
	var sum money.Amount
	for _, p := range d.Payments {
		sum += p.Price
	}
	return sum
}

// Unpaid is what is still to be paid, signed like Total.
func (d Document) Unpaid() money.Amount { return d.Total() - d.Paid() }

// EffectiveState is the state to show on the day.
func (d Document) EffectiveState(today time.Time) string {
	return effectiveState(d.State, d.DueDate, d.Pending, today)
}

// complete reports a document with what booking needs: lines, and the
// supplier of a purchase invoice.
func (d Document) complete() bool {
	return len(d.Lines) > 0 && (d.ContactID != 0 || d.Kind == Receipt)
}

// label names the document in what it posts.
func (d Document) label() string {
	if d.Kind == Receipt {
		return cmpString(d.Reference, "receipt")
	}
	return cmpString(d.Reference, "purchase invoice")
}

// Changes holds the fields to set; nil fields stay as they are. A zero
// contact id removes the contact.
type Changes struct {
	ContactID         *resource.ID
	Reference         *string
	Date              *string
	DueDate           *string
	Currency          *string
	PricesAreInclTax  *bool
	RevenueInvoice    *bool
	Details           []sales.DetailInput
	FiscalAllocations []AllocationInput
	// PaymentAccountID pays a receipt at once from this financial account,
	// such as the cash register it was paid from.
	PaymentAccountID *resource.ID
}

// AllocationInput changes, adds or removes a fiscal allocation.
type AllocationInput struct {
	ID                     *resource.ID
	LedgerAccountID        *resource.ID
	PrivateLedgerAccountID *resource.ID
	Percentage             *string
	Destroy                bool
}

func checkKind(kind string) error {
	if kind != PurchaseInvoice && kind != Receipt {
		return fmt.Errorf("unknown document kind %q", kind)
	}
	return nil
}

// Create adds a purchase invoice or receipt. With what booking needs it is
// booked at once; without, it is new.
func (s *Service) Create(ctx context.Context, admin, user resource.ID, kind string, c Changes) (Document, error) {
	if err := checkKind(kind); err != nil {
		return Document{}, err
	}
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Document{}, err
	}
	d, err := s.newDocument(ctx, a, kind, OriginAPI)
	if err != nil {
		return Document{}, err
	}
	if err := s.apply(ctx, a, user, &d, c); err != nil {
		return Document{}, err
	}
	if err := s.save(ctx, a, user, &d, true); err != nil {
		return Document{}, err
	}
	if err := s.payAtOnce(ctx, a, user, d, c.PaymentAccountID); err != nil {
		return Document{}, err
	}
	s.edited(ctx, admin, user, Document{}, d)
	return s.Document(ctx, admin, kind, d.ID, user)
}

// CreateFromFile adds a new document holding an uploaded file, to fill in
// later.
func (s *Service) CreateFromFile(ctx context.Context, admin, user resource.ID, kind, filename, contentType string, data []byte) (Document, error) {
	if err := checkKind(kind); err != nil {
		return Document{}, err
	}
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Document{}, err
	}
	d, err := s.newDocument(ctx, a, kind, OriginUpload)
	if err != nil {
		return Document{}, err
	}
	if err := s.save(ctx, a, user, &d, true); err != nil {
		return Document{}, err
	}
	if _, err := s.attachments.Add(ctx, admin, kind, d.ID, filename, contentType, data); err != nil {
		if derr := s.store.DeleteDocument(ctx, admin, d.ID); derr != nil {
			return Document{}, fmt.Errorf("%w; remove the document again: %w", err, derr)
		}
		return Document{}, err
	}
	if err := s.queue(ctx, admin, d.ID, user); err != nil {
		return Document{}, err
	}
	return s.Document(ctx, admin, kind, d.ID, user)
}

func (s *Service) newDocument(ctx context.Context, a administration.Listed, kind, origin string) (Document, error) {
	number, err := s.store.NextEntryNumber(ctx, a.ID)
	if err != nil {
		return Document{}, wrap("number document", err)
	}
	now := s.now()
	return Document{
		ID: s.ids.Next(), AdministrationID: a.ID, Kind: kind, EntryNumber: number, Date: s.today(a), State: New, Origin: origin,
		Currency: a.Currency, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Update changes a document and books it again. Payments or a locked
// period stop it.
func (s *Service) Update(ctx context.Context, admin, user resource.ID, kind string, id resource.ID, c Changes) (Document, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Document{}, err
	}
	d, err := s.store.Document(ctx, admin, kind, id)
	if err != nil {
		return Document{}, err
	}
	if d.State == Paid || len(d.Payments) > 0 {
		return Document{}, ErrNotEditable
	}
	if d.Booked() {
		if err := s.checkDay(ctx, admin, user, d.Date, "date"); err != nil {
			return Document{}, err
		}
	}
	before := d
	before.Lines = slices.Clone(d.Lines)
	if err := s.apply(ctx, a, user, &d, c); err != nil {
		return Document{}, err
	}
	d.UpdatedAt = s.now()
	if err := s.save(ctx, a, user, &d, false); err != nil {
		return Document{}, err
	}
	if err := s.payAtOnce(ctx, a, user, d, c.PaymentAccountID); err != nil {
		return Document{}, err
	}
	s.edited(ctx, admin, user, before, d)
	s.confirmed(ctx, admin, id, c.confirmedFields()...)
	return s.Document(ctx, admin, kind, id, user)
}

// save works out the totals and the state and stores the document with
// what it posts: nothing while it is new.
func (s *Service) save(ctx context.Context, a administration.Listed, user resource.ID, d *Document, created bool) error {
	rates, err := s.rates(ctx, a.ID, user, *d)
	if err != nil {
		return err
	}
	d.Totals = computeTotals(*d, rates)
	allocate(d, rates)
	entries := []journal.Entry{}
	if d.complete() {
		if err := s.checkDay(ctx, a.ID, user, d.Date, "date"); err != nil {
			return err
		}
		d.State = Open
		if entries, err = s.post(ctx, a.ID, user, *d, rates); err != nil {
			return err
		}
	} else {
		d.State = New
	}
	return wrap("store document", s.store.SaveDocument(ctx, *d, entries, created))
}

// payAtOnce pays a receipt's unpaid amount from the financial account, as
// when it was paid in cash or by card.
func (s *Service) payAtOnce(ctx context.Context, a administration.Listed, user resource.ID, d Document, account *resource.ID) error {
	if account == nil || *account == 0 || !d.Booked() || d.Unpaid() == 0 {
		return nil
	}
	day := d.Date.Format(time.DateOnly)
	_, err := s.registerPayment(ctx, a.ID, user, d.Kind, d.ID, sales.PaymentInput{PaymentDate: day, FinancialAccountID: account, ManualPaymentAction: sales.CashPayment})
	return err
}

func (s *Service) apply(ctx context.Context, a administration.Listed, user resource.ID, d *Document, c Changes) error {
	var verr resource.ValidationError
	if c.ContactID != nil {
		d.ContactID = *c.ContactID
	}
	if d.ContactID != 0 {
		if _, err := s.contacts.Contact(ctx, a.ID, d.ContactID, user); err != nil {
			verr.Add("contact_id", "does not exist")
		}
	}
	if c.Reference != nil {
		if d.Reference = strings.TrimSpace(*c.Reference); len(d.Reference) > 1000 {
			verr.Add("reference", "is too long (maximum is 1000 characters)")
		}
	}
	if c.Currency != nil {
		d.Currency = strings.ToUpper(strings.TrimSpace(*c.Currency))
	}
	if !currencyPattern.MatchString(d.Currency) {
		verr.Add("currency", "is not a currency")
	}
	if c.PricesAreInclTax != nil {
		d.PricesAreInclTax = *c.PricesAreInclTax
	}
	if c.RevenueInvoice != nil {
		if d.Kind == Receipt && *c.RevenueInvoice {
			verr.Add("revenue_invoice", "is only for purchase invoices")
		}
		d.RevenueInvoice = *c.RevenueInvoice
	}
	if c.Date != nil {
		if day, err := time.Parse(time.DateOnly, strings.TrimSpace(*c.Date)); err != nil {
			verr.Add("date", "is not a date")
		} else {
			d.Date = day
		}
	}
	if c.DueDate != nil {
		d.DueDate = nil
		if v := strings.TrimSpace(*c.DueDate); v != "" {
			if day, err := time.Parse(time.DateOnly, v); err != nil {
				verr.Add("due_date", "is not a date")
			} else {
				d.DueDate = &day
			}
		}
	}
	if d.DueDate != nil && d.DueDate.Before(d.Date) {
		verr.Add("due_date", "must not be before the date")
	}
	if c.Details != nil {
		s.applyDetails(ctx, a.ID, user, d, c.Details, &verr)
		if d.Booked() && len(d.Lines) == 0 {
			verr.Add(detailsField, "can't be empty on a booked document")
		}
	}
	if d.Booked() && d.ContactID == 0 && d.Kind == PurchaseInvoice {
		verr.Add("contact_id", "can't be blank")
	}
	if c.FiscalAllocations != nil {
		s.applyAllocations(ctx, a.ID, user, d, c.FiscalAllocations, &verr)
	}
	if c.PaymentAccountID != nil && *c.PaymentAccountID != 0 {
		if d.Kind != Receipt {
			verr.Add("payment.financial_account_id", "is only for receipts")
		} else if s.bank == nil {
			verr.Add("payment.financial_account_id", "does not exist")
		} else if _, err := s.bank.FinancialAccount(ctx, a.ID, *c.PaymentAccountID, user); err != nil {
			verr.Add("payment.financial_account_id", "does not exist")
		}
	}
	return verr.OrNil()
}

const detailsField = "details_attributes"

func (s *Service) applyDetails(ctx context.Context, admin, user resource.ID, doc *Document, inputs []sales.DetailInput, verr *resource.ValidationError) {
	now := s.now()
	lines := slices.Clone(doc.Lines)
	for i, in := range inputs {
		field := fmt.Sprintf("%s.%d", detailsField, i)
		var d *sales.Detail
		if in.ID != nil {
			at := slices.IndexFunc(lines, func(d sales.Detail) bool { return d.ID == *in.ID })
			if at < 0 {
				verr.Add(field+".id", "is not a line of this document")
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
			lines = append(lines, sales.Detail{ID: s.ids.Next(), Amount: "1", Quantity: 1000, Selected: true, RowOrder: len(lines) + 1, CreatedAt: now, UpdatedAt: now})
			d = &lines[len(lines)-1]
		}
		s.applyLine(ctx, admin, user, d, in, field, verr)
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
		} else if rate, err := s.ledger.TaxRate(ctx, admin, d.TaxRateID, user); err != nil || rate.Type != "purchase_invoice" {
			verr.Add(field+".tax_rate_id", "is not a purchase VAT rate")
		} else if !rate.Active {
			verr.Add(field+".tax_rate_id", "is deactivated")
		}
		if d.LedgerAccountID == nil {
			// Lines without a category book on expenses still to categorize,
			// as at Moneybird.
			id, err := s.ledger.SystemLedgerAccount(ctx, admin, user, "uncategorized_expenses")
			if err != nil {
				verr.Add(field+".ledger_account_id", "can't be blank")
				continue
			}
			lines[i].LedgerAccountID = &id
		} else if problem, err := s.ledger.Bookable(ctx, admin, user, *d.LedgerAccountID, "purchase_invoice"); err != nil || problem != "" {
			verr.Add(field+".ledger_account_id", cmpString(problem, "could not be checked"))
		}
	}
	slices.SortStableFunc(lines, func(a, b sales.Detail) int { return a.RowOrder - b.RowOrder })
	doc.Lines = lines
}

// applyLine sets the fields of one line.
func (s *Service) applyLine(ctx context.Context, admin, user resource.ID, d *sales.Detail, in sales.DetailInput, field string, verr *resource.ValidationError) {
	if in.ProductID != nil {
		id := optional(*in.ProductID)
		if id != nil && (d.ProductID == nil || *d.ProductID != *id) {
			p, err := s.products.Product(ctx, admin, *id, user)
			if err != nil {
				verr.Add(field+".product_id", "does not exist")
			} else if in.Description == nil && d.Description == "" {
				d.Description = p.LineDescription()
			}
		}
		d.ProductID = id
	}
	if in.ProjectID != nil {
		id := optional(*in.ProjectID)
		if id != nil && (d.ProjectID == nil || *d.ProjectID != *id) {
			p, err := s.projects.Project(ctx, admin, *id, user)
			switch {
			case err != nil:
				verr.Add(field+".project_id", "does not exist")
			case !p.Active():
				verr.Add(field+".project_id", "is archived")
			}
		}
		d.ProjectID = id
	}
	if in.Description != nil {
		d.Description = strings.TrimSpace(*in.Description)
	}
	if in.Period != nil {
		d.Period = strings.TrimSpace(*in.Period)
	}
	if in.Price != nil {
		if p, err := money.ParseAmount(*in.Price); err != nil {
			verr.Add(field+".price", "is not an amount")
		} else {
			d.Price = p
		}
	}
	if in.Amount != nil {
		if q, err := sales.ParseQuantity(*in.Amount); err != nil {
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
}

const allocationsField = "fiscal_allocations_attributes"

func (s *Service) applyAllocations(ctx context.Context, admin, user resource.ID, doc *Document, inputs []AllocationInput, verr *resource.ValidationError) {
	now := s.now()
	list := slices.Clone(doc.FiscalAllocations)
	for i, in := range inputs {
		field := fmt.Sprintf("%s.%d", allocationsField, i)
		var f *FiscalAllocation
		if in.ID != nil {
			at := slices.IndexFunc(list, func(f FiscalAllocation) bool { return f.ID == *in.ID })
			if at < 0 {
				verr.Add(field+".id", "is not a fiscal allocation of this document")
				continue
			}
			if in.Destroy {
				list = slices.Delete(list, at, at+1)
				continue
			}
			f = &list[at]
			f.UpdatedAt = now
		} else {
			if in.Destroy {
				continue
			}
			list = append(list, FiscalAllocation{ID: s.ids.Next(), CreatedAt: now, UpdatedAt: now})
			f = &list[len(list)-1]
		}
		if in.LedgerAccountID != nil {
			f.LedgerAccountID = *in.LedgerAccountID
		}
		if in.PrivateLedgerAccountID != nil {
			f.PrivateLedgerAccountID = *in.PrivateLedgerAccountID
		}
		if in.Percentage != nil {
			p, err := sales.ParsePercentage(*in.Percentage)
			if err != nil || p == 0 {
				verr.Add(field+".percentage", "is not a percentage from 0 to 100")
			} else {
				f.Percentage = p
			}
		}
		for _, l := range []struct {
			id   resource.ID
			name string
		}{{f.LedgerAccountID, "ledger_account_id"}, {f.PrivateLedgerAccountID, "private_ledger_account_id"}} {
			if l.id == 0 {
				verr.Add(field+"."+l.name, "can't be blank")
				continue
			}
			if _, err := s.ledger.LedgerAccount(ctx, admin, l.id, user); err != nil {
				verr.Add(field+"."+l.name, "is not a category of this administration")
			}
		}
		if f.Percentage == 0 && in.Percentage == nil {
			verr.Add(field+".percentage", "can't be blank")
		}
	}
	doc.FiscalAllocations = list
}

// rates loads the VAT rates of the lines.
func (s *Service) rates(ctx context.Context, admin, user resource.ID, d Document) (map[resource.ID]bookkeeping.TaxRate, error) {
	out := map[resource.ID]bookkeeping.TaxRate{}
	for _, l := range d.Lines {
		if _, ok := out[l.TaxRateID]; ok || l.TaxRateID == 0 {
			continue
		}
		r, err := s.ledger.TaxRate(ctx, admin, l.TaxRateID, user)
		if err != nil {
			return nil, wrap("load tax rate", err)
		}
		out[l.TaxRateID] = r
	}
	return out, nil
}

// reverseCharged reports a rate whose VAT the administration owes itself,
// such as for services from abroad: the supplier charges none, so the VAT
// is not part of the total.
func reverseCharged(r bookkeeping.TaxRate) bool {
	pct, _ := sales.ParsePercentage(r.Percentage)
	return !r.ShowTax && pct > 0
}

// computeTotals works out the document's amounts; VAT of reverse-charged
// rates is left out, as the supplier did not charge it.
func computeTotals(d Document, rates map[resource.ID]bookkeeping.TaxRate) sales.Totals {
	lines := make([]sales.Line, len(d.Lines))
	for i, l := range d.Lines {
		r := rates[l.TaxRateID]
		pct, _ := sales.ParsePercentage(r.Percentage)
		if reverseCharged(r) {
			pct = 0
		}
		lines[i] = sales.Line{Price: l.Price, Quantity: l.Quantity, TaxRateID: l.TaxRateID, Rate: pct, Selected: true}
	}
	return sales.Compute(lines, 0, d.PricesAreInclTax)
}

// reverseTax is the VAT per reverse-charged rate.
func reverseTax(d Document, rates map[resource.ID]bookkeeping.TaxRate) []sales.RateTotal {
	var out []sales.RateTotal
	for i, l := range d.Lines {
		r := rates[l.TaxRateID]
		if !reverseCharged(r) || i >= len(d.Totals.LineBases) {
			continue
		}
		pct, _ := sales.ParsePercentage(r.Percentage)
		at := slices.IndexFunc(out, func(t sales.RateTotal) bool { return t.TaxRateID == l.TaxRateID })
		if at < 0 {
			out = append(out, sales.RateTotal{TaxRateID: l.TaxRateID})
			at = len(out) - 1
		}
		out[at].Taxable += d.Totals.LineBases[i]
		out[at].Tax = money.Amount(divRound(int64(out[at].Taxable)*pct, 10000))
	}
	return out
}

// lineTax is the charged VAT of line i: its share of its rate's VAT.
func lineTax(d Document, i int) money.Amount {
	l := d.Lines[i]
	for _, r := range d.Totals.ByRate {
		if r.TaxRateID == l.TaxRateID && r.Taxable != 0 {
			return money.Amount(divRound(int64(r.Tax)*int64(d.Totals.LineBases[i]), int64(r.Taxable)))
		}
	}
	return 0
}

// allocate works out what each fiscal allocation moves: its percentage of
// what the lines book on its category, VAT included.
func allocate(d *Document, _ map[resource.ID]bookkeeping.TaxRate) {
	for i := range d.FiscalAllocations {
		f := &d.FiscalAllocations[i]
		var gross int64
		for j, l := range d.Lines {
			if l.LedgerAccountID != nil && *l.LedgerAccountID == f.LedgerAccountID && j < len(d.Totals.LineBases) {
				gross += int64(d.Totals.LineBases[j] + lineTax(*d, j))
			}
		}
		f.Price = money.Amount(divRound(gross*f.Percentage, 10000))
	}
}

// divRound divides and rounds half away from zero.
func divRound(n, d int64) int64 {
	if d < 0 {
		n, d = -n, -d
	}
	if n < 0 {
		return -((-n + d/2) / d)
	}
	return (n + d/2) / d
}

// post works out what a document books: the total on the creditors (a
// credit), each line's amount excluding VAT on its category, the VAT per
// rate on receivable VAT, reverse-charged VAT on both receivable and
// payable VAT, and fiscal allocations moved to their private category. A
// revenue invoice books the other way.
func (s *Service) post(ctx context.Context, admin, user resource.ID, d Document, rates map[resource.ID]bookkeeping.TaxRate) ([]journal.Entry, error) {
	role := func(r string) (resource.ID, error) {
		id, err := s.ledger.SystemLedgerAccount(ctx, admin, user, r)
		return id, wrap("find category "+r, err)
	}
	creditors, err := role("accounts_payable")
	if err != nil {
		return nil, err
	}
	sign := d.sign()
	contact := optional(d.ContactID)
	entry := func(ledger resource.ID, amount money.Amount, description string, rate *resource.ID) journal.Entry {
		return journal.Entry{
			ID: s.ids.Next(), AdministrationID: admin, DocumentType: d.Kind, DocumentID: d.ID, Date: d.Date,
			LedgerAccountID: ledger, ContactID: contact, TaxRateID: rate, Amount: sign * amount, Description: description,
		}
	}
	label := d.label()
	entries := []journal.Entry{entry(creditors, -d.Totals.InclTax, label, nil)}
	for i, l := range d.Lines {
		if d.Totals.LineBases[i] == 0 {
			continue
		}
		rate := l.TaxRateID
		e := entry(*l.LedgerAccountID, d.Totals.LineBases[i], cmpString(l.Description, label), &rate)
		e.ProjectID = l.ProjectID
		entries = append(entries, e)
	}
	reverse := reverseTax(d, rates)
	var receivable, payable resource.ID
	if d.Totals.Tax != 0 || len(reverse) > 0 || len(d.FiscalAllocations) > 0 {
		if receivable, err = role("receivable_vat"); err != nil {
			return nil, err
		}
	}
	for _, r := range d.Totals.ByRate {
		if r.Tax != 0 {
			rate := r.TaxRateID
			entries = append(entries, entry(receivable, r.Tax, label, &rate))
		}
	}
	for _, r := range reverse {
		if r.Tax == 0 {
			continue
		}
		if payable == 0 {
			if payable, err = role("payable_vat"); err != nil {
				return nil, err
			}
		}
		rate := r.TaxRateID
		entries = append(entries, entry(receivable, r.Tax, label, &rate), entry(payable, -r.Tax, label, &rate))
	}
	for _, f := range d.FiscalAllocations {
		if f.Price == 0 {
			continue
		}
		var base int64
		for j, l := range d.Lines {
			if l.LedgerAccountID != nil && *l.LedgerAccountID == f.LedgerAccountID {
				base += int64(d.Totals.LineBases[j])
			}
		}
		baseShare := money.Amount(divRound(base*f.Percentage, 10000))
		entries = append(entries, entry(f.PrivateLedgerAccountID, f.Price, "fiscal allocation "+label, nil), entry(f.LedgerAccountID, -baseShare, "fiscal allocation "+label, nil))
		if taxShare := f.Price - baseShare; taxShare != 0 {
			entries = append(entries, entry(receivable, -taxShare, "fiscal allocation "+label, nil))
		}
	}
	for i := range entries {
		entries[i].RowOrder = i
	}
	return entries, journal.Balance(entries)
}

// Delete removes a document with what it posts. Payments made with bank
// transactions are unlinked first, so the transactions are open again.
func (s *Service) Delete(ctx context.Context, admin, user resource.ID, kind string, id resource.ID) error {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return err
	}
	d, err := s.store.Document(ctx, admin, kind, id)
	if err != nil {
		return err
	}
	if d.Booked() {
		if err := s.checkDay(ctx, admin, user, d.Date, "date"); err != nil {
			return err
		}
	}
	for _, p := range d.Payments {
		if err := s.checkDay(ctx, admin, user, p.PaymentDate, "payment_date"); err != nil {
			return err
		}
	}
	for _, p := range d.Payments {
		if p.FinancialMutationID != nil {
			if s.bank == nil {
				return ErrNotEditable
			}
			if err := s.bank.UnlinkPayment(ctx, admin, *p.FinancialMutationID, user, p.ID); err != nil {
				return err
			}
		}
	}
	if err := s.attachments.DeleteAll(ctx, admin, kind, id); err != nil {
		return wrap("delete attachments", err)
	}
	if err := s.store.DeleteDocument(ctx, admin, id); err != nil {
		return wrap("delete document", err)
	}
	return s.forget(ctx, admin, id)
}

// List returns the page of documents of the kind the query selects.
func (s *Service) List(ctx context.Context, admin, user resource.ID, kind string, q Query) ([]Document, int, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, 0, err
	}
	q.Kind, q.Today = kind, s.today(a)
	pageOf(&q)
	list, total, err := s.store.Documents(ctx, admin, q)
	if err != nil {
		return nil, 0, fmt.Errorf("list documents: %w", err)
	}
	list, err = s.complete(ctx, admin, kind, list)
	return list, total, err
}

// Versions lists the documents of the kind the query selects, for
// synchronization.
func (s *Service) Versions(ctx context.Context, admin, user resource.ID, kind string, q Query) ([]Document, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	q.Kind, q.Today, q.PerPage = kind, s.today(a), 0
	list, _, err := s.store.Documents(ctx, admin, q)
	return list, err
}

// DocumentsByID returns the documents of the kind with these ids.
func (s *Service) DocumentsByID(ctx context.Context, admin, user resource.ID, kind string, ids []resource.ID) ([]Document, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	if err := checkIDs(ids); err != nil {
		return nil, err
	}
	list, err := s.store.DocumentsByID(ctx, admin, kind, ids)
	if err != nil {
		return nil, err
	}
	return s.complete(ctx, admin, kind, list)
}

// Document returns one document of the kind.
func (s *Service) Document(ctx context.Context, admin resource.ID, kind string, id, user resource.ID) (Document, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Document{}, err
	}
	d, err := s.store.Document(ctx, admin, kind, id)
	if err != nil {
		return Document{}, err
	}
	list, err := s.complete(ctx, admin, kind, []Document{d})
	if err != nil {
		return Document{}, err
	}
	return list[0], nil
}

// complete adds notes and attachments.
func (s *Service) complete(ctx context.Context, admin resource.ID, kind string, list []Document) ([]Document, error) {
	ids := make([]resource.ID, len(list))
	for i, d := range list {
		ids[i] = d.ID
	}
	byDoc, err := s.notes.Of(ctx, admin, kind, ids)
	if err != nil {
		return nil, err
	}
	files, err := s.attachments.Of(ctx, admin, kind, ids)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Notes, list[i].Attachments = byDoc[list[i].ID], files[list[i].ID]
	}
	return list, nil
}

// AddNote adds a note to a document.
func (s *Service) AddNote(ctx context.Context, admin, user resource.ID, kind string, id resource.ID, in notes.Input) (notes.Note, error) {
	if _, err := s.Document(ctx, admin, kind, id, user); err != nil {
		return notes.Note{}, err
	}
	n, err := s.notes.Add(ctx, admin, kind, id, user, in)
	if err != nil {
		return notes.Note{}, err
	}
	return n, s.store.TouchDocument(ctx, admin, id, n.CreatedAt)
}

// DeleteNote removes a note from a document.
func (s *Service) DeleteNote(ctx context.Context, admin, user resource.ID, kind string, id, noteID resource.ID) error {
	if _, err := s.Document(ctx, admin, kind, id, user); err != nil {
		return err
	}
	if err := s.notes.Delete(ctx, admin, kind, id, noteID); err != nil {
		return err
	}
	return s.store.TouchDocument(ctx, admin, id, s.now())
}

// AddAttachment attaches a file to a document.
// A new document has the file read by Scan and Recognise.
func (s *Service) AddAttachment(ctx context.Context, admin, user resource.ID, kind string, id resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error) {
	d, err := s.Document(ctx, admin, kind, id, user)
	if err != nil {
		return attachments.Attachment{}, err
	}
	a, err := s.attachments.Add(ctx, admin, kind, id, filename, contentType, data)
	if err != nil {
		return attachments.Attachment{}, err
	}
	if err := s.store.TouchDocument(ctx, admin, id, a.CreatedAt); err != nil {
		return a, err
	}
	if !d.Booked() {
		return a, s.queue(ctx, admin, id, user)
	}
	return a, nil
}

// OpenAttachment returns an attachment of a document; the caller closes it.
func (s *Service) OpenAttachment(ctx context.Context, admin, user resource.ID, kind string, id, attachment resource.ID) (attachments.Attachment, io.ReadCloser, error) {
	if _, err := s.Document(ctx, admin, kind, id, user); err != nil {
		return attachments.Attachment{}, nil, err
	}
	return s.attachments.Open(ctx, admin, kind, id, attachment)
}

// DeleteAttachment removes an attachment from a document.
func (s *Service) DeleteAttachment(ctx context.Context, admin, user resource.ID, kind string, id, attachment resource.ID) error {
	if _, err := s.Document(ctx, admin, kind, id, user); err != nil {
		return err
	}
	if err := s.attachments.Delete(ctx, admin, kind, id, attachment); err != nil {
		return err
	}
	return s.store.TouchDocument(ctx, admin, id, s.now())
}
