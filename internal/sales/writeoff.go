package sales

import (
	"context"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// BookMethods are the ways an uncollectible invoice is written off;
// Moneybird's API knows only revenue.
var BookMethods = []string{"revenue"}

// MarkDubious marks an invoice that may not be paid: its open amount
// excluding VAT goes into the provision for doubtful debtors. The state
// stays as it is.
func (s *Service) MarkDubious(ctx context.Context, admin, id, user resource.ID, date string) (Invoice, error) {
	if err := s.markDubious(ctx, admin, user, EntityType, id, date); err != nil {
		return Invoice{}, err
	}
	return s.Invoice(ctx, admin, id, user)
}

// MarkExternalDubious marks an external invoice dubious, as MarkDubious
// does.
func (s *Service) MarkExternalDubious(ctx context.Context, admin, id, user resource.ID, date string) (ExternalInvoice, error) {
	if err := s.markDubious(ctx, admin, user, ExternalEntityType, id, date); err != nil {
		return ExternalInvoice{}, err
	}
	return s.ExternalInvoice(ctx, admin, id, user)
}

func (s *Service) markDubious(ctx context.Context, admin, user resource.ID, kind string, id resource.ID, date string) error {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return err
	}
	inv, err := s.loadInvoice(ctx, admin, kind, id)
	if err != nil {
		return err
	}
	if !payable(inv) || inv.MarkedDubiousOn != nil {
		return ErrNotEditable
	}
	day, err := s.writeOffDay(ctx, a, user, inv, date, "dubious_date", nil)
	if err != nil {
		return err
	}
	inv.MarkedDubiousOn, inv.UpdatedAt = &day, s.now()
	if err := s.repost(ctx, admin, user, &inv); err != nil {
		return err
	}
	return s.record(ctx, user, on(inv, inv.events()+"_marked_as_dubious"))
}

// MarkUncollectible writes off what is still open of an invoice, for good:
// excluding VAT on uncollectible revenue (or the provision, when the
// invoice was dubious) and the VAT back off payable VAT.
func (s *Service) MarkUncollectible(ctx context.Context, admin, id, user resource.ID, date, bookMethod string) (Invoice, error) {
	if err := s.markUncollectible(ctx, admin, user, EntityType, id, date, bookMethod); err != nil {
		return Invoice{}, err
	}
	return s.Invoice(ctx, admin, id, user)
}

// MarkExternalUncollectible writes off an external invoice, as
// MarkUncollectible does.
func (s *Service) MarkExternalUncollectible(ctx context.Context, admin, id, user resource.ID, date, bookMethod string) (ExternalInvoice, error) {
	if err := s.markUncollectible(ctx, admin, user, ExternalEntityType, id, date, bookMethod); err != nil {
		return ExternalInvoice{}, err
	}
	return s.ExternalInvoice(ctx, admin, id, user)
}

func (s *Service) markUncollectible(ctx context.Context, admin, user resource.ID, kind string, id resource.ID, date, bookMethod string) error {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return err
	}
	if bookMethod = strings.TrimSpace(bookMethod); bookMethod != "" && bookMethod != "revenue" {
		return invalid("book_method", "is not included in the list")
	}
	inv, err := s.loadInvoice(ctx, admin, kind, id)
	if err != nil {
		return err
	}
	if !payable(inv) {
		return ErrNotEditable
	}
	day, err := s.writeOffDay(ctx, a, user, inv, date, "uncollectible_date", inv.MarkedDubiousOn)
	if err != nil {
		return err
	}
	inv.MarkedUncollectibleOn, inv.State, inv.UpdatedAt = &day, Uncollectible, s.now()
	if err := s.repost(ctx, admin, user, &inv); err != nil {
		return err
	}
	s.notify(ctx, admin, inv.events()+"_state_changed_to_uncollectible", inv)
	return s.record(ctx, user, stateChanged(inv, Uncollectible), on(inv, inv.events()+"_marked_as_uncollectible"))
}

// repost stores an invoice of either kind with what it posts again.
func (s *Service) repost(ctx context.Context, admin, user resource.ID, inv *Invoice) error {
	entries, err := s.post(ctx, admin, user, *inv)
	if err != nil {
		return err
	}
	if !inv.external() {
		return s.save(ctx, admin, user, inv, entries, false)
	}
	return wrap("store invoice", s.store.SavePayments(ctx, PaymentChange{Invoices: []Posting{{Invoice: *inv, Entries: entries}}}))
}

