package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/mbimport"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestMoneybirdImportStoreRoundTripLeaseAndEmpty(t *testing.T) {
	ctx := context.Background()
	svc, _, _, admin, owner := contactService(t)
	store := postgres.NewMoneybirdImportStore(testPool)
	if _, err := store.Job(ctx, admin); err != mbimport.ErrNotFound {
		t.Fatalf("Job() before any = %v", err)
	}
	if empty, err := store.Empty(ctx, admin); err != nil || !empty {
		t.Fatalf("Empty() = %v, %v", empty, err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	due := now.Add(-time.Second)
	j := mbimport.Job{
		AdministrationID: admin, UserID: owner, SourceID: "123", SourceName: "Example Shop B.V.", State: mbimport.Running,
		Steps: []mbimport.Step{{Kind: mbimport.StepContacts, Total: 2, Imported: 1, Problems: []string{"contact 502: no name"}}},
		Token: "sealed", NextAttemptAt: &due, StartedAt: now, UpdatedAt: now,
	}
	if err := store.SaveJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	leased, err := store.LeaseDueJobs(ctx, now, now.Add(time.Minute), 5)
	if err != nil || len(leased) != 1 || leased[0].Steps[0].Problems[0] != "contact 502: no name" || leased[0].SourceName != "Example Shop B.V." {
		t.Fatalf("LeaseDueJobs() = %+v, %v", leased, err)
	}
	if again, _ := store.LeaseDueJobs(ctx, now, now.Add(time.Minute), 5); len(again) != 0 {
		t.Errorf("a leased job was leased again")
	}

	if err := store.RecordImported(ctx, admin, "ledger_account", 9001, 42, now); err != nil {
		t.Fatal(err)
	}
	// Recording it again keeps when it first came in.
	if err := store.RecordImported(ctx, admin, "ledger_account", 9001, 42, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if at, ok, err := store.ImportedAt(ctx, admin, "ledger_account", 9001); err != nil || !ok || !at.Equal(now) {
		t.Errorf("ImportedAt() = %v, %v, %v", at, ok, err)
	}
	if got, ok, err := store.Imported(ctx, admin, "ledger_account", 9001); err != nil || !ok || got != 42 {
		t.Errorf("Imported() = %v, %v, %v", got, ok, err)
	}
	if _, ok, _ := store.Imported(ctx, admin, "contact", 9001); ok {
		t.Error("an id is known for another kind")
	}

	// A contact with its Moneybird id and people goes in as it was.
	id := resource.ID(504315055158854794) + resource.ID(time.Now().UnixNano()%1_000_000)
	first, last := "Ann", "de Vries"
	if _, err := svc.Import(ctx, admin, owner, resource.Origin{ID: id, CreatedAt: now.AddDate(-1, 0, 0)}, contacts.Changes{CompanyName: ptr("Example Shop B.V.")},
		[]contacts.ImportedPerson{{Origin: resource.Origin{ID: id + 1}, Changes: contacts.PersonChanges{Firstname: &first, Lastname: &last}}}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Contact(ctx, admin, id, owner)
	if err != nil || len(got.People) != 1 || got.People[0].ID != id+1 || !got.CreatedAt.Equal(now.AddDate(-1, 0, 0)) {
		t.Errorf("imported contact %+v, %v", got, err)
	}
	if empty, _ := store.Empty(ctx, admin); empty {
		t.Error("an administration with a contact is empty")
	}
}
