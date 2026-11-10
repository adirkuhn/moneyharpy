package banking

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestSeedsACashAccountOnce(t *testing.T) {
	svc, store, _ := newTestService()
	ctx := context.Background()
	for range 2 {
		if _, err := svc.Accounts(ctx, adminID, owner); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := svc.Accounts(ctx, adminID, owner)
	if len(list) != 1 || list[0].Type != CashAccount || list[0].Name != "Cash" || list[0].LedgerAccountID != cashCategory || len(store.accounts) != 1 {
		t.Fatalf("accounts = %+v", list)
	}
	if _, err := svc.Accounts(ctx, adminID, stranger); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("stranger error = %v", err)
	}
}

func TestCreateAccount(t *testing.T) {
	svc, _, ledger := newTestService()
	ctx := context.Background()
	a := bank(t, svc)
	if a.Identifier != "NL91ABNA0417164300" || a.Currency != "EUR" || a.Position != 2 || a.LedgerAccountID == 0 || ledger.created[0] != "Main bank" {
		t.Errorf("account = %+v", a)
	}
	_, err := svc.CreateAccount(ctx, adminID, owner, AccountChanges{Name: ptr("main BANK"), Type: ptr("Piggy"), Identifier: ptr("NL91ABNA0417164301"), Currency: ptr("euro")})
	fields := fieldsOf(t, err)
	for _, f := range []string{"name", "type", "identifier", "currency"} {
		if fields[f] == nil {
			t.Errorf("fields = %v, want %s", fields, f)
		}
	}
	if _, err := svc.CreateAccount(ctx, adminID, owner, AccountChanges{Name: ptr("Card"), Type: ptr(CreditCardAccount), Identifier: ptr("0001")}); err != nil {
		t.Errorf("card with last digits: %v", err)
	}
	if _, err := svc.CreateAccount(ctx, adminID, owner, AccountChanges{Name: ptr("Other"), Identifier: ptr("NL91ABNA0417164300")}); fieldsOf(t, err)["identifier"] == nil {
		t.Error("accepted an account number twice")
	}
}

func TestReorderAccounts(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	b := bank(t, svc)
	list, _ := svc.Accounts(ctx, adminID, owner)
	if err := svc.ReorderAccounts(ctx, adminID, owner, []resource.ID{b.ID, list[0].ID}); err != nil {
		t.Fatal(err)
	}
	if list, _ = svc.Accounts(ctx, adminID, owner); list[0].ID != b.ID {
		t.Errorf("first = %s", list[0].Name)
	}
	if fieldsOf(t, svc.ReorderAccounts(ctx, adminID, owner, []resource.ID{b.ID}))["ids"] == nil {
		t.Error("accepted an incomplete order")
	}
}

func TestCreateStatementSkipsKnownTransactions(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	acc := bank(t, svc)
	in := StatementInput{AccountID: &acc.ID, Reference: ptr("S1"), OfficialBalance: ptr("10,00"), Mutations: []MutationInput{
		{Date: ptr("2026-11-01"), Amount: ptr("10"), TransactionID: ptr("T1"), ContraAccount: ptr("nl02 rabo 0123 4567 89")},
	}}
	st, err := svc.CreateStatement(ctx, adminID, owner, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Mutations) != 1 || st.Mutations[0].ContraAccount != "NL02RABO0123456789" || *st.OfficialBalance != 1000 || st.Importer != ImporterAPI {
		t.Fatalf("statement = %+v", st)
	}
	if again, _ := svc.CreateStatement(ctx, adminID, owner, in); len(again.Mutations) != 0 {
		t.Errorf("duplicate imported again: %+v", again.Mutations)
	}
	if got, _ := svc.Account(ctx, adminID, acc.ID, owner); got.LastImportAt == nil {
		t.Error("account not marked updated")
	}
	_, err = svc.CreateStatement(ctx, adminID, owner, StatementInput{AccountID: &acc.ID, Mutations: []MutationInput{{Amount: ptr("0")}}})
	fields := fieldsOf(t, err)
	for _, f := range []string{"reference", "financial_mutations_attributes.0.date", "financial_mutations_attributes.0.amount"} {
		if fields[f] == nil {
			t.Errorf("fields = %v, want %s", fields, f)
		}
	}
	if _, err := svc.CreateStatement(ctx, adminID, owner, StatementInput{AccountID: ptr(resource.ID(1)), Reference: ptr("x")}); fieldsOf(t, err)["financial_account_id"] == nil {
		t.Error("accepted an unknown account")
	}
}

