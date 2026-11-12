package bookkeeping

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func template(t *testing.T, svc *Service, key string) Template {
	t.Helper()
	list, err := svc.Templates(context.Background(), adminID, owner)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(list, func(tpl Template) bool { return tpl.Key == key })
	if i < 0 {
		t.Fatalf("no template %s", key)
	}
	return list[i]
}

func templateAccount(t *testing.T, tpl Template, key string) TemplateAccount {
	t.Helper()
	i := slices.IndexFunc(tpl.Accounts, func(a TemplateAccount) bool { return a.Key == key })
	if i < 0 {
		t.Fatalf("%s does not show %s", tpl.Key, key)
	}
	return tpl.Accounts[i]
}

func TestTemplatesShowWhatExistsAndTheTree(t *testing.T) {
	svc, _ := newTestService("NL")
	housing := template(t, svc, "housing")
	if housing.Name != "Housing" || housing.Intro == "" || housing.Applied {
		t.Errorf("housing = %+v", housing)
	}
	costs := templateAccount(t, housing, "housing")
	if !costs.Exists || costs.Parent != "" || costs.Name != "Housing costs" {
		t.Errorf("housing costs = %+v", costs)
	}
	tax := templateAccount(t, housing, "housing_tax")
	if tax.Exists || tax.Parent != "housing" || tax.Type != "expenses" || tax.Taxonomy != "WBedHuiOnz" {
		t.Errorf("housing tax = %+v", tax)
	}

	car := template(t, svc, "car_set")
	if car.Choice == nil || len(car.Choice.Options) != 3 || car.Choice.Label != "Financing of the car" {
		t.Fatalf("car choice = %+v", car.Choice)
	}
	lease := templateAccount(t, car, "company_car_lease_costs")
	if !slices.Equal(lease.Choices, []string{"operational_lease"}) || !lease.Selected || lease.Description == "" {
		t.Errorf("lease costs = %+v", lease)
	}

	revenue := template(t, svc, "revenue")
	if revenue.Single == nil || revenue.Single.Type != "revenue" || !revenue.Single.Parent || revenue.Single.Options[0].Name != "Revenue from trade goods" {
		t.Errorf("revenue = %+v", revenue.Single)
	}
	if entrepreneur := template(t, svc, "entrepreneur"); entrepreneur.NameLabel != "Name entrepreneur" {
		t.Errorf("entrepreneur asks for %q", entrepreneur.NameLabel)
	}
}

func TestTemplatesLeaveCodesOffWithoutTheScheme(t *testing.T) {
	svc, _ := newTestService("NL")
	ctx := context.Background()
	if _, err := svc.UpdateSettings(ctx, adminID, owner, SettingsChanges{Scheme: ptr("")}); err != nil {
		t.Fatal(err)
	}
	if tax := templateAccount(t, template(t, svc, "housing"), "housing_tax"); tax.Taxonomy != "" {
		t.Errorf("code %q without a scheme", tax.Taxonomy)
	}
	if o := template(t, svc, "revenue").Single.Options[0]; o.Taxonomy != "" {
		t.Errorf("option code %q without a scheme", o.Taxonomy)
	}
}

