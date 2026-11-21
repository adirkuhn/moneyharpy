package sales

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/money"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ImportedDetail is an invoice line brought in with the id it had.
type ImportedDetail struct {
	Origin resource.Origin
	Input  DetailInput
}

// ImportedPayment is a payment brought in with an invoice. A payment linked
// to a bank transaction is made with that transaction, which must be here.
type ImportedPayment struct {
	Origin resource.Origin
	Input  PaymentInput
}

// Writeoff is how an invoice was written off: dates as yyyy-mm-dd.
type Writeoff struct {
	DubiousOn       string
	UncollectibleOn string
}

// ImportedInvoice is a sales invoice brought in from another system, such
// as Moneybird, as it was there: its number, dates and payments.
type ImportedInvoice struct {
	Origin  resource.Origin
	Changes Changes
	Details []ImportedDetail
	DraftID int
	// InvoiceID is the number of a sent invoice; empty for a draft.
	InvoiceID string
	// Sent tells a sent invoice from a draft; a scheduled one comes in as a
	// draft, so nothing sends it on its own.
	Sent             bool
	DueDate          *time.Time
	SentAt           *time.Time
	PaymentReference string
	// OriginalID is the invoice a credit invoice credits.
	OriginalID *resource.ID
	// OriginalEstimateID and RecurringID are where the invoice came from;
	// both must be here already.
	OriginalEstimateID, RecurringID *resource.ID
	Payments                        []ImportedPayment
	Writeoff                        Writeoff
}

// ImportInvoice adds a sales invoice brought in from another system,
// keeping its id, number and dates, and books it as the books here would
// have: a sent one is posted on its invoice date, its payments are made,
// and a write-off is booked last. Nothing is sent to the contact and no
// number is taken from the administration's sequence. Bringing in an
// invoice that is here already changes nothing.
func (s *Service) ImportInvoice(ctx context.Context, admin, user resource.ID, in ImportedInvoice) (Invoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Invoice{}, err
	}
	if inv, err := s.store.Invoice(ctx, admin, in.Origin.ID); err == nil {
		return inv, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Invoice{}, err
	}
	created, updated := in.Origin.Or(s.now())
	inv := Invoice{ID: in.Origin.ID, AdministrationID: admin, State: Draft, Currency: a.Currency, Language: a.Language, CreatedAt: created, UpdatedAt: updated}
	if err := s.defaults(ctx, a, user, &inv, in.Changes); err != nil {
		return Invoice{}, err
	}
	inv.DraftID = in.DraftID
	if inv.DraftID == 0 {
		if inv.DraftID, err = s.store.NextDraftID(ctx, admin); err != nil {
			return Invoice{}, wrap("number draft", err)
		}
	}
	c := in.Changes
	c.Details = make([]DetailInput, len(in.Details))
	for i, d := range in.Details {
		c.Details[i] = d.Input
	}
	if err := s.apply(ctx, a, user, &inv, c); err != nil {
		return Invoice{}, err
	}
	keepLineIDs(inv.Lines, in.Details, created)
	inv.OriginalEstimateID, inv.RecurringSalesInvoiceID = in.OriginalEstimateID, in.RecurringID
	if in.Sent {
		if inv.InvoiceDate == nil {
			return Invoice{}, invalid("invoice_date", "can't be blank")
		}
		inv.State, inv.InvoiceID, inv.PaymentReference, inv.OriginalID = Open, in.InvoiceID, in.PaymentReference, in.OriginalID
		inv.SentAt, inv.DueDate = in.SentAt, in.DueDate
		if inv.SentAt == nil {
			inv.SentAt = &created
		}
		if inv.DueDate == nil {
			due := inv.InvoiceDate.AddDate(0, 0, inv.FirstDueInterval)
			inv.DueDate = &due
		}
	}
	// A draft is not in the books: it posts nothing.
	var entries []journal.Entry
	if inv.Sent() {
		if entries, err = s.post(ctx, admin, user, inv); err != nil {
			return Invoice{}, err
		}
	}
	if err := s.save(ctx, admin, user, &inv, entries, true); err != nil {
		return Invoice{}, err
	}
	if err := s.importPayments(ctx, admin, user, EntityType, inv.ID, in.Payments); err != nil {
		return Invoice{}, err
	}
	if err := s.importWriteoff(ctx, admin, user, EntityType, inv.ID, in.Writeoff); err != nil {
		return Invoice{}, err
	}
	return s.store.Invoice(ctx, admin, inv.ID)
}

// ImportedExternal is an external invoice brought in from another system.
type ImportedExternal struct {
	Origin      resource.Origin
	Changes     ExternalChanges
	Details     []ImportedDetail
	EntryNumber int
	Payments    []ImportedPayment
	Writeoff    Writeoff
}

