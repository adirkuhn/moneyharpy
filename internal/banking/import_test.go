package banking

import (
	"context"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestImportAccountKeepsItsIDAndCategory(t *testing.T) {
	s, store, _ := newTestService()
	ctx := context.Background()
	category := resource.ID(9301)
	a, err := s.ImportAccount(ctx, adminID, owner, ImportedAccount{
		Origin: resource.Origin{ID: 9201}, LedgerAccountID: &category,
		Changes: AccountChanges{Type: ptr(BankAccount), Name: ptr("Business account"), Identifier: ptr("NL91ABNA0417164300"), Currency: ptr("EUR")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != 9201 || store.accounts[9201].LedgerAccountID != category || store.accounts[9201].Name != "Business account" {
		t.Errorf("account %+v", store.accounts[9201])
	}
	// Again changes nothing.
	if again, err := s.ImportAccount(ctx, adminID, owner, ImportedAccount{Origin: resource.Origin{ID: 9201}, Changes: AccountChanges{Name: ptr("Other")}}); err != nil || again.Name != "Business account" {
		t.Errorf("again %+v, %v", again, err)
	}
}

func TestImportAccountTakesOverTheCashAccount(t *testing.T) {
	s, store, _ := newTestService()
	ctx := context.Background()
	accounts, err := s.Accounts(ctx, adminID, owner)
	if err != nil || len(accounts) != 1 || accounts[0].Type != CashAccount {
		t.Fatalf("seeded %+v, %v", accounts, err)
	}
	cash := accounts[0]
	a, err := s.ImportAccount(ctx, adminID, owner, ImportedAccount{Origin: resource.Origin{ID: 9202}, TakeOverCash: true, Changes: AccountChanges{Type: ptr(CashAccount), Name: ptr("Kas")}})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != cash.ID || store.accounts[cash.ID].Name != "Kas" || len(store.accounts) != 1 {
		t.Errorf("took over %+v, accounts %d", a, len(store.accounts))
	}
}

func TestImportStatementKeepsTheTransactionsAsTheyWere(t *testing.T) {
	s, store, _ := newTestService()
	ctx := context.Background()
	if _, err := s.ImportAccount(ctx, adminID, owner, ImportedAccount{Origin: resource.Origin{ID: 9201}, Changes: AccountChanges{Name: ptr("Business account")}}); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2024, 5, 1, 8, 0, 0, 0, time.UTC)
	added, err := s.ImportStatement(ctx, adminID, owner, ImportedStatement{
		Origin: resource.Origin{ID: 9401, CreatedAt: created}, AccountID: 9201, Reference: "Imported from Moneybird",
		Mutations: []ImportedMutation{
			{Origin: resource.Origin{ID: 9501}, Input: MutationInput{Date: ptr("2024-04-30"), Message: ptr("Invoice 2024-0001"), Amount: ptr("121.00"), ContraName: ptr("Example Shop B.V.")}},
			{Origin: resource.Origin{ID: 9502}, Input: MutationInput{Date: ptr("2024-04-30"), Message: ptr("Rent"), Amount: ptr("-500")}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 2 || store.mutations[9501].Amount != 12100 || store.mutations[9502].StatementID != 9401 || store.mutations[9501].State != Unprocessed {
		t.Errorf("added %+v, stored %+v", added, store.mutations)
	}
	if st := store.statements[9401]; !st.CreatedAt.Equal(created) || st.Reference != "Imported from Moneybird" {
		t.Errorf("statement %+v", st)
	}
}
