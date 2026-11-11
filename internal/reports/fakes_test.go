package reports

import (
	"context"
	"slices"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/downloads"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/taxonomy"
)

const (
	adminID  resource.ID = 1
	owner    resource.ID = 2
	stranger resource.ID = 99

	bank        resource.ID = 10
	debtorsID   resource.ID = 11
	creditorsID resource.ID = 12
	payableVAT  resource.ID = 13
	equityID    resource.ID = 14
	inventory   resource.ID = 15
	stock       resource.ID = 16 // below inventory
	sales       resource.ID = 20
	purchases   resource.ID = 21
	rent        resource.ID = 22
	interest    resource.ID = 23

	alice  resource.ID = 100
	bob    resource.ID = 101
	webApp resource.ID = 200
)

type fakeAdmins struct{ language string }

func (f fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID || user == stranger {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{Administration: administration.Administration{
		ID: adminID, Name: "Example Shop B.V.", Currency: "EUR", Country: "NL", TimeZone: "UTC", Language: f.language,
	}}, nil
}

type fakeStore struct {
	accounts      []Account
	entries       []journal.Entry
	cash          []CashSum
	tax           []TaxSum
	open          map[string][]OpenItem
	subscriptions []SubscriptionItem
	contacts      []Contact
	gotUntil      time.Time
}

func chart() []Account {
	inv := inventory
	return []Account{
		{ID: inventory, Code: "0100", Name: "Inventory", Type: NonCurrentAssets, Active: true, Taxonomy: "BMvaTer"},
		{ID: stock, Code: "0110", Name: "Stock", Type: NonCurrentAssets, ParentID: &inv, Active: true, Taxonomy: "BMva"},
		{ID: equityID, Code: "0500", Name: "Equity", Type: Equity, Active: true, Taxonomy: "BEivKap"},
		{ID: bank, Code: "1010", Name: "Bank", Type: CurrentAssets, Active: true, Financial: true, Taxonomy: "BLimBan"},
		{ID: debtorsID, Code: "1300", Name: "Debtors", Type: CurrentAssets, Active: true, System: "accounts_receivable", Taxonomy: "BVorDeb"},
		{ID: payableVAT, Code: "1510", Name: "VAT", Type: CurrentLiabilities, Active: true, System: "payable_vat", Taxonomy: "BSchBepBtw"},
		{ID: creditorsID, Code: "1600", Name: "Creditors", Type: CurrentLiabilities, Active: true, System: "accounts_payable", Taxonomy: "BSchCre"},
		{ID: rent, Code: "4110", Name: "Rent", Type: Expenses, Active: true, Taxonomy: "WBedHui"},
		{ID: purchases, Code: "7000", Name: "Purchases", Type: DirectCosts, Active: true, Taxonomy: "WKprInhInh"},
		{ID: sales, Code: "8000", Name: "Revenue", Type: Revenue, Active: true, Taxonomy: "WOmzNodOdh"},
		{ID: interest, Code: "9000", Name: "Interest", Type: OtherIncomeExpenses, Active: true, Taxonomy: "WFbeRlm"},
	}
}

func (f *fakeStore) Accounts(context.Context, resource.ID) ([]Account, error) { return f.accounts, nil }

func (f *fakeStore) typeOf(id resource.ID) Account {
	for _, a := range f.accounts {
		if a.ID == id {
			return a
		}
	}
	return Account{}
}

func within(e journal.Entry, from *time.Time, to time.Time) bool {
	return (from == nil || !e.Date.Before(*from)) && !e.Date.After(to)
}

func (f *fakeStore) Movements(_ context.Context, _ resource.ID, from *time.Time, to time.Time, project *resource.ID) ([]Sum, error) {
	by := map[resource.ID]money.Amount{}
	var order []resource.ID
	for _, e := range f.entries {
		if !within(e, from, to) || project != nil && (e.ProjectID == nil || *e.ProjectID != *project) {
			continue
		}
		if _, ok := by[e.LedgerAccountID]; !ok {
			order = append(order, e.LedgerAccountID)
		}
		by[e.LedgerAccountID] += e.Amount
	}
	out := []Sum{}
	for _, id := range order {
		out = append(out, Sum{id, by[id]})
	}
	return out, nil
}

func (f *fakeStore) YearResults(_ context.Context, _ resource.ID, to time.Time) ([]YearResult, error) {
	by := map[int]money.Amount{}
	for _, e := range f.entries {
		if !e.Date.After(to) && !OnBalanceSheet(f.typeOf(e.LedgerAccountID).Type) {
			by[e.Date.Year()] += e.Amount
		}
	}
	out := []YearResult{}
	for y, a := range by {
		out = append(out, YearResult{y, a})
	}
	slices.SortFunc(out, func(a, b YearResult) int { return a.Year - b.Year })
	return out, nil
}

func (f *fakeStore) subjectSums(q SubjectQuery, of func(journal.Entry) *resource.ID) []SubjectSum {
	by := map[resource.ID]money.Amount{}
	var none *money.Amount
	for _, e := range f.entries {
		a := f.typeOf(e.LedgerAccountID)
		if !within(e, q.From, q.To) || !slices.Contains(q.Types, a.Type) && !slices.Contains(q.Roles, a.System) {
			continue
		}
		if q.ContactID != nil && (e.ContactID == nil || *e.ContactID != *q.ContactID) {
			continue
		}
		if id := of(e); id != nil {
			by[*id] += e.Amount
		} else {
			if none == nil {
				none = new(money.Amount)
			}
			*none += e.Amount
		}
	}
	out := []SubjectSum{}
	for id, a := range by {
		out = append(out, SubjectSum{ptr(id), a})
	}
	if none != nil {
		out = append(out, SubjectSum{nil, *none})
	}
	return out
}

func (f *fakeStore) ContactSums(_ context.Context, _ resource.ID, q SubjectQuery) ([]SubjectSum, error) {
	return f.subjectSums(q, func(e journal.Entry) *resource.ID { return e.ContactID }), nil
}

func (f *fakeStore) ProjectSums(_ context.Context, _ resource.ID, q SubjectQuery) ([]SubjectSum, error) {
	return f.subjectSums(q, func(e journal.Entry) *resource.ID { return e.ProjectID }), nil
}

func (f *fakeStore) CashFlow(context.Context, resource.ID, money.Period, *resource.ID) ([]CashSum, error) {
	return f.cash, nil
}

func (f *fakeStore) CashBalance(_ context.Context, _ resource.ID, at time.Time, _ *resource.ID) (money.Amount, error) {
	var b money.Amount
	for _, e := range f.entries {
		if !e.Date.After(at) && f.typeOf(e.LedgerAccountID).Financial {
			b += e.Amount
		}
	}
	return b, nil
}

func (f *fakeStore) TaxSums(context.Context, resource.ID, money.Period) ([]TaxSum, error) {
	return f.tax, nil
}

func (f *fakeStore) OpenItems(_ context.Context, _ resource.ID, role string, until time.Time) ([]OpenItem, error) {
	f.gotUntil = until
	return f.open[role], nil
}

func (f *fakeStore) Subscriptions(context.Context, resource.ID) ([]SubscriptionItem, error) {
	return f.subscriptions, nil
}

func (f *fakeStore) LedgerEntries(_ context.Context, _ resource.ID, ledger resource.ID, p money.Period, page, perPage int) ([]journal.Entry, int, error) {
	var out []journal.Entry
	for _, e := range f.entries {
		if e.LedgerAccountID == ledger && p.Contains(e.Date) {
			out = append(out, e)
		}
	}
	slices.Reverse(out)
	return slice(out, page, perPage), len(out), nil
}

func (f *fakeStore) Entries(_ context.Context, _ resource.ID, p money.Period) ([]journal.Entry, error) {
	var out []journal.Entry
	for _, e := range f.entries {
		if p.Contains(e.Date) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeStore) Contacts(_ context.Context, _ resource.ID, ids []resource.ID) ([]Contact, error) {
	var out []Contact
	for _, c := range f.contacts {
		if slices.Contains(ids, c.ID) {
			out = append(out, c)
		}
	}
	return out, nil
}

type added struct {
	downloadType, filename, contentType string
	data                                []byte
	failed                              bool
}

type fakeDownloads struct{ added []added }

func (f *fakeDownloads) Add(_ context.Context, _ resource.ID, _ *resource.ID, downloadType, filename, contentType string, data []byte) (downloads.Download, error) {
	f.added = append(f.added, added{downloadType, filename, contentType, data, false})
	return downloads.Download{}, nil
}

func (f *fakeDownloads) AddFailed(_ context.Context, _ resource.ID, _ *resource.ID, downloadType, filename string) (downloads.Download, error) {
	f.added = append(f.added, added{downloadType: downloadType, filename: filename, failed: true})
	return downloads.Download{}, nil
}

type fakeSchemes struct{ none bool }

func (f fakeSchemes) Scheme(context.Context, resource.ID, resource.ID) (*taxonomy.Scheme, error) {
	if f.none {
		return nil, nil
	}
	s, _ := taxonomy.Lookup("nl-rgs-3.8")
	return s, nil
}

// fixture is a service on an administration whose today is 2025-06-15.
type fixture struct {
	*Service
	store     *fakeStore
	downloads *fakeDownloads
}

func newFixture() fixture {
	store := &fakeStore{accounts: chart(), open: map[string][]OpenItem{}}
	files := &fakeDownloads{}
	now := func() time.Time { return time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC) }
	return fixture{Service: NewService(fakeAdmins{language: "nl"}, store, files, fakeSchemes{}, now), store: store, downloads: files}
}

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

var entrySeq resource.ID = 1000

// post adds a balanced document: pairs of category and amount.
func (f fixture) post(documentType, date string, contact, project *resource.ID, lines ...any) {
	doc := entrySeq
	entrySeq++
	for i := 0; i < len(lines); i += 2 {
		entrySeq++
		e := journal.Entry{
			ID: entrySeq, AdministrationID: adminID, DocumentType: documentType, DocumentID: doc, Date: day(date),
			LedgerAccountID: lines[i].(resource.ID), Amount: money.Amount(lines[i+1].(int)), ContactID: contact, Description: "Booking", RowOrder: i / 2,
		}
		if f.store.typeOf(e.LedgerAccountID).Type == Revenue || f.store.typeOf(e.LedgerAccountID).Type == DirectCosts {
			e.ProjectID = project
		}
		f.store.entries = append(f.store.entries, e)
	}
}