// ImportExternal adds an external invoice brought in from another system,
// keeping its id and entry number, and books it with its payments and
// write-off as ImportInvoice does.
func (s *Service) ImportExternal(ctx context.Context, admin, user resource.ID, in ImportedExternal) (ExternalInvoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return ExternalInvoice{}, err
	}
	if e, err := s.store.ExternalInvoice(ctx, admin, in.Origin.ID); err == nil {
		return e, nil
	} else if !errors.Is(err, ErrNotFound) {
		return ExternalInvoice{}, err
	}
	e, err := s.newExternal(ctx, a, OriginAPI)
	if err != nil {
		return ExternalInvoice{}, err
	}
	created, updated := in.Origin.Or(s.now())
	e.ID, e.CreatedAt, e.UpdatedAt = in.Origin.ID, created, updated
	if in.EntryNumber > 0 {
		e.EntryNumber = in.EntryNumber
	}
	c := in.Changes
	c.Details = make([]DetailInput, len(in.Details))
	for i, d := range in.Details {
		c.Details[i] = d.Input
	}
	if err := s.applyExternal(ctx, a, user, &e, c); err != nil {
		return ExternalInvoice{}, err
	}
	keepLineIDs(e.Lines, in.Details, created)
	if err := s.saveExternal(ctx, a, user, &e, true); err != nil {
		return ExternalInvoice{}, err
	}
	if err := s.importPayments(ctx, admin, user, ExternalEntityType, e.ID, in.Payments); err != nil {
		return ExternalInvoice{}, err
	}
	if err := s.importWriteoff(ctx, admin, user, ExternalEntityType, e.ID, in.Writeoff); err != nil {
		return ExternalInvoice{}, err
	}
	return s.store.ExternalInvoice(ctx, admin, e.ID)
}

// keepLineIDs gives the lines made from the details the ids they had.
func keepLineIDs(lines []Detail, details []ImportedDetail, created time.Time) {
	for i := range lines {
		if i >= len(details) || details[i].Origin.ID == 0 {
			continue
		}
		lines[i].ID = details[i].Origin.ID
		lines[i].CreatedAt, lines[i].UpdatedAt = details[i].Origin.Or(created)
	}
}

// importPayments makes an imported invoice's payments, oldest first.
// ImportPayments brings in the payments the source has on an invoice that
// came in before that are not here, such as one refused then. It reports
// whether it added any; the payments here stay.
func (s *Service) ImportPayments(ctx context.Context, admin, user resource.ID, kind string, id resource.ID, payments []ImportedPayment) (bool, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return false, err
	}
	inv, err := s.loadInvoice(ctx, admin, kind, id)
	if err != nil {
		return false, err
	}
	missing := MissingPayments(inv.Payments, payments)
	if len(missing) == 0 || inv.State == Draft {
		return false, nil
	}
	return true, s.importPayments(ctx, admin, user, kind, id, missing)
}

func (s *Service) importPayments(ctx context.Context, admin, user resource.ID, kind string, id resource.ID, payments []ImportedPayment) error {
	for i, p := range payments {
		in := p.Input
		in.Imported = true
		if _, err := s.registerPayment(ctx, admin, user, kind, id, in); err != nil {
			return fmt.Errorf("payment %d: %w", i+1, err)
		}
	}
	return nil
}

// importWriteoff books how an imported invoice was written off, after its
// payments.
func (s *Service) importWriteoff(ctx context.Context, admin, user resource.ID, kind string, id resource.ID, w Writeoff) error {
	if w.DubiousOn != "" {
		if err := s.markDubious(ctx, admin, user, kind, id, w.DubiousOn); err != nil {
			return fmt.Errorf("doubtful: %w", err)
		}
	}
	if w.UncollectibleOn != "" {
		if err := s.markUncollectible(ctx, admin, user, kind, id, w.UncollectibleOn, ""); err != nil {
			return fmt.Errorf("uncollectible: %w", err)
		}
	}
	return nil
}

// ImportedEstimate is an estimate brought in from another system as it was
// there: its number, dates and state.
type ImportedEstimate struct {
	Origin  resource.Origin
	Changes EstimateChanges
	Details []ImportedDetail
	DraftID int
	// EstimateID is the number of a sent estimate; empty for a draft.
	EstimateID string
	// State is draft, open, late, accepted, rejected, billed or archived.
	State                              string
	DueDate, SentAt                    *time.Time
	AcceptedAt, RejectedAt, ArchivedAt *time.Time
}

