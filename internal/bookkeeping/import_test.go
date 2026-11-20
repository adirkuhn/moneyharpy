package bookkeeping

import (
	"context"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func accountBy(store *memStore, key string) LedgerAccount {
	for _, a := range store.accounts {
		if a.Key == key {
			return a
		}
	}
	return LedgerAccount{}
}

func TestImportChartTakesOverMatchingDefaultsAndKeepsTheRest(t *testing.T) {
	s, store := newTestService("NL")
	debtors, vat, rent, bike := resource.ID(9001), resource.ID(9002), resource.ID(9003), resource.ID(9004)
	in := ChartImport{
		Accounts: []ImportedAccount{
			// One category each side with this chart scheme code: ours keeps its role and takes the name.
			{Origin: resource.Origin{ID: debtors}, Name: "Debiteuren", Type: "current_assets", Code: "13011", Taxonomy: "BVorDebHad", Active: true},
			{Origin: resource.Origin{ID: vat}, Name: "Betaalde en/of ontvangen btw", Type: "current_liabilities", Code: "16221.01", Taxonomy: "BSchBepBtwAfo", Active: true},
			{Origin: resource.Origin{ID: rent}, Name: "Huur", Type: "expenses", Code: "4110", Taxonomy: "WBedHuiBeh", Active: true},
			// No default like it: it comes with its Moneybird id, under its parent.
			{Origin: resource.Origin{ID: bike}, Name: "Fiets", Type: "expenses", Code: "4999.1", ParentID: &rent, Active: true},
		},
		Rates: []ImportedRate{
			{Origin: resource.Origin{ID: 9101}, Name: "21% btw", Percentage: "21.0", Type: "sales_invoice", ShowTax: true, Active: true},
			{Origin: resource.Origin{ID: 9102}, Name: "Btw verlegd (binnenland)", Percentage: "0.0", Type: "sales_invoice", Active: true},
			{Origin: resource.Origin{ID: 9103}, Name: "12% btw", Percentage: "12", Type: "sales_invoice", ShowTax: true, Active: true},
		},
	}
	ids, err := s.ImportChart(context.Background(), adminID, owner, in)
	if err != nil {
		t.Fatal(err)
	}

	receivable := accountBy(store, "accounts_receivable")
	if ids[debtors] != receivable.ID || receivable.Name != "Debiteuren" || receivable.Code != "13011" || receivable.System != "accounts_receivable" {
		t.Errorf("debtors map to %v, ours %+v", ids[debtors], receivable)
	}
	if ids[vat] != accountBy(store, "vat_paid_or_received").ID {
		t.Errorf("VAT account not taken over")
	}
	created, ok := store.accounts[bike]
	if !ok || ids[bike] != bike || created.ParentID == nil || *created.ParentID != ids[rent] {
		t.Errorf("new category %+v, parent mapped to %v", created, ids[rent])
	}
	// Defaults Moneybird has no category for go, but the ones the books rely on stay.
	if accountBy(store, "energy").ID != 0 {
		t.Error("an unused default category is left")
	}
	if accountBy(store, "uncategorized_expenses").ID == 0 {
		t.Error("a system category was removed")
	}

	var reverse TaxRate
	for _, r := range store.rates {
		if r.Name == "Btw verlegd (binnenland)" {
			reverse = r
		}
	}
	if ids[9102] != reverse.ID || len(reverse.ReportBoxes) == 0 {
		t.Errorf("reverse charge rate maps to %v, ours %+v", ids[9102], reverse)
	}
	if r, ok := store.rates[9103]; !ok || r.Percentage != "12" || !r.Active {
		t.Errorf("new rate %+v", r)
	}
	for _, r := range store.rates {
		if r.Name == "Outside the EU" && r.Active {
			t.Error("a default rate Moneybird does not have stays active")
		}
	}
}

func TestImportChartTwiceChangesNothingMore(t *testing.T) {
	s, store := newTestService("NL")
	in := ChartImport{Accounts: []ImportedAccount{{Origin: resource.Origin{ID: 9004}, Name: "Fiets", Type: "expenses", Active: true}}}
	if _, err := s.ImportChart(context.Background(), adminID, owner, in); err != nil {
		t.Fatal(err)
	}
	count := len(store.accounts)
	if _, err := s.ImportChart(context.Background(), adminID, owner, in); err != nil {
		t.Fatal(err)
	}
	if len(store.accounts) != count {
		t.Errorf("accounts %d, then %d", count, len(store.accounts))
	}
}
