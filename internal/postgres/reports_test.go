package postgres_test

import (
	"context"
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
	"github.com/adirkuhn/moneyharpy/internal/reports"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestReportStoreReadsTheJournal(t *testing.T) {
	ctx := context.Background()
	bank, books, _, admin, owner := bankingService(t)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	noteSvc := notes.NewService(postgres.NewNoteStore(testPool), ids, time.Now)
	journalStore := postgres.NewJournalStore(testPool)
	people := contacts.NewService(admins, noWorkflows{}, admins, journalStore, noteSvc, postgres.NewContactStore(testPool), ids, time.Now)
	catalog := postgres.NewCatalogStore(testPool)
	svc := journal.NewService(admins, books, people, projects.NewService(admins, catalog, journalStore, ids, time.Now), journalStore, journalStore, noteSvc,
		attachments.NewService(postgres.NewAttachmentStore(testPool), diskless{}, ids, time.Now), ids, time.Now)
	store := postgres.NewReportStore(testPool)

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
	if _, err := svc.CreateDocument(ctx, admin, owner, journal.DocumentChanges{
		Reference: ptr("Rent paid privately"), Date: ptr("2026-11-05"),
		Rows: []journal.RowInput{{LedgerAccountID: &rent, Debit: ptr("400"), ContactID: &customer.ID}, {LedgerAccountID: &deposits, Credit: ptr("400")}},
	}); err != nil {
		t.Fatal(err)
	}

	accounts, err := store.Accounts(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[resource.ID]reports.Account{}
	for _, a := range accounts {
		byID[a.ID] = a
	}
	if a := byID[acc.LedgerAccountID]; !a.Financial || a.Type != "current_assets" {
		t.Errorf("bank category = %+v", a)
	}
	if a := byID[rent]; a.Financial || a.Code == "" || a.ParentID == nil || a.Taxonomy == "" {
		t.Errorf("rent category = %+v", a)
	}

	nov := money.Period{From: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)}
	sums, err := store.Movements(ctx, admin, &nov.From, nov.To, nil)
	if err != nil {
		t.Fatal(err)
	}
	moved := map[resource.ID]money.Amount{}
	for _, s := range sums {
		moved[s.LedgerAccountID] = s.Amount
	}
	if moved[acc.LedgerAccountID] != 97450 || moved[software] != 2550 || moved[rent] != 40000 {
		t.Errorf("movements = %v", moved)
	}
	if before, _ := store.Movements(ctx, admin, nil, nov.From.AddDate(0, 0, -1), nil); len(before) != 0 {
		t.Errorf("before november = %+v", before)
	}
	years, err := store.YearResults(ctx, admin, nov.To)
	if err != nil || len(years) != 1 || years[0].Year != 2026 || years[0].Amount != 42550 {
		t.Errorf("years = %+v, %v", years, err)
	}

	byContact, err := store.ContactSums(ctx, admin, reports.SubjectQuery{From: &nov.From, To: nov.To, Types: []string{"expenses"}, Roles: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]money.Amount{}
	for _, s := range byContact {
		key := "none"
		if s.ID != nil {
			key = s.ID.String()
		}
		got[key] = s.Amount
	}
	if got[customer.ID.String()] != 40000 || got["none"] != 2550 {
		t.Errorf("by contact = %v", got)
	}
	only, _ := store.ContactSums(ctx, admin, reports.SubjectQuery{From: &nov.From, To: nov.To, Types: []string{"expenses"}, Roles: []string{}, ContactID: &customer.ID})
	if len(only) != 1 {
		t.Errorf("one contact = %+v", only)
	}
	byProject, _ := store.ProjectSums(ctx, admin, reports.SubjectQuery{From: &nov.From, To: nov.To, Types: []string{"expenses"}, Roles: []string{}})
	if len(byProject) != 1 || byProject[0].ID != nil || byProject[0].Amount != 42550 {
		t.Errorf("by project = %+v", byProject)
	}

	cash, err := store.CashFlow(ctx, admin, nov, nil)
	if err != nil {
		t.Fatal(err)
	}
	flows := map[resource.ID]reports.CashSum{}
	for _, c := range cash {
		flows[c.LedgerAccountID] = c
	}
	unprocessed := categoryByKey(t, books, admin, owner, "unprocessed_transactions")
	if flows[software].Paid != 2550 || flows[unprocessed].Received != -100000 {
		t.Errorf("cash flow = %+v", flows)
	}
	if other, _ := store.CashFlow(ctx, admin, nov, ptr(resource.ID(1))); len(other) != 0 {
		t.Errorf("another account = %+v", other)
	}
	if b, err := store.CashBalance(ctx, admin, nov.To, &acc.ID); err != nil || b != 97450 {
		t.Errorf("cash balance = %s, %v", b, err)
	}

	entries, err := store.Entries(ctx, admin, nov)
	if err != nil || len(entries) < 5 {
		t.Errorf("entries = %d, %v", len(entries), err)
	}
	page, total, err := store.LedgerEntries(ctx, admin, acc.LedgerAccountID, nov, 1, 1)
	if err != nil || total != 2 || len(page) != 1 {
		t.Errorf("ledger entries = %+v (%d), %v", page, total, err)
	}
	people2, err := store.Contacts(ctx, admin, []resource.ID{customer.ID})
	if err != nil || len(people2) != 1 || people2[0].Name != "Example Shop B.V." {
		t.Errorf("contacts = %+v, %v", people2, err)
	}
	if tax, err := store.TaxSums(ctx, admin, nov); err != nil || len(tax) != 0 {
		t.Errorf("tax = %+v, %v", tax, err)
	}
	if open, err := store.OpenItems(ctx, admin, "accounts_receivable", nov.To); err != nil || len(open) != 0 {
		t.Errorf("open = %+v, %v", open, err)
	}
	if subs, err := store.Subscriptions(ctx, admin); err != nil || len(subs) != 0 {
		t.Errorf("subscriptions = %+v, %v", subs, err)
	}
}