// ImportEstimate adds an estimate brought in from another system, keeping
// its id, number, dates and state. Estimates book nothing, and nothing is
// sent. Bringing in an estimate that is here already changes nothing.
func (s *Service) ImportEstimate(ctx context.Context, admin, user resource.ID, in ImportedEstimate) (Estimate, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Estimate{}, err
	}
	if e, err := s.store.Estimate(ctx, admin, in.Origin.ID); err == nil {
		return e, nil
	} else if !IsNotFound(err) {
		return Estimate{}, err
	}
	created, updated := in.Origin.Or(s.now())
	e := Estimate{ID: in.Origin.ID, AdministrationID: admin, State: Draft, Currency: a.Currency, Language: a.Language, ShowTax: true, CreatedAt: created, UpdatedAt: updated}
	c := in.Changes
	c.Details = make([]DetailInput, len(in.Details))
	for i, d := range in.Details {
		c.Details[i] = d.Input
	}
	inv := e.invoice()
	if err := s.defaults(ctx, a, user, &inv, c.invoiceChanges()); err != nil {
		return Estimate{}, err
	}
	e.take(inv)
	e.DraftID = in.DraftID
	if e.DraftID == 0 {
		if e.DraftID, err = s.store.NextEstimateDraftID(ctx, admin); err != nil {
			return Estimate{}, wrap("number draft", err)
		}
	}
	if err := s.applyEstimate(ctx, a, user, &e, c); err != nil {
		return Estimate{}, err
	}
	keepLineIDs(e.Lines, in.Details, created)
	if in.State != "" && in.State != Draft {
		e.State, e.EstimateID = in.State, in.EstimateID
		if e.State == Late {
			// Late is worked out from the due date.
			e.State = Open
		}
		e.SentAt, e.AcceptedAt, e.RejectedAt, e.ArchivedAt = in.SentAt, in.AcceptedAt, in.RejectedAt, in.ArchivedAt
		if in.DueDate != nil {
			e.DueDate = in.DueDate
		}
		if e.SentAt == nil {
			e.SentAt = &created
		}
	}
	if err := s.saveEstimate(ctx, admin, user, &e, true); err != nil {
		return Estimate{}, err
	}
	return s.store.Estimate(ctx, admin, e.ID)
}

// ImportedRecurring is a recurring invoice brought in from another system.
type ImportedRecurring struct {
	Origin  resource.Origin
	Changes RecurringChanges
	Details []ImportedDetail
	// StartDate and LastDate are as they were; the next invoice date is in
	// Changes when it lies ahead.
	StartDate *time.Time
}

// ImportRecurring adds a recurring invoice brought in from another system,
// keeping its id and schedule, but stopped and without sending by itself:
// while the other system still makes the invoices, starting it here would
// bill the contact twice. Bringing in one that is here already changes
// nothing.
func (s *Service) ImportRecurring(ctx context.Context, admin, user resource.ID, in ImportedRecurring) (Recurring, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Recurring{}, err
	}
	if r, err := s.store.RecurringInvoice(ctx, admin, in.Origin.ID); err == nil {
		return r, nil
	} else if !IsNotFound(err) {
		return Recurring{}, err
	}
	created, updated := in.Origin.Or(s.now())
	tomorrow := s.today(a).AddDate(0, 0, 1)
	r := Recurring{
		ID: in.Origin.ID, AdministrationID: admin, Active: true, Currency: a.Currency, Language: a.Language, FrequencyType: Month, Frequency: 1,
		StartDate: tomorrow, InvoiceDate: tomorrow, Anchor: tomorrow, CreatedAt: created, UpdatedAt: updated,
	}
	c := in.Changes
	c.Details = make([]DetailInput, len(in.Details))
	for i, d := range in.Details {
		c.Details[i] = d.Input
	}
	off := false
	c.AutoSend = &off
	if c.InvoiceDate != nil {
		if d, err := time.Parse(time.DateOnly, *c.InvoiceDate); err != nil || !d.After(s.today(a)) {
			// A next date that passed would make invoices at once.
			c.InvoiceDate = nil
		}
	}
	inv := r.invoice()
	if err := s.defaults(ctx, a, user, &inv, c.invoiceChanges()); err != nil {
		return Recurring{}, err
	}
	r.take(inv)
	if err := s.applyRecurring(ctx, a, user, &r, c, true); err != nil {
		return Recurring{}, err
	}
	keepLineIDs(r.Lines, in.Details, created)
	if in.StartDate != nil {
		r.StartDate = *in.StartDate
	}
	r.Active = false
	if err := s.saveRecurring(ctx, admin, user, &r, true); err != nil {
		return Recurring{}, err
	}
	return s.store.RecurringInvoice(ctx, admin, r.ID)
}

// MissingPayments returns the payments another system has that are not
// here: one linked to a transaction is here when a payment here is linked
// to it, or waits unlinked with its price, which linking takes over; any
// other when one here has its price and day. Each payment here stands for
// one of them only.
func MissingPayments(here []Payment, theirs []ImportedPayment) []ImportedPayment {
	used := make([]bool, len(here))
	take := func(match func(Payment) bool) bool {
		for i, p := range here {
			if !used[i] && match(p) {
				used[i] = true
				return true
			}
		}
		return false
	}
	var missing []ImportedPayment
	for _, t := range theirs {
		var price money.Amount
		if t.Input.Price != nil {
			price, _ = money.ParseAmount(*t.Input.Price)
		}
		day, _, _ := strings.Cut(strings.TrimSpace(t.Input.PaymentDate), "T")
		unlinked := func(p Payment) bool { return p.FinancialMutationID == nil && p.Price == price }
		var found bool
		if m := t.Input.FinancialMutationID; m != nil {
			found = take(func(p Payment) bool { return p.FinancialMutationID != nil && *p.FinancialMutationID == *m }) || take(unlinked)
		} else {
			found = take(func(p Payment) bool { return unlinked(p) && p.PaymentDate.Format(time.DateOnly) == day })
		}
		if !found {
			missing = append(missing, t)
		}
	}
	return missing
}
