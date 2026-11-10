package overview

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID                resource.ID = 100
	owner, clerk, outsider resource.ID = 1, 2, 3
)

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID {
		return administration.Listed{}, administration.ErrNotFound
	}
	switch user {
	case owner:
		return administration.Listed{Membership: administration.Membership{Permissions: slices.Clone(administration.Permissions)}}, nil
	case clerk:
		return administration.Listed{Membership: administration.Membership{Permissions: []administration.Permission{administration.SalesInvoices}}}, nil
	}
	return administration.Listed{}, administration.ErrNotFound
}

type fakeBank struct {
	calls  int
	recent int
}

var processed = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

func (f *fakeBank) Summary(_ context.Context, _, _ resource.ID, recent int) (banking.Summary, error) {
	f.calls++
	f.recent = recent
	return banking.Summary{Balance: 409126, Currency: "EUR", Unprocessed: 3, RecentlyProcessed: []banking.Mutation{
		{ID: 7, Date: processed, Amount: -1250, Currency: "EUR", ContraName: "Hosting Company", Message: "Invoice 12", State: banking.Processed, ProcessedAt: &processed},
		{ID: 8, Date: processed, Amount: 500, Currency: "EUR", Message: "Cash deposit\nshop", State: banking.Processed, ProcessedAt: &processed},
	}}, nil
}

func TestOwnerSeesTheBank(t *testing.T) {
	bank := &fakeBank{}
	got, err := NewService(fakeAdmins{}, bank).Get(context.Background(), adminID, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bank == nil || got.Bank.Balance != 409126 || got.Bank.Currency != "EUR" || got.Bank.Unprocessed != 3 {
		t.Fatalf("bank = %+v", got.Bank)
	}
	if bank.recent != RecentLimit || len(got.RecentlyProcessed) != 2 {
		t.Fatalf("recent = %d, items %+v", bank.recent, got.RecentlyProcessed)
	}
	first, second := got.RecentlyProcessed[0], got.RecentlyProcessed[1]
	if first.Type != FinancialMutation || first.ID != 7 || first.Title != "Hosting Company" || first.Description != "Invoice 12" || first.Amount != "-12.5" || first.State != "processed" {
		t.Errorf("first = %+v", first)
	}
	// Without a contra name, the message's first line is the title.
	if second.Title != "Cash deposit" || second.Description != "Cash deposit shop" {
		t.Errorf("second = %+v", second)
	}
}

func TestBankNeedsThePermissionAndTheScope(t *testing.T) {
	bank := &fakeBank{}
	svc := NewService(fakeAdmins{}, bank)
	got, err := svc.Get(context.Background(), adminID, clerk, nil)
	if err != nil || got.Bank != nil || len(got.RecentlyProcessed) != 0 || got.RecentlyProcessed == nil {
		t.Errorf("clerk = %+v, %v", got, err)
	}
	got, err = svc.Get(context.Background(), adminID, owner, []administration.Permission{administration.Documents})
	if err != nil || got.Bank != nil {
		t.Errorf("token without bank scope = %+v, %v", got, err)
	}
	if bank.calls != 0 {
		t.Errorf("bank asked %d times", bank.calls)
	}
}

func TestStrangersGetNotFound(t *testing.T) {
	if _, err := NewService(fakeAdmins{}, &fakeBank{}).Get(context.Background(), adminID, outsider, nil); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("error = %v", err)
	}
}