func TestStatementsRespectTheLockedPeriod(t *testing.T) {
	svc, _, ledger := newTestService()
	ctx := context.Background()
	acc := bank(t, svc)
	ledger.locked = "2026-10-31"
	_, err := svc.CreateStatement(ctx, adminID, owner, StatementInput{AccountID: &acc.ID, Reference: ptr("S"), Mutations: []MutationInput{{Date: ptr("2026-10-31"), Amount: ptr("1")}}})
	if fieldsOf(t, err)["financial_mutations_attributes.0.date"] == nil {
		t.Error("accepted a transaction in the locked period")
	}
}

func TestUpdateAndDeleteStatement(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	acc := bank(t, svc)
	ms := transactions(t, svc, acc, "10", "-5")
	stID := ms[0].StatementID
	st, err := svc.UpdateStatement(ctx, adminID, stID, owner, StatementInput{Reference: ptr("S1b"), Mutations: []MutationInput{
		{ID: &ms[0].ID, Message: ptr("Changed")}, {ID: &ms[1].ID, Destroy: true}, {Date: ptr("2026-11-02"), Amount: ptr("3")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if st.Reference != "S1b" || len(st.Mutations) != 2 {
		t.Fatalf("statement = %+v", st)
	}
	if _, err := svc.LinkBooking(ctx, adminID, ms[0].ID, owner, Link{Type: "LedgerAccount", BookingID: ptr(rentCategory)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateStatement(ctx, adminID, stID, owner, StatementInput{Mutations: []MutationInput{{ID: &ms[0].ID, Destroy: true}}}); err == nil {
		t.Error("removed a booked transaction")
	}
	if err := svc.DeleteStatement(ctx, adminID, stID, owner); !errors.Is(err, ErrStatementBooked) {
		t.Errorf("delete booked statement error = %v", err)
	}
	other := transactions(t, svc, acc, "1")
	if err := svc.DeleteStatement(ctx, adminID, other[0].StatementID, owner); err != nil {
		t.Errorf("delete = %v", err)
	}
	if _, err := svc.Statement(ctx, adminID, other[0].StatementID, owner); !errors.Is(err, ErrStatementNotFound) {
		t.Errorf("deleted statement error = %v", err)
	}
}

func TestImportFiles(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	acc := bank(t, svc)
	for _, name := range []string{"camt053.xml", "mt940.sta"} {
		data, err := os.ReadFile("../bankfile/testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		res, err := svc.Import(ctx, adminID, acc.ID, owner, data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.Imported == 0 || len(res.Statements) != 1 {
			t.Errorf("%s: result = %+v", name, res)
		}
		again, err := svc.Import(ctx, adminID, acc.ID, owner, data)
		if err != nil || again.Imported != 0 || again.Duplicates != res.Imported+res.Duplicates {
			t.Errorf("%s again: %+v, %v", name, again, err)
		}
	}
	card, _ := svc.CreateAccount(ctx, adminID, owner, AccountChanges{Name: ptr("Other bank"), Identifier: ptr("NL02ABNA0123456789")})
	data, _ := os.ReadFile("../bankfile/testdata/camt053.xml")
	if _, err := svc.Import(ctx, adminID, card.ID, owner, data); fieldsOf(t, err)["file"] == nil {
		t.Error("imported a file for another account")
	}
	if _, err := svc.Import(ctx, adminID, acc.ID, owner, []byte("hello")); fieldsOf(t, err)["file"] == nil {
		t.Error("imported an unknown format")
	}
}
