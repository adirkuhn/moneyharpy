package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/projects"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

func TestProductsRoundTripAndStayWhileInUse(t *testing.T) {
	ctx := context.Background()
	f := newSalesFixture(t)
	p, err := f.products.Create(ctx, f.admin, f.owner, products.Changes{
		Title: ptr("Widget"), Description: ptr("Widget, blue"), Identifier: ptr("W-1"), Price: ptr("25"), TaxRateID: &f.vat21,
		LedgerAccountID: &f.revenue, FrequencyPreset: ptr("1:month"), ProductType: ptr("product"), MaxAmountPerOrder: ptr(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.products.Product(ctx, f.admin, p.ID, f.owner)
	if err != nil || got.Price != 2500 || got.Frequency != 1 || got.FrequencyType != "month" || *got.TaxRateID != f.vat21 || got.ProductType != "product" ||
		*got.MaxAmountPerOrder != 1 || !got.Active || got.Currency != "EUR" {
		t.Fatalf("Product() = %+v, %v", got, err)
	}
	if byCode, err := f.products.ByIdentifier(ctx, f.admin, f.owner, "W-1"); err != nil || byCode.ID != p.ID {
		t.Errorf("ByIdentifier() = %+v, %v", byCode, err)
	}
	if _, err := f.products.Create(ctx, f.admin, f.owner, products.Changes{Title: ptr("Copy"), Identifier: ptr("W-1"), Price: ptr("1"), LedgerAccountID: &f.revenue}); err == nil {
		t.Error("duplicate identifier accepted")
	}
	other, err := f.products.Create(ctx, f.admin, f.owner, products.Changes{Description: ptr("Gadget 100%"), Price: ptr("1"), LedgerAccountID: &f.revenue, Currency: ptr("USD")})
	if err != nil {
		t.Fatal(err)
	}
	if list, total, err := f.products.List(ctx, f.admin, f.owner, products.Query{Search: "100%"}); err != nil || total != 1 || list[0].ID != other.ID {
		t.Errorf("search = %+v (%d), %v", list, total, err)
	}
	if list, total, _ := f.products.List(ctx, f.admin, f.owner, products.Query{PerPage: 1, Page: 2}); total != 2 || len(list) != 1 || list[0].ID != p.ID {
		t.Errorf("page 2 = %+v of %d", list, total)
	}
	if list, _, _ := f.products.List(ctx, f.admin, f.owner, products.Query{Currency: "USD", LedgerAccountID: &f.revenue}); len(list) != 1 || list[0].ID != other.ID {
		t.Errorf("by currency = %+v", list)
	}
	if list, _, _ := f.products.List(ctx, f.admin, f.owner, products.Query{ForCheckout: ptr(true)}); len(list) != 0 {
		t.Errorf("for checkout = %+v", list)
	}

	inv, err := f.svc.Create(ctx, f.admin, f.owner, sales.Changes{ContactID: &f.customer, Details: []sales.DetailInput{{ProductID: &p.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if stored, _ := f.svc.Invoice(ctx, f.admin, inv.ID, f.owner); *stored.Lines[0].ProductID != p.ID || stored.Lines[0].Description != "Widget, blue" || stored.Lines[0].Price != 2500 {
		t.Errorf("line = %+v", stored.Lines[0])
	}
	if kept, err := f.products.Delete(ctx, f.admin, p.ID, f.owner); err != nil || !kept {
		t.Errorf("delete used = %v, %v", kept, err)
	}
	if got, _ := f.products.Product(ctx, f.admin, p.ID, f.owner); got.Active {
		t.Error("used product still active")
	}
	if list, _, _ := f.products.List(ctx, f.admin, f.owner, products.Query{}); len(list) != 1 {
		t.Errorf("active products = %+v", list)
	}
	if kept, err := f.products.Delete(ctx, f.admin, other.ID, f.owner); err != nil || kept {
		t.Errorf("delete free = %v, %v", kept, err)
	}
	if _, err := f.products.Product(ctx, f.admin, other.ID, f.owner); !errors.Is(err, products.ErrNotFound) {
		t.Errorf("deleted product = %v", err)
	}
}

func TestProjectsRoundTripAndStayWhileInUse(t *testing.T) {
	ctx := context.Background()
	f := newSalesFixture(t)
	falcon, err := f.projects.Create(ctx, f.admin, f.owner, projects.Changes{Name: ptr("Falcon"), Budget: ptr("12")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.projects.Create(ctx, f.admin, f.owner, projects.Changes{Name: ptr("FALCON")}); err == nil {
		t.Error("duplicate name accepted")
	}
	eagle, _ := f.projects.Create(ctx, f.admin, f.owner, projects.Changes{Name: ptr("Eagle")})
	booked, _ := f.projects.Create(ctx, f.admin, f.owner, projects.Changes{Name: ptr("Booked")})
	if got, err := f.projects.Project(ctx, f.admin, falcon.ID, f.owner); err != nil || *got.Budget != 12 || got.State != projects.Active {
		t.Errorf("Project() = %+v, %v", got, err)
	}

	if _, err := f.svc.Create(ctx, f.admin, f.owner, sales.Changes{ContactID: &f.customer, Details: []sales.DetailInput{
		{Description: ptr("Work"), Price: ptr("1"), TaxRateID: &f.vat21, ProjectID: &falcon.ID},
	}}); err != nil {
		t.Fatal(err)
	}
	rows := []journal.RowInput{
		{LedgerAccountID: &f.revenue, Debit: ptr("1"), ProjectID: &booked.ID},
		{LedgerAccountID: &f.revenue, Credit: ptr("1")},
	}
	if _, err := f.journal.CreateDocument(ctx, f.admin, f.owner, journal.DocumentChanges{Reference: ptr("Booked"), Date: ptr("2026-11-01"), Rows: rows}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []resource.ID{falcon.ID, booked.ID} {
		if kept, err := f.projects.Delete(ctx, f.admin, id, f.owner); err != nil || !kept {
			t.Errorf("delete used %d = %v, %v", id, kept, err)
		}
	}
	if kept, err := f.projects.Delete(ctx, f.admin, eagle.ID, f.owner); err != nil || kept {
		t.Errorf("delete free = %v, %v", kept, err)
	}
	active, total, err := f.projects.List(ctx, f.admin, f.owner, projects.Query{States: []string{projects.Active}})
	if err != nil || total != 0 || len(active) != 0 {
		t.Errorf("active = %+v (%d), %v", active, total, err)
	}
	if all, total, _ := f.projects.List(ctx, f.admin, f.owner, projects.Query{}); total != 2 || all[0].Name != "Booked" || all[0].State != projects.Archived {
		t.Errorf("all = %+v", all)
	}
}
