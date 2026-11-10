package bookkeeping

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestTaxRatesAreOrderedAndFiltered(t *testing.T) {
	svc, _ := newTestService("NL")
	ctx := context.Background()
	all, err := svc.TaxRates(ctx, adminID, owner, TaxRateFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if all[0].Type != "sales_invoice" || all[0].Position != 1 || all[len(all)-1].Type != "general_journal_document" {
		t.Errorf("first %+v, last %+v", all[0], all[len(all)-1])
	}
	got, _ := svc.TaxRates(ctx, adminID, owner, TaxRateFilter{Type: ptr("purchase_invoice"), Percentage: ptr("21.0")})
	if len(got) == 0 {
		t.Fatal("no purchase rates of 21%")
	}
	for _, r := range got {
		if r.Type != "purchase_invoice" || r.Percentage != "21" {
			t.Errorf("filter matched %+v", r)
		}
	}
	later := now.Add(time.Hour)
	if got, _ := svc.TaxRates(ctx, adminID, owner, TaxRateFilter{CreatedAfter: &later}); len(got) != 0 {
		t.Errorf("created_after matched %d rates", len(got))
	}
	if got, _ := svc.TaxRates(ctx, adminID, owner, TaxRateFilter{PartialName: ptr("REVERSE")}); len(got) == 0 {
		t.Error("partial_name is not case-insensitive")
	}
}

func TestCreateAndUpdateTaxRate(t *testing.T) {
	svc, _ := newTestService("NL")
	ctx := context.Background()
	r, err := svc.CreateTaxRate(ctx, adminID, owner, TaxRateChanges{
		Name: ptr(" Sales 9.5% "), Percentage: ptr("9,50"), Type: ptr("sales_invoice"), ReportBoxes: ptr([]string{"NL/1b", "NL/1b"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	sales, _ := svc.TaxRates(ctx, adminID, owner, TaxRateFilter{Type: ptr("sales_invoice")})
	if r.Name != "Sales 9.5%" || r.Percentage != "9.5" || !r.Active || !r.ShowTax || len(r.ReportBoxes) != 1 || r.Position != len(sales) {
		t.Errorf("CreateTaxRate() = %+v, %d sales rates", r, len(sales))
	}

	u, err := svc.UpdateTaxRate(ctx, adminID, r.ID, owner, TaxRateChanges{Type: ptr("purchase_invoice"), Active: ptr(false)})
	if err != nil {
		t.Fatal(err)
	}
	purchase, _ := svc.TaxRates(ctx, adminID, owner, TaxRateFilter{Type: ptr("purchase_invoice")})
	if u.Active || u.Position != len(purchase) {
		t.Errorf("UpdateTaxRate() = %+v, %d purchase rates", u, len(purchase))
	}

	_, err = svc.CreateTaxRate(ctx, adminID, owner, TaxRateChanges{
		Name: ptr(""), Percentage: ptr("101"), Type: ptr("order"), Country: ptr("NLD"), ReportBoxes: ptr([]string{"DE/81"}),
	})
	fields := fieldsOf(t, err)
	for _, f := range []string{"name", "percentage", "tax_rate_type", "country", "tax_report_boxes"} {
		if fields[f] == nil {
			t.Errorf("fields = %v, want %s", fields, f)
		}
	}
	for _, p := range []string{"1.234", "-1", "abc"} {
		if _, err := svc.CreateTaxRate(ctx, adminID, owner, TaxRateChanges{Name: ptr("x"), Percentage: ptr(p), Type: ptr("sales_invoice")}); fieldsOf(t, err)["percentage"] == nil {
			t.Errorf("accepted percentage %q", p)
		}
	}
	if _, err := svc.UpdateTaxRate(ctx, adminID, 1, owner, TaxRateChanges{}); !errors.Is(err, ErrTaxRateNotFound) {
		t.Errorf("UpdateTaxRate() unknown error = %v", err)
	}
}

func TestReorderTaxRates(t *testing.T) {
	svc, _ := newTestService("NL")
	ctx := context.Background()
	sales, _ := svc.TaxRates(ctx, adminID, owner, TaxRateFilter{Type: ptr("sales_invoice")})
	ids := make([]resource.ID, len(sales))
	for i, r := range sales {
		ids[len(sales)-1-i] = r.ID
	}
	if err := svc.ReorderTaxRates(ctx, adminID, owner, "sales_invoice", ids); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.TaxRates(ctx, adminID, owner, TaxRateFilter{Type: ptr("sales_invoice")})
	for i, r := range got {
		if r.ID != ids[i] || r.Position != i+1 {
			t.Fatalf("position %d = %d (%d), want %d", i, r.ID, r.Position, ids[i])
		}
	}
	if err := svc.ReorderTaxRates(ctx, adminID, owner, "sales_invoice", ids[1:]); fieldsOf(t, err)["ids"] == nil {
		t.Error("accepted an incomplete order")
	}
}
