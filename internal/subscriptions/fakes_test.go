package subscriptions

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

const (
	adminID  resource.ID = 1
	owner    resource.ID = 2
	customer resource.ID = 3
	monthly  resource.ID = 10
	premium  resource.ID = 11
	oneOff   resource.ID = 12
	vat21    resource.ID = 20
	stranger resource.ID = 99
)

func ptrTo[T any](v T) *T { return &v }

func day(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID || user != owner {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{Administration: administration.Administration{ID: adminID, TimeZone: "UTC"}}, nil
}

type fakeContacts struct{}

func (fakeContacts) Contact(_ context.Context, _, id, _ resource.ID) (contacts.Contact, error) {
	if id != customer {
		return contacts.Contact{}, contacts.ErrNotFound
	}
	return contacts.Contact{ID: customer}, nil
}

type fakeProducts struct{}

func (fakeProducts) Product(_ context.Context, _, id, _ resource.ID) (products.Product, error) {
	rate := vat21
	switch id {
	case monthly:
		return products.Product{ID: monthly, Description: "Hosting", Price: 2000, Frequency: 1, FrequencyType: "month", TaxRateID: &rate, Active: true}, nil
	case premium:
		return products.Product{ID: premium, Description: "Premium hosting", Price: 3000, Frequency: 1, FrequencyType: "month", TaxRateID: &rate, Active: true}, nil
	case oneOff:
		return products.Product{ID: oneOff, Description: "Setup", Price: 5000, TaxRateID: &rate, Active: true}, nil
	}
	return products.Product{}, products.ErrNotFound
}

// fakeSales keeps recurring invoices and invoices in memory, applying only
// what subscriptions use.
type fakeSales struct {
	ids       *resource.Generator
	recurring map[resource.ID]sales.Recurring
	invoices  map[resource.ID]sales.Invoice
	deleted   []resource.ID
	sent      []resource.ID
}

func newFakeSales(ids *resource.Generator) *fakeSales {
	return &fakeSales{ids: ids, recurring: map[resource.ID]sales.Recurring{}, invoices: map[resource.ID]sales.Invoice{}}
}

func (f *fakeSales) lines(existing []sales.Detail, in []sales.DetailInput) []sales.Detail {
	out := slices.Clone(existing)
	for _, d := range in {
		var line *sales.Detail
		if d.ID != nil {
			i := slices.IndexFunc(out, func(x sales.Detail) bool { return x.ID == *d.ID })
			line = &out[i]
		} else {
			out = append(out, sales.Detail{ID: f.ids.Next(), Amount: "1", Quantity: 1000, RowOrder: len(out) + 1})
			line = &out[len(out)-1]
		}
		if d.ProductID != nil {
			line.ProductID = d.ProductID
		}
		if d.Description != nil {
			line.Description = *d.Description
		}
		if d.Price != nil {
			line.Price, _ = money.ParseAmount(*d.Price)
		}
		if d.Amount != nil {
			line.Amount = *d.Amount
			line.Quantity, _ = sales.ParseQuantity(*d.Amount)
		}
		if d.Period != nil {
			line.Period = *d.Period
		}
		if d.RowOrder != nil {
			line.RowOrder = *d.RowOrder
		}
	}
	return out
}

func (f *fakeSales) applyRecurring(r *sales.Recurring, c sales.RecurringChanges) {
	if c.ContactID != nil {
		r.ContactID = *c.ContactID
	}
	if c.Reference != nil {
		r.Reference = *c.Reference
	}
	if c.Discount != nil {
		r.Discount, _ = sales.ParsePercentage(*c.Discount)
	}
	if c.InvoiceDate != nil {
		r.InvoiceDate, r.Anchor, r.Periods = day(*c.InvoiceDate), day(*c.InvoiceDate), 0
	}
	if c.FrequencyType != nil {
		r.FrequencyType = *c.FrequencyType
	}
	if c.Frequency != nil {
		r.Frequency = *c.Frequency
	}
	if c.AutoSend != nil {
		r.AutoSend = *c.AutoSend
	}
	if c.LastDate != nil {
		d := day(*c.LastDate)
		r.LastDate = &d
	}
	r.Lines = f.lines(r.Lines, c.Details)
}

func (f *fakeSales) CreateRecurring(_ context.Context, admin, _ resource.ID, c sales.RecurringChanges) (sales.Recurring, error) {
	r := sales.Recurring{ID: f.ids.Next(), AdministrationID: admin, Active: true, FrequencyType: "month", Frequency: 1}
	f.applyRecurring(&r, c)
	r.StartDate = r.InvoiceDate
	f.recurring[r.ID] = r
	return r, nil
}

func (f *fakeSales) UpdateRecurring(_ context.Context, _, id, _ resource.ID, c sales.RecurringChanges) (sales.Recurring, error) {
	r := f.recurring[id]
	f.applyRecurring(&r, c)
	f.recurring[id] = r
	return r, nil
}

func (f *fakeSales) Recurring(_ context.Context, _, id, _ resource.ID) (sales.Recurring, error) {
	r, ok := f.recurring[id]
	if !ok {
		return r, sales.ErrNotFound
	}
	return r, nil
}

func (f *fakeSales) DeactivateRecurring(_ context.Context, _, id, _ resource.ID) (sales.Recurring, error) {
	r := f.recurring[id]
	r.Active = false
	f.recurring[id] = r
	return r, nil
}

func (f *fakeSales) DeleteRecurring(_ context.Context, _, id, _ resource.ID) (bool, error) {
	delete(f.recurring, id)
	return false, nil
}

// generate makes the next invoice of a recurring invoice, as the sales
// worker does.
func (f *fakeSales) generate(id resource.ID) sales.Invoice {
	r := f.recurring[id]
	date := r.InvoiceDate
	inv := sales.Invoice{ID: f.ids.Next(), AdministrationID: r.AdministrationID, ContactID: r.ContactID, State: sales.Draft, InvoiceDate: &date, Discount: r.Discount, RecurringSalesInvoiceID: &r.ID}
	for _, d := range r.Lines {
		d.ID = f.ids.Next()
		inv.Lines = append(inv.Lines, d)
	}
	f.invoices[inv.ID] = inv
	r.Periods++
	r.Generated++
	r.InvoiceDate = sales.Occurrence(r.Anchor, r.FrequencyType, r.Frequency, r.Periods)
	f.recurring[id] = r
	return inv
}

func (f *fakeSales) Create(_ context.Context, admin, _ resource.ID, c sales.Changes) (sales.Invoice, error) {
	inv := sales.Invoice{ID: f.ids.Next(), AdministrationID: admin, ContactID: *c.ContactID, State: sales.Draft}
	if c.InvoiceDate != nil {
		d := day(*c.InvoiceDate)
		inv.InvoiceDate = &d
	}
	inv.Lines = f.lines(nil, c.Details)
	f.invoices[inv.ID] = inv
	return inv, nil
}

func (f *fakeSales) Update(_ context.Context, _, id, _ resource.ID, c sales.Changes) (sales.Invoice, error) {
	inv := f.invoices[id]
	inv.Lines = f.lines(inv.Lines, c.Details)
	if c.Discount != nil {
		inv.Discount, _ = sales.ParsePercentage(*c.Discount)
	}
	f.invoices[id] = inv
	return inv, nil
}

func (f *fakeSales) Send(_ context.Context, _, id, _ resource.ID, in sales.SendInput) (sales.Invoice, error) {
	inv := f.invoices[id]
	inv.State = sales.Open
	if in.Scheduled {
		inv.State = sales.Scheduled
	}
	f.invoices[id] = inv
	f.sent = append(f.sent, id)
	return inv, nil
}

func (f *fakeSales) Invoice(_ context.Context, _, id, _ resource.ID) (sales.Invoice, error) {
	inv, ok := f.invoices[id]
	if !ok {
		return inv, sales.ErrNotFound
	}
	return inv, nil
}

func (f *fakeSales) Delete(_ context.Context, _, id, _ resource.ID) error {
	delete(f.invoices, id)
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeStore struct {
	subs    map[resource.ID]Subscription
	charges []Charge
	oneOffs map[resource.ID][]resource.ID
}

func newFakeStore() *fakeStore {
	return &fakeStore{subs: map[resource.ID]Subscription{}, oneOffs: map[resource.ID][]resource.ID{}}
}

func (s *fakeStore) Subscriptions(_ context.Context, _, contact resource.ID) ([]Subscription, error) {
	var out []Subscription
	for _, sub := range s.subs {
		if sub.ContactID == contact {
			out = append(out, sub)
		}
	}
	slices.SortFunc(out, func(a, b Subscription) int { return int(a.ID - b.ID) })
	return out, nil
}

func (s *fakeStore) Subscription(_ context.Context, _, id resource.ID) (Subscription, error) {
	sub, ok := s.subs[id]
	if !ok {
		return sub, ErrNotFound
	}
	return sub, nil
}

func (s *fakeStore) SubscriptionByRecurring(_ context.Context, _, recurring resource.ID) (Subscription, error) {
	for _, sub := range s.subs {
		if sub.RecurringID != nil && *sub.RecurringID == recurring {
			return sub, nil
		}
	}
	return Subscription{}, ErrNotFound
}

func (s *fakeStore) SaveSubscription(_ context.Context, sub Subscription, _ bool) error {
	sub.Items = slices.Clone(sub.Items)
	s.subs[sub.ID] = sub
	return nil
}

func (s *fakeStore) Charges(_ context.Context, _ resource.ID, q ChargeQuery) ([]Charge, error) {
	var out []Charge
	for _, c := range s.charges {
		switch {
		case q.ContactID != nil && (c.ContactID == nil || *c.ContactID != *q.ContactID),
			q.SubscriptionID != nil && (c.SubscriptionID == nil || *c.SubscriptionID != *q.SubscriptionID),
			!q.IncludeBilled && c.Billed():
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func (s *fakeStore) SaveCharge(_ context.Context, c Charge) error {
	s.charges = append(s.charges, c)
	return nil
}

func (s *fakeStore) BillCharges(_ context.Context, _ resource.ID, billed []Charge) error {
	for _, b := range billed {
		for i := range s.charges {
			if s.charges[i].ID == b.ID {
				s.charges[i].DetailID, s.charges[i].InvoiceID = b.DetailID, b.InvoiceID
			}
		}
	}
	return nil
}

func (s *fakeStore) UnbilledContactCharges(_ context.Context, _ resource.ID) ([]Charge, error) {
	var out []Charge
	for _, c := range s.charges {
		if c.ContactID != nil && !c.Billed() {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *fakeStore) ChargeWorkload(context.Context) ([]Workload, error) {
	return []Workload{{AdministrationID: adminID, UserID: owner}}, nil
}

func (s *fakeStore) Lock(context.Context, resource.ID) (func(), bool, error) {
	return func() {}, true, nil
}

func (s *fakeStore) OneOffInvoices(_ context.Context, _, subscription resource.ID) ([]resource.ID, error) {
	return s.oneOffs[subscription], nil
}

func (s *fakeStore) AddOneOffInvoice(_ context.Context, _, subscription, invoice resource.ID) error {
	s.oneOffs[subscription] = append(s.oneOffs[subscription], invoice)
	return nil
}

type fakeNotifier struct{ actions []string }

func (n *fakeNotifier) Notify(_ context.Context, _ resource.ID, action string, _ sales.Invoice) {
	n.actions = append(n.actions, action)
}

type harness struct {
	svc      *Service
	sales    *fakeSales
	store    *fakeStore
	notifier *fakeNotifier
	clock    *time.Time
}

func newHarness(t *testing.T) harness {
	t.Helper()
	clock := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	ids := resource.NewGenerator(1)
	h := harness{sales: newFakeSales(ids), store: newFakeStore(), notifier: &fakeNotifier{}, clock: &clock}
	h.svc = NewService(Deps{
		Administrations: fakeAdmins{}, Contacts: fakeContacts{}, Products: fakeProducts{}, Sales: h.sales, Store: h.store,
		Notifier: h.notifier, IDs: ids, Now: func() time.Time { return *h.clock },
	})
	return h
}
