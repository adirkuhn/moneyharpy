package postgres_test

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/projects"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

type diskless struct{}

func (diskless) Put(context.Context, string, string, []byte) error { return nil }
func (diskless) Open(context.Context, string) (io.ReadCloser, string, error) {
	return nil, "", os.ErrNotExist
}
func (diskless) Delete(context.Context, string) error { return nil }

func TestJournalPostsBankAndJournalEntries(t *testing.T) {
	ctx := context.Background()
	bank, books, _, admin, owner := bankingService(t)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	noteSvc := notes.NewService(postgres.NewNoteStore(testPool), ids, time.Now)
	store := postgres.NewJournalStore(testPool)
	people := contacts.NewService(admins, noWorkflows{}, admins, store, noteSvc, postgres.NewContactStore(testPool), ids, time.Now)
	catalog := postgres.NewCatalogStore(testPool)
	svc := journal.NewService(admins, books, people, projects.NewService(admins, catalog, store, ids, time.Now), store, store, noteSvc, attachments.NewService(postgres.NewAttachmentStore(testPool), diskless{}, ids, time.Now), ids, time.Now)

	acc, err := bank.CreateAccount(ctx, admin, owner, banking.AccountChanges{Name: ptr("Main bank"), Identifier: ptr("NL91ABNA0417164300")})
	if err != nil {
		t.Fatal(err)
	}
	software := categoryByKey(t, books, admin, owner, "software")
	if _, err := bank.CreateRule(ctx, admin, owner, banking.RuleChanges{
		Name: ptr("Hosting"), LedgerAccountID: &software, Direction: ptr("debit"),
		Conditions: &[]banking.Condition{{Field: "contra_account_name", Operator: "contains", Value: "hosting"}},
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../bankfile/testdata/camt053.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bank.Import(ctx, admin, acc.ID, owner, data); err != nil {
		t.Fatal(err)
	}

	rent := categoryByKey(t, books, admin, owner, "rent")
	deposits := categoryByKey(t, books, admin, owner, "private_deposits")
	customer, err := people.Create(ctx, admin, owner, contacts.Changes{CompanyName: ptr("Example Shop B.V.")})
	if err != nil {
		t.Fatal(err)
	}
	d, err := svc.CreateDocument(ctx, admin, owner, journal.DocumentChanges{
		Reference: ptr("Rent paid privately"), Date: ptr("2026-11-05"),
		Rows: []journal.RowInput{
			{LedgerAccountID: &rent, Debit: ptr("400"), ContactID: &customer.ID},
			{LedgerAccountID: &deposits, Credit: ptr("400")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := svc.GeneralLedger(ctx, admin, owner, "202611", "")
	if err != nil {
		t.Fatal(err)
	}
	sums := map[resource.ID]journal.AccountSum{}
	var total money.Amount
	for _, s := range report.Sums {
		sums[s.LedgerAccountID] = s
		total += s.Debit - s.Credit
	}
	unprocessed := categoryByKey(t, books, admin, owner, "unprocessed_transactions")
	if total != 0 {
		t.Errorf("journal does not balance: %s", total)
	}
	if s := sums[acc.LedgerAccountID]; s.Debit != 100000 || s.Credit != 2550 || s.Final != 97450 {
		t.Errorf("bank = %+v", s)
	}
	if s := sums[software]; s.Debit != 2550 {
		t.Errorf("software = %+v", s)
	}
	if s := sums[unprocessed]; s.Credit != 100000 {
		t.Errorf("unprocessed = %+v", s)
	}
	if s := sums[rent]; s.Debit != 40000 {
		t.Errorf("rent = %+v", s)
	}

	entries, total2, err := svc.Entries(ctx, admin, owner, "202611", journal.EntryQuery{ContactID: &customer.ID})
	if err != nil || total2 != 1 || entries[0].DocumentID != d.ID || entries[0].Amount != 40000 {
		t.Errorf("entries for contact = %+v (%d), %v", entries, total2, err)
	}
	if kept, err := people.Delete(ctx, admin, customer.ID, owner); err != nil || !kept {
		t.Errorf("contact in a journal entry: kept %v, %v; want it archived", kept, err)
	}

	list, n, err := svc.Documents(ctx, admin, owner, journal.DocumentQuery{ContactID: &customer.ID})
	if err != nil || n != 1 || len(list[0].Rows) != 2 || list[0].Rows[0].Debit != 40000 {
		t.Errorf("documents = %+v, %v", list, err)
	}
	if err := svc.DeleteDocument(ctx, admin, d.ID, owner); err != nil {
		t.Fatal(err)
	}
	report, _ = svc.GeneralLedger(ctx, admin, owner, "202611", "")
	for _, s := range report.Sums {
		if s.LedgerAccountID == rent {
			t.Errorf("rent still booked: %+v", s)
		}
	}
}
