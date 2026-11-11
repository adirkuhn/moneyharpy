package products

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID   = resource.ID(1)
	owner     = resource.ID(2)
	vat21     = resource.ID(10)
	buyVAT    = resource.ID(11)
	revenue   = resource.ID(20)
	expenses  = resource.ID(21)
	customer  = resource.ID(30)
	otherUser = resource.ID(99)
)

var now = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID || user != owner {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{Administration: administration.Administration{ID: id, Currency: "EUR"}}, nil
}

type fakeLedger struct{}

func (fakeLedger) TaxRate(_ context.Context, _, id, _ resource.ID) (bookkeeping.TaxRate, error) {
	switch id {
	case vat21:
		return bookkeeping.TaxRate{ID: id, Type: "sales_invoice", Active: true, Percentage: "21"}, nil
	case buyVAT:
		return bookkeeping.TaxRate{ID: id, Type: "purchase_invoice", Active: true}, nil
	}
	return bookkeeping.TaxRate{}, bookkeeping.ErrTaxRateNotFound
}

func (fakeLedger) Bookable(_ context.Context, _, _, id resource.ID, _ string) (string, error) {
	switch id {
	case revenue:
		return "", nil
	case expenses:
		return "is not a revenue category", nil
	}
	return "", bookkeeping.ErrLedgerAccountNotFound
}

type fakeContacts struct{}

func (fakeContacts) Contact(_ context.Context, _, id, _ resource.ID) (contacts.Contact, error) {
	if id != customer {
		return contacts.Contact{}, contacts.ErrNotFound
	}
	return contacts.Contact{ID: id}, nil
}

type memStore struct{ products map[resource.ID]Product }

func (m *memStore) Products(_ context.Context, _ resource.ID, q Query) ([]Product, int, error) {
	var out []Product
	for _, p := range m.products {
		if q.Matches(p) {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b Product) int { return strings.Compare(a.Title+a.Description, b.Title+b.Description) })
	total := len(out)
	if q.PerPage > 0 {
		from := min((q.Page-1)*q.PerPage, total)
		out = out[from:min(from+q.PerPage, total)]
	}
	return out, total, nil
}

func (m *memStore) Product(_ context.Context, _, id resource.ID) (Product, error) {
	p, ok := m.products[id]
	if !ok {
		return Product{}, ErrNotFound
	}
	return p, nil
}

func (m *memStore) ProductByIdentifier(_ context.Context, _ resource.ID, identifier string) (Product, error) {
	for _, p := range m.products {
		if p.Identifier == identifier {
			return p, nil
		}
	}
	return Product{}, ErrNotFound
}

func (m *memStore) SaveProduct(_ context.Context, p Product, _ bool) error {
	m.products[p.ID] = p
	return nil
}

func (m *memStore) DeleteProduct(_ context.Context, _, id resource.ID) error {
	delete(m.products, id)
	return nil
}

type fakeReferences map[resource.ID]bool

func (f fakeReferences) ProductInUse(_ context.Context, _, id resource.ID) (bool, error) {
	return f[id], nil
}

func newService() (*Service, *memStore) {
	store := &memStore{products: map[resource.ID]Product{}}
	return NewService(fakeAdmins{}, fakeLedger{}, fakeContacts{}, store, nil, resource.NewGenerator(1), func() time.Time { return now }), store
}

func ptr[T any](v T) *T { return &v }

func fieldsOf(t *testing.T, err error) map[string][]string {
	t.Helper()
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want a validation error, got %v", err)
	}
	return verr.Fields
}

func create(t *testing.T, s *Service, c Changes) Product {
	t.Helper()
	if c.Price == nil {
		c.Price = ptr("50,50")
	}
	if c.LedgerAccountID == nil {
		c.LedgerAccountID = ptr(revenue)
	}
	p, err := s.Create(context.Background(), adminID, owner, c)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCreateTakesDefaultsAndValidates(t *testing.T) {
	s, _ := newService()
	ctx := context.Background()
	p := create(t, s, Changes{Description: ptr("Geldvogel"), TaxRateID: ptr(vat21), Identifier: ptr("GV-1"), FrequencyPreset: ptr("3:month")})
	if p.Price != 5050 || p.Currency != "EUR" || !p.Active || *p.TaxRateID != vat21 || p.Frequency != 3 || p.FrequencyType != "month" || p.LineDescription() != "Geldvogel" {
		t.Errorf("created = %+v", p)
	}
	if p.Ref().URI() != "/1/products/"+p.ID.String() {
		t.Errorf("uri = %s", p.Ref().URI())
	}
	_, err := s.Create(ctx, adminID, owner, Changes{})
	if f := fieldsOf(t, err); f["price"] == nil || f["ledger_account_id"] == nil || f["base"] == nil || f["title"] == nil {
		t.Errorf("empty product = %v", f)
	}
	for field, c := range map[string]Changes{
		"identifier":        {Title: ptr("Copy"), Identifier: ptr("GV-1")},
		"tax_rate_id":       {Title: ptr("Bought"), TaxRateID: ptr(buyVAT)},
		"ledger_account_id": {Title: ptr("Cost"), LedgerAccountID: ptr(expenses)},
		"frequency_type":    {Title: ptr("Often"), FrequencyType: ptr("fortnight")},
		"frequency_preset":  {Title: ptr("Odd"), FrequencyPreset: ptr("monthly")},
		"product_type":      {Title: ptr("Kind"), ProductType: ptr("thing")},
		"currency":          {Title: ptr("Money"), Currency: ptr("euro")},
		"price":             {Title: ptr("Cheap"), Price: ptr("free")},
	} {
		if c.Price == nil {
			c.Price = ptr("1")
		}
		if c.LedgerAccountID == nil {
			c.LedgerAccountID = ptr(revenue)
		}
		if _, err := s.Create(ctx, adminID, owner, c); fieldsOf(t, err)[field] == nil {
			t.Errorf("%s: accepted %+v", field, c)
		}
	}
	if _, err := s.Create(ctx, adminID, otherUser, Changes{}); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("other user = %v", err)
	}
}