// writeOffDay reads the date of a write-off: today when empty, after the
// invoice date (and after, when given), not in the future or the locked
// period.
func (s *Service) writeOffDay(ctx context.Context, a administration.Listed, user resource.ID, inv Invoice, date, field string, after *time.Time) (time.Time, error) {
	day := s.today(a)
	if v := strings.TrimSpace(date); v != "" {
		d, err := time.Parse(time.DateOnly, v)
		if err != nil {
			return day, invalid(field, "is not a date")
		}
		day = d
	}
	switch {
	case inv.InvoiceDate != nil && !day.After(*inv.InvoiceDate):
		return day, invalid(field, "must be after the invoice date")
	case after != nil && !day.After(*after):
		return day, invalid(field, "must be after the date the invoice was marked dubious")
	case day.After(s.today(a)):
		return day, invalid(field, "can't be in the future")
	}
	return day, s.checkDay(ctx, a.ID, user, day, field)
}

// split divides an amount including VAT over the invoice's VAT rates, in
// the proportion of their VAT to the total.
func split(amount money.Amount, tot Totals) (money.Amount, []RateTotal) {
	if tot.InclTax == 0 {
		return amount, nil
	}
	excl := amount
	var taxes []RateTotal
	for _, r := range tot.ByRate {
		tax := money.Amount(divRound(int64(r.Tax)*int64(amount), int64(tot.InclTax)))
		if tax != 0 {
			taxes = append(taxes, RateTotal{TaxRateID: r.TaxRateID, Tax: tax})
			excl -= tax
		}
	}
	return excl, taxes
}

// writeOffs are what marking an invoice dubious or uncollectible posts,
// with the invoice. entry makes an entry of the invoice.
func (s *Service) writeOffs(ctx context.Context, admin, user resource.ID, inv Invoice, tot Totals,
	entry func(ledger resource.ID, amount money.Amount, description string, rate *resource.ID) journal.Entry) ([]journal.Entry, error) {
	if inv.MarkedDubiousOn == nil && inv.MarkedUncollectibleOn == nil {
		return nil, nil
	}
	accounts := map[string]resource.ID{}
	for _, role := range []string{"doubtful_debtors_allocation", "doubtful_debtors_provision", "uncollectible_revenue", "payable_vat", "accounts_receivable"} {
		id, err := s.ledger.SystemLedgerAccount(ctx, admin, user, role)
		if err != nil {
			return nil, wrap("find category "+role, err)
		}
		accounts[role] = id
	}
	number := cmpString(inv.InvoiceID, inv.Reference)
	if inv.external() {
		number = inv.label()
	}
	var out []journal.Entry
	add := func(day time.Time, role string, amount money.Amount, rate *resource.ID) {
		if amount == 0 {
			return
		}
		e := entry(accounts[role], amount, number, rate)
		e.Date = day
		out = append(out, e)
	}
	var provision money.Amount
	if d := inv.MarkedDubiousOn; d != nil {
		open := tot.InclTax
		for _, p := range inv.Payments {
			if !p.PaymentDate.After(*d) {
				open -= p.Price
			}
		}
		provision, _ = split(open, tot)
		add(*d, "doubtful_debtors_allocation", provision, nil)
		add(*d, "doubtful_debtors_provision", -provision, nil)
		if inv.State == Paid && inv.PaidAt != nil {
			// The payment came after all: the provision is released.
			add(*inv.PaidAt, "doubtful_debtors_provision", provision, nil)
			add(*inv.PaidAt, "doubtful_debtors_allocation", -provision, nil)
		}
	}
	if u := inv.MarkedUncollectibleOn; u != nil {
		open := tot.InclTax - inv.Paid()
		excl, taxes := split(open, tot)
		if inv.MarkedDubiousOn != nil {
			add(*u, "doubtful_debtors_provision", provision, nil)
			add(*u, "doubtful_debtors_allocation", excl-provision, nil)
		} else {
			add(*u, "uncollectible_revenue", excl, nil)
		}
		for _, t := range taxes {
			rate := t.TaxRateID
			add(*u, "payable_vat", t.Tax, &rate)
		}
		add(*u, "accounts_receivable", -open, nil)
	}
	return out, nil
}
