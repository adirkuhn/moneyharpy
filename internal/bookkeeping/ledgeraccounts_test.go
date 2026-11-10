package bookkeeping

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestLedgerAccountsAreOrderedByTypeAndCode(t *testing.T) {
	svc, _ := newTestService("NL")
	got, err := svc.LedgerAccounts(context.Background(), adminID, owner)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(got); i++ {
		a, b := got[i-1], got[i]
		ta, tb := slices.Index(AccountTypes, a.Type), slices.Index(AccountTypes, b.Type)
		if ta > tb || (ta == tb && a.Code > b.Code) {
			t.Fatalf("%s (%s %s) before %s (%s %s)", a.Name, a.Type, a.Code, b.Name, b.Type, b.Code)
		}
	}
}

func TestCreateLedgerAccount(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	if _, err := svc.Settings(ctx, adminID, owner); err != nil {
		t.Fatal(err)
	}
	office := byKey(t, store, "office")
	l, err := svc.CreateLedgerAccount(ctx, adminID, owner, LedgerAccountChanges{
		Name: ptr(" Hosting "), Type: ptr("expenses"), Code: ptr("4460"), ParentID: &office.ID, Taxonomy: ptr("WBedKanSof"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if l.Name != "Hosting" || !l.Active || *l.ParentID != office.ID || !slices.Contains(l.AllowedDocumentTypes, "purchase_invoice") {
		t.Errorf("CreateLedgerAccount() = %+v", l)
	}

	revenue := byKey(t, store, "revenue")
	_, err = svc.CreateLedgerAccount(ctx, adminID, owner, LedgerAccountChanges{
		Name: ptr("hosting"), Type: ptr("expenses"), ParentID: &revenue.ID, Code: ptr("123456789012345678901"),
		AllowedDocumentTypes: ptr([]string{"order"}),
	})
	fields := fieldsOf(t, err)
	for _, f := range []string{"name", "parent_id", "account_id", "allowed_document_types", "rgs_code"} {
		if fields[f] == nil {
			t.Errorf("fields = %v, want %s", fields, f)
		}
	}

	for code, want := range map[string]string{"Nope": "is not a code", "BLimKas": "profit and loss"} {
		_, err := svc.CreateLedgerAccount(ctx, adminID, owner, LedgerAccountChanges{Name: ptr("x " + code), Type: ptr("expenses"), Taxonomy: ptr(code)})
		if got := fieldsOf(t, err)["rgs_code"]; len(got) == 0 || !contains(got[0], want) {
			t.Errorf("rgs_code %s problems = %v, want %q", code, got, want)
		}
	}
}

func TestLedgerAccountWithoutSchemeNeedsNoCode(t *testing.T) {
	svc, _ := newTestService("BR")
	ctx := context.Background()
	if _, err := svc.CreateLedgerAccount(ctx, adminID, owner, LedgerAccountChanges{Name: ptr("Hosting"), Type: ptr("expenses")}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreateLedgerAccount(ctx, adminID, owner, LedgerAccountChanges{Name: ptr("Rent 2"), Type: ptr("expenses"), Taxonomy: ptr("WBedHuiBeh")})
	if fieldsOf(t, err)["rgs_code"] == nil {
		t.Error("accepted a code without a chart scheme")
	}
}

func TestUpdateLedgerAccountRejectsCycles(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	if _, err := svc.Settings(ctx, adminID, owner); err != nil {
		t.Fatal(err)
	}
	office, software := byKey(t, store, "office"), byKey(t, store, "software")
	_, err := svc.UpdateLedgerAccount(ctx, adminID, office.ID, owner, LedgerAccountChanges{ParentID: &software.ID})
	if fieldsOf(t, err)["parent_id"] == nil {
		t.Error("accepted a category under its own sub-category")
	}
	if _, err := svc.UpdateLedgerAccount(ctx, adminID, office.ID, owner, LedgerAccountChanges{ParentID: &office.ID}); fieldsOf(t, err)["parent_id"] == nil {
		t.Error("accepted a category as its own parent")
	}
	l, err := svc.UpdateLedgerAccount(ctx, adminID, software.ID, owner, LedgerAccountChanges{ParentID: ptr(resource.ID(0)), Description: ptr("Licences")})
	if err != nil || l.ParentID != nil || l.Description != "Licences" {
		t.Errorf("UpdateLedgerAccount() = %+v, %v", l, err)
	}
}

func TestSystemLedgerAccountsStay(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	if _, err := svc.Settings(ctx, adminID, owner); err != nil {
		t.Fatal(err)
	}
	payable := byKey(t, store, "accounts_payable")
	if _, err := svc.UpdateLedgerAccount(ctx, adminID, payable.ID, owner, LedgerAccountChanges{Type: ptr("equity")}); !errors.Is(err, ErrSystemAccount) {
		t.Errorf("type change error = %v", err)
	}
	if _, err := svc.UpdateLedgerAccount(ctx, adminID, payable.ID, owner, LedgerAccountChanges{Active: ptr(false)}); !errors.Is(err, ErrSystemAccount) {
		t.Errorf("deactivate error = %v", err)
	}
	if _, _, err := svc.DeleteLedgerAccount(ctx, adminID, payable.ID, owner); !errors.Is(err, ErrSystemAccount) {
		t.Errorf("delete error = %v", err)
	}
	if l, err := svc.UpdateLedgerAccount(ctx, adminID, payable.ID, owner, LedgerAccountChanges{Name: ptr("Suppliers")}); err != nil || l.Name != "Suppliers" {
		t.Errorf("rename = %+v, %v", l, err)
	}
}

func TestDeleteDeactivatesThenDeletes(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	if _, err := svc.Settings(ctx, adminID, owner); err != nil {
		t.Fatal(err)
	}
	office, software := byKey(t, store, "office"), byKey(t, store, "software")
	if _, _, err := svc.DeleteLedgerAccount(ctx, adminID, office.ID, owner); !errors.Is(err, ErrHasChildren) {
		t.Errorf("delete with active children error = %v", err)
	}
	l, kept, err := svc.DeleteLedgerAccount(ctx, adminID, software.ID, owner)
	if err != nil || !kept || l.Active {
		t.Fatalf("first delete = %+v, %v, %v", l, kept, err)
	}
	if _, kept, err := svc.DeleteLedgerAccount(ctx, adminID, software.ID, owner); err != nil || kept {
		t.Fatalf("second delete kept = %v, %v", kept, err)
	}
	if _, ok := store.accounts[software.ID]; ok {
		t.Error("inactive category was not deleted")
	}
}

func TestUpdateScheme(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	if _, err := svc.Settings(ctx, adminID, owner); err != nil {
		t.Fatal(err)
	}
	rent, cash := byKey(t, store, "rent"), byKey(t, store, "cash")
	if _, err := svc.UpdateScheme(ctx, adminID, owner, map[resource.ID]string{rent.ID: "WBedHuiSer", cash.ID: ""}); err != nil {
		t.Fatal(err)
	}
	if store.accounts[rent.ID].Taxonomy != "WBedHuiSer" || store.accounts[cash.ID].Taxonomy != "" {
		t.Errorf("rent %q, cash %q", store.accounts[rent.ID].Taxonomy, store.accounts[cash.ID].Taxonomy)
	}
	_, err := svc.UpdateScheme(ctx, adminID, owner, map[resource.ID]string{rent.ID: "BLimKas", 1: "WBedHuiBeh"})
	fields := fieldsOf(t, err)
	if fields["ledger_accounts."+rent.ID.String()] == nil || fields["ledger_accounts.1"] == nil {
		t.Errorf("fields = %v", fields)
	}
}

func TestApplyTemplate(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	before, _ := svc.LedgerAccounts(ctx, adminID, owner)
	added, err := svc.ApplyTemplate(ctx, adminID, owner, "company_car")
	if err != nil {
		t.Fatal(err)
	}
	if len(added) == 0 || len(store.accounts) != len(before)+len(added) {
		t.Fatalf("added %d, have %d, had %d", len(added), len(store.accounts), len(before))
	}
	depreciation := byKey(t, store, "depreciation")
	if p := byKey(t, store, "company_car_depreciation").ParentID; p == nil || *p != depreciation.ID {
		t.Error("template category is not under the existing depreciation category")
	}
	if p := byKey(t, store, "company_car_fuel").ParentID; p == nil || *p != byKey(t, store, "company_car_costs").ID {
		t.Error("template category is not under its template parent")
	}
	again, err := svc.ApplyTemplate(ctx, adminID, owner, "company_car")
	if err != nil || len(again) != 0 {
		t.Errorf("second apply added %d, %v", len(again), err)
	}
	templates, _ := svc.Templates(ctx, adminID, owner)
	for _, tpl := range templates {
		if tpl.Applied != (tpl.Key == "company_car") {
			t.Errorf("%s applied = %v", tpl.Key, tpl.Applied)
		}
	}
	if _, err := svc.ApplyTemplate(ctx, adminID, owner, "spaceship"); !errors.Is(err, ErrTemplateNotFound) {
		t.Errorf("unknown template error = %v", err)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
