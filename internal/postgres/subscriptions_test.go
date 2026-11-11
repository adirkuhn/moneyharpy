package postgres_test

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/downloads"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
	"github.com/adirkuhn/moneyharpy/internal/subscriptions"
)

func TestSubscriptionsRoundTrip(t *testing.T) {
	ctx := context.Background()
	f := newSalesFixture(t)
	store := postgres.NewSubscriptionStore(testPool)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	svc := subscriptions.NewService(subscriptions.Deps{
		Administrations: admins, Contacts: f.contacts, Products: f.products, Sales: f.svc, Store: store, IDs: ids, Now: time.Now,
	})
	p, err := f.products.Create(ctx, f.admin, f.owner, products.Changes{
		Description: ptr("Hosting"), Price: ptr("20"), TaxRateID: &f.vat21, LedgerAccountID: &f.revenue, FrequencyPreset: ptr("1:month"),
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().AddDate(0, 0, 3).Format(time.DateOnly)
	sub, err := svc.Create(ctx, f.admin, f.owner, subscriptions.Input{
		ContactID: &f.customer, ProductID: &p.ID, StartDate: &start, Amount: ptr("2"), Discount: ptr("10"), Reference: ptr("Contract 7"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Subscription(ctx, f.admin, sub.ID, f.owner)
	if err != nil || got.Reference != "Contract 7" || got.StartDate.Format(time.DateOnly) != start || len(got.Items) != 1 ||
		got.Items[0].Amount != "2 x" || got.Items[0].Discount != 1000 || got.RecurringID == nil || got.FrequencyType != "month" {
		t.Fatalf("Subscription() = %+v, %v", got, err)
	}
	r, err := f.svc.Recurring(ctx, f.admin, *got.RecurringID, f.owner)
	if err != nil || r.Totals.ExclTax != 3600 || !r.AutoSend {
		t.Errorf("recurring = %+v, %v", r, err)
	}
	if byRec, err := store.SubscriptionByRecurring(ctx, f.admin, r.ID); err != nil || byRec.ID != sub.ID {
		t.Errorf("by recurring = %+v, %v", byRec, err)
	}
	if list, err := svc.List(ctx, f.admin, f.owner, f.customer); err != nil || len(list) != 1 {
		t.Errorf("List() = %+v, %v", list, err)
	}
	if _, err := store.Subscription(ctx, f.admin, 1); !errors.Is(err, subscriptions.ErrNotFound) {
		t.Errorf("missing = %v", err)
	}

	later := time.Now().AddDate(0, 2, 0).Format(time.DateOnly)
	if got, err = svc.Update(ctx, f.admin, sub.ID, f.owner, subscriptions.Changes{Amount: ptr("3"), StartDate: &later}); err != nil || len(got.Items) != 2 {
		t.Fatalf("Update() = %+v, %v", got, err)
	}
	if stored, _ := store.Subscription(ctx, f.admin, sub.ID); len(stored.Items) != 2 || stored.Items[0].EndDate == nil || stored.Items[1].Amount != "3 x" {
		t.Errorf("items = %+v", stored.Items)
	}
	if inUse, err := store.ProductInUse(ctx, f.admin, p.ID); err != nil || !inUse {
		t.Errorf("ProductInUse() = %v, %v", inUse, err)
	}

	c, err := svc.AddCharge(ctx, f.admin, f.owner, subscriptions.Owner{ContactID: &f.customer}, subscriptions.ChargeInput{
		ProductID: p.ID, Amount: "5 x", Period: "20210601..20210630", Description: "Transaction Costs", Identifier: ptr("tx-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	charges, err := svc.Charges(ctx, f.admin, f.owner, subscriptions.Owner{ContactID: &f.customer}, false)
	if err != nil || len(charges) != 1 || charges[0].Price != 2000 || charges[0].Period.Monthly() != "202106..202106" || charges[0].Identifier != "tx-1" {
		t.Errorf("charges = %+v, %v", charges, err)
	}
	if work, err := store.ChargeWorkload(ctx); err != nil || !slices.ContainsFunc(work, func(w subscriptions.Workload) bool { return w.AdministrationID == f.admin && w.UserID == f.owner }) {
		t.Errorf("workload = %+v, %v", work, err)
	}
	inv := f.invoice(t, "10", time.Now().Format(time.DateOnly))
	c.DetailID, c.InvoiceID = &inv.Lines[0].ID, &inv.ID
	if err := store.BillCharges(ctx, f.admin, []subscriptions.Charge{c}); err != nil {
		t.Fatal(err)
	}
	if open, _ := store.UnbilledContactCharges(ctx, f.admin); len(open) != 0 {
		t.Errorf("unbilled = %+v", open)
	}
	if all, _ := store.Charges(ctx, f.admin, subscriptions.ChargeQuery{ContactID: &f.customer, IncludeBilled: true}); len(all) != 1 || *all[0].DetailID != inv.Lines[0].ID {
		t.Errorf("billed = %+v", all)
	}

	draft, err := f.svc.Create(ctx, f.admin, f.owner, sales.Changes{ContactID: &f.customer})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddOneOffInvoice(ctx, f.admin, sub.ID, draft.ID); err != nil {
		t.Fatal(err)
	}
	if list, err := store.OneOffInvoices(ctx, f.admin, sub.ID); err != nil || len(list) != 1 || list[0] != draft.ID {
		t.Errorf("one-offs = %v, %v", list, err)
	}
	if err := f.svc.Delete(ctx, f.admin, draft.ID, f.owner); err != nil {
		t.Fatal(err)
	}
	if list, _ := store.OneOffInvoices(ctx, f.admin, sub.ID); len(list) != 0 {
		t.Errorf("one-offs after delete = %v", list)
	}

	release, ok, err := store.Lock(ctx, f.admin)
	if err != nil || !ok {
		t.Fatalf("Lock() = %v, %v", ok, err)
	}
	if _, again, _ := store.Lock(ctx, f.admin); again {
		t.Error("locked twice")
	}
	release()

	cancelled, err := svc.Cancel(ctx, f.admin, sub.ID, f.owner, nil)
	if err != nil || cancelled.CancelledAt == nil {
		t.Fatalf("Cancel() = %+v, %v", cancelled, err)
	}
	if stored, _ := store.Subscription(ctx, f.admin, sub.ID); stored.CancelledAt == nil || stored.EndDate == nil || len(stored.Items) != 1 {
		t.Errorf("cancelled = %+v", stored)
	}
	if r, _ := f.svc.Recurring(ctx, f.admin, r.ID, f.owner); r.Active {
		t.Error("the recurring invoice still runs")
	}
}

type memoryFiles map[string][]byte

func (m memoryFiles) Put(_ context.Context, key, _ string, data []byte) error {
	m[key] = data
	return nil
}

func (m memoryFiles) Open(_ context.Context, key string) (io.ReadCloser, string, error) {
	return io.NopCloser(strings.NewReader(string(m[key]))), "", nil
}

func (m memoryFiles) Delete(_ context.Context, key string) error {
	delete(m, key)
	return nil
}

func TestDownloadsRoundTrip(t *testing.T) {
	ctx := context.Background()
	_, _, _, admin, owner := bankingService(t)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	store := postgres.NewSubscriptionStore(testPool)
	svc := downloads.NewService(admins, store, memoryFiles{}, ids, time.Now)
	user := owner
	first, err := svc.Add(ctx, admin, &user, downloads.ExportContacts, "contacts.csv", "text/csv", []byte("a,b"))
	if err != nil {
		t.Fatal(err)
	}
	failed, err := svc.AddFailed(ctx, admin, nil, downloads.Auditfile, "")
	if err != nil {
		t.Fatal(err)
	}
	all := downloads.Types()
	list, total, err := svc.List(ctx, admin, owner, downloads.Query{Types: all})
	if err != nil || total != 2 || list[0].ID != failed.ID || *list[1].UserID != owner || list[1].Size != 3 {
		t.Errorf("List() = %+v (%d), %v", list, total, err)
	}
	if list, total, _ := svc.List(ctx, admin, owner, downloads.Query{Types: all, PerPage: 1, Page: 3}); len(list) != 0 || total != 2 {
		t.Errorf("past the end = %+v (%d)", list, total)
	}
	if list, _, _ := svc.List(ctx, admin, owner, downloads.Query{Types: all, Type: downloads.Auditfile, Failed: ptr(true)}); len(list) != 1 {
		t.Errorf("by type = %+v", list)
	}
	if list, _, _ := svc.List(ctx, admin, owner, downloads.Query{Types: []string{downloads.ExportDocuments}}); len(list) != 0 {
		t.Errorf("not allowed = %+v", list)
	}
	if _, body, err := svc.Open(ctx, admin, first.ID, owner); err != nil {
		t.Fatal(err)
	} else {
		_ = body.Close()
	}
	if list, _, _ := svc.List(ctx, admin, owner, downloads.Query{Types: all, Downloaded: ptr(true)}); len(list) != 1 || list[0].ID != first.ID {
		t.Errorf("downloaded = %+v", list)
	}
	if _, err := store.Download(ctx, admin, resource.ID(1)); !errors.Is(err, downloads.ErrNotFound) {
		t.Errorf("missing = %v", err)
	}
}