func TestApplyTemplateWithoutChoiceAddsAllMissing(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	before, _ := svc.LedgerAccounts(ctx, adminID, owner)
	added, err := svc.ApplyTemplate(ctx, adminID, owner, "car_set", TemplateInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(added) == 0 || len(store.accounts) != len(before)+len(added) {
		t.Fatalf("added %d, have %d, had %d", len(added), len(store.accounts), len(before))
	}
	depreciation := byKey(t, store, "depreciation")
	if p := byKey(t, store, "company_car_depreciation").ParentID; p == nil || *p != depreciation.ID {
		t.Error("theme category is not under the existing depreciation category")
	}
	if p := byKey(t, store, "company_car_fuel").ParentID; p == nil || *p != byKey(t, store, "company_car_costs").ID {
		t.Error("theme category is not under its theme parent")
	}
	if l := byKey(t, store, "company_car_fuel"); l.Template != "car_set" || l.Taxonomy != "WBedAutBra" {
		t.Errorf("fuel = %+v", l)
	}
	again, err := svc.ApplyTemplate(ctx, adminID, owner, "car_set", TemplateInput{})
	if err != nil || len(again) != 0 {
		t.Errorf("second apply added %d, %v", len(again), err)
	}
	if !template(t, svc, "car_set").Applied || template(t, svc, "housing").Applied {
		t.Error("applied is wrong")
	}
	if _, err := svc.ApplyTemplate(ctx, adminID, owner, "spaceship", TemplateInput{}); !errors.Is(err, ErrTemplateNotFound) {
		t.Errorf("unknown template error = %v", err)
	}
}

func TestApplyTemplateAddsTheTickedCategoriesWithTheirParents(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	added, err := svc.ApplyTemplate(ctx, adminID, owner, "staff_set", TemplateInput{Accounts: []string{"wages", "rent"}})
	if fieldsOf(t, err)["ledger_accounts"] == nil {
		t.Fatalf("a category of another theme: %v, added %d", err, len(added))
	}
	added, err = svc.ApplyTemplate(ctx, adminID, owner, "staff_set", TemplateInput{Accounts: []string{"wages", "pensions"}})
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, l := range added {
		keys = append(keys, l.Key)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, []string{"pensions", "staff", "wages"}) {
		t.Errorf("added %v", keys)
	}
	if p := byKey(t, store, "wages").ParentID; p == nil || *p != byKey(t, store, "staff").ID {
		t.Error("wages are not under staff")
	}
	exists := templateAccount(t, template(t, svc, "staff_set"), "wages").Exists
	if !exists {
		t.Error("added category does not exist")
	}
	// Ticking what exists adds nothing.
	if again, err := svc.ApplyTemplate(ctx, adminID, owner, "staff_set", TemplateInput{Accounts: []string{"wages"}}); err != nil || len(again) != 0 {
		t.Errorf("again added %d, %v", len(again), err)
	}
}

func TestApplyTemplateFillsInTheName(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	if _, err := svc.ApplyTemplate(ctx, adminID, owner, "entrepreneur", TemplateInput{Accounts: []string{"entrepreneur_withdrawals"}}); fieldsOf(t, err)["name"] == nil {
		t.Errorf("without a name: %v", err)
	}
	added, err := svc.ApplyTemplate(ctx, adminID, owner, "entrepreneur", TemplateInput{Name: " Ann ", Accounts: []string{"entrepreneur_withdrawals"}})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, l := range added {
		names = append(names, l.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"Private Ann (capital)", "Private withdrawals Ann"}) {
		t.Fatalf("added %v", names)
	}
	capital := store.accounts[added[slices.IndexFunc(added, func(l LedgerAccount) bool { return l.Name == "Private Ann (capital)" })].ID]
	// A second entrepreneur gets their own categories; the first one's are not added again.
	more, err := svc.ApplyTemplate(ctx, adminID, owner, "entrepreneur", TemplateInput{Name: "Bob", Accounts: []string{"entrepreneur_deposits"}})
	if err != nil || len(more) != 2 {
		t.Fatalf("Bob added %d, %v", len(more), err)
	}
	again, err := svc.ApplyTemplate(ctx, adminID, owner, "entrepreneur", TemplateInput{Name: "Ann", Accounts: []string{"entrepreneur_deposits"}})
	if err != nil || len(again) != 1 || again[0].Name != "Deposits Ann" || again[0].ParentID == nil || *again[0].ParentID != capital.ID {
		t.Errorf("Ann again = %+v, %v", again, err)
	}
}

func TestApplyTemplateAddsOneNamedCategory(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	if _, err := svc.LedgerAccounts(ctx, adminID, owner); err != nil {
		t.Fatal(err)
	}
	revenue := byKey(t, store, "revenue")
	added, err := svc.ApplyTemplate(ctx, adminID, owner, "revenue", TemplateInput{Name: "Workshops", Option: "revenue_from_provision_of_services", ParentID: &revenue.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 {
		t.Fatalf("added %d", len(added))
	}
	l := added[0]
	if l.Name != "Workshops" || l.Type != "revenue" || l.Taxonomy != "WOmzNod" || l.ParentID == nil || *l.ParentID != revenue.ID || l.Template != "revenue" {
		t.Errorf("added %+v", l)
	}
	_, err = svc.ApplyTemplate(ctx, adminID, owner, "revenue", TemplateInput{Option: "spaceship"})
	if f := fieldsOf(t, err); f["name"] == nil || f["option"] == nil {
		t.Errorf("fields = %v", f)
	}
	_, err = svc.ApplyTemplate(ctx, adminID, owner, "revenue", TemplateInput{Name: "Workshops", Option: "net_sales_of_trade_goods"})
	if fieldsOf(t, err)["name"] == nil {
		t.Errorf("a taken name: %v", err)
	}
	housing := byKey(t, store, "housing")
	_, err = svc.ApplyTemplate(ctx, adminID, owner, "revenue", TemplateInput{Name: "Rentals", Option: "net_sales_of_trade_goods", ParentID: &housing.ID})
	if fieldsOf(t, err)["parent_id"] == nil {
		t.Errorf("a parent of another type: %v", err)
	}
}