func TestUpdateFrequencyAndActive(t *testing.T) {
	s, _ := newService()
	ctx := context.Background()
	p := create(t, s, Changes{Title: ptr("Hosting"), FrequencyType: ptr("year")})
	if p.Frequency != 1 {
		t.Errorf("frequency = %d", p.Frequency)
	}
	up, err := s.Update(ctx, adminID, p.ID, owner, Changes{FrequencyPreset: ptr("0:nil"), Price: ptr("12.5"), Active: ptr(false), TaxRateID: ptr(resource.ID(0))})
	if err != nil || up.Frequency != 0 || up.FrequencyType != "" || up.Price != 1250 || up.Active || up.TaxRateID != nil || !up.UpdatedAt.Equal(now) {
		t.Errorf("updated = %+v, %v", up, err)
	}
	if same, err := s.Update(ctx, adminID, p.ID, owner, Changes{FrequencyPreset: ptr("1:custom")}); err != nil || same.FrequencyType != "" {
		t.Errorf("custom preset = %+v, %v", same, err)
	}
	if _, err := s.Update(ctx, adminID, 12345, owner, Changes{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown = %v", err)
	}
}

func TestListLooksUpAndDeletes(t *testing.T) {
	s, store := newService()
	ctx := context.Background()
	a := create(t, s, Changes{Title: ptr("Apple"), Identifier: ptr("A.1")})
	b := create(t, s, Changes{Description: ptr("Banana box"), Currency: ptr("usd")})
	if list, total, err := s.List(ctx, adminID, owner, Query{Search: "BOX"}); err != nil || total != 1 || list[0].ID != b.ID {
		t.Errorf("search = %+v, %v", list, err)
	}
	if list, _, _ := s.List(ctx, adminID, owner, Query{Currency: "USD"}); len(list) != 1 || list[0].Currency != "USD" {
		t.Errorf("by currency = %+v", list)
	}
	if list, _, _ := s.List(ctx, adminID, owner, Query{ForCheckout: ptr(true)}); len(list) != 0 {
		t.Errorf("for checkout = %+v", list)
	}
	if list, total, _ := s.List(ctx, adminID, owner, Query{PerPage: 1, Page: 2}); total != 2 || len(list) != 1 {
		t.Errorf("page 2 = %+v of %d", list, total)
	}
	if got, err := s.ByIdentifier(ctx, adminID, owner, "A.1"); err != nil || got.ID != a.ID {
		t.Errorf("by identifier = %+v, %v", got, err)
	}
	if _, err := s.ByIdentifier(ctx, adminID, owner, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("blank identifier = %v", err)
	}

	s.references = fakeReferences{a.ID: true}
	if kept, err := s.Delete(ctx, adminID, a.ID, owner); err != nil || !kept || store.products[a.ID].Active {
		t.Errorf("delete used = %v, %v", kept, err)
	}
	if list, _, _ := s.List(ctx, adminID, owner, Query{}); len(list) != 1 || list[0].ID != b.ID {
		t.Errorf("active after deactivating = %+v", list)
	}
	if list, _, _ := s.List(ctx, adminID, owner, Query{IncludeInactive: true}); len(list) != 2 {
		t.Errorf("with inactive = %+v", list)
	}
	if kept, err := s.Delete(ctx, adminID, b.ID, owner); err != nil || kept {
		t.Errorf("delete free = %v, %v", kept, err)
	}
	if _, ok := store.products[b.ID]; ok {
		t.Error("free product still stored")
	}
}

func TestSalesLinkIsNotAvailable(t *testing.T) {
	s, _ := newService()
	ctx := context.Background()
	p := create(t, s, Changes{Title: ptr("Apple")})
	if _, err := s.SalesLink(ctx, adminID, p.ID, owner, ptr(customer)); !errors.Is(err, ErrNotForOnlineSales) {
		t.Errorf("sales link = %v", err)
	}
	if _, err := s.SalesLink(ctx, adminID, p.ID, owner, ptr(resource.ID(5))); !errors.Is(err, contacts.ErrNotFound) {
		t.Errorf("unknown contact = %v", err)
	}
	if _, err := s.SalesLink(ctx, adminID, 5, owner, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown product = %v", err)
	}
}
