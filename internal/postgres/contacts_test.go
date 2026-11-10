package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

type noWorkflows struct{}

func (noWorkflows) WorkflowKind(context.Context, resource.ID, resource.ID, resource.ID) (string, bool, error) {
	return "", false, errors.New("workflow not found")
}

func contactService(t *testing.T) (*contacts.Service, *administration.Service, *postgres.ContactStore, resource.ID, resource.ID) {
	t.Helper()
	a, _, owner := newAdministrationWithIdentity(t)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	store := postgres.NewContactStore(testPool)
	return contacts.NewService(admins, noWorkflows{}, admins, nil, notes.NewService(postgres.NewNoteStore(testPool), ids, time.Now), store, ids, time.Now), admins, store, a.ID, owner
}

func TestContactsRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc, admins, store, admin, owner := contactService(t)
	field, err := admins.CreateCustomField(ctx, admin, owner, administration.NewCustomField{Name: "Account manager", Source: administration.SourceContact})
	if err != nil {
		t.Fatal(err)
	}
	c, err := svc.Create(ctx, admin, owner, contacts.Changes{
		CompanyName: ptr("Example Shop B.V."), City: ptr("Utrecht"), SendInvoicesToEmail: ptr("billing@example.com"), SepaActive: ptr(true),
		SepaIBAN: ptr("NL91ABNA0417164300"), SepaIBANAccountName: ptr("Example Shop"), SepaMandateID: ptr("M-1"), SepaMandateDate: ptr("2026-11-01"),
		CustomFields: []contacts.FieldValue{{FieldID: field.ID, Value: "Ann"}},
		Person:       &contacts.PersonChanges{Firstname: ptr("Zoë"), Email: ptr("zoe@example.com")},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Contact(ctx, admin, c.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if got.CustomerID != "1" || got.SepaMandateDate == nil || got.SepaMandateDate.Format(time.DateOnly) != "2026-11-01" || got.City != "Utrecht" {
		t.Errorf("contact = %+v", got)
	}
	if len(got.People) != 1 || got.People[0].Email != "zoe@example.com" {
		t.Errorf("people = %+v", got.People)
	}
	if len(got.CustomFields) != 1 || got.CustomFields[0].Name != "Account manager" || got.CustomFields[0].Value != "Ann" {
		t.Errorf("custom fields = %+v", got.CustomFields)
	}
	if byNumber, err := svc.ContactByCustomerID(ctx, admin, owner, "1"); err != nil || byNumber.ID != c.ID {
		t.Errorf("by customer id = %v, %v", byNumber.ID, err)
	}
	if _, err := store.Contact(ctx, admin, 1); !errors.Is(err, contacts.ErrNotFound) {
		t.Errorf("unknown = %v", err)
	}

	second, err := svc.Create(ctx, admin, owner, contacts.Changes{Firstname: ptr("Ann"), Lastname: ptr("Jansen")})
	if err != nil || second.CustomerID != "2" {
		t.Fatalf("second = %+v, %v", second, err)
	}
	if _, err := svc.Create(ctx, admin, owner, contacts.Changes{CompanyName: ptr("Dup"), CustomerID: ptr("2")}); err == nil {
		t.Error("duplicate customer id accepted")
	}

	page, err := svc.List(ctx, admin, owner, contacts.Query{Search: "zoe@"})
	if err != nil || page.Total != 1 || page.Contacts[0].ID != c.ID {
		t.Errorf("search by person = %+v, %v", page, err)
	}
	page, _ = svc.List(ctx, admin, owner, contacts.Query{Search: "100%"})
	if page.Total != 0 {
		t.Errorf("wildcards not escaped: %d", page.Total)
	}
	page, _ = svc.List(ctx, admin, owner, contacts.Query{PerPage: 1, Page: 2})
	if page.Total != 2 || len(page.Contacts) != 1 || page.Contacts[0].ID != c.ID {
		t.Errorf("page 2 = %+v", page) // "ann jansen" sorts before "example shop"
	}
	page, _ = svc.List(ctx, admin, owner, contacts.Query{PerPage: 1, Page: 9})
	if page.Total != 2 || len(page.Contacts) != 0 {
		t.Errorf("past the end = %+v", page)
	}
	filtered, _ := contacts.ParseFilter("contact_type:private_individual,mandate_type:none")
	page, _ = svc.List(ctx, admin, owner, filtered)
	if page.Total != 1 || page.Contacts[0].ID != second.ID {
		t.Errorf("filter = %+v", page)
	}

	if _, err := svc.Archive(ctx, admin, second.ID, owner); err != nil {
		t.Fatal(err)
	}
	versions, err := svc.Versions(ctx, admin, owner, contacts.Query{})
	if err != nil || len(versions) != 1 || versions[0].ID != c.ID {
		t.Errorf("versions = %+v, %v", versions, err)
	}
	fetched, _ := svc.ContactsByID(ctx, admin, owner, []resource.ID{c.ID, second.ID, 1})
	if len(fetched) != 2 {
		t.Errorf("fetched = %d", len(fetched))
	}

	n, err := svc.AddNote(ctx, admin, c.ID, owner, contacts.NoteInput{Note: "Call back", Todo: true, AssigneeID: &owner})
	if err != nil {
		t.Fatal(err)
	}
	todos, _ := svc.List(ctx, admin, owner, contacts.Query{OpenTodos: true})
	if todos.Total != 1 {
		t.Errorf("open to-dos = %d", todos.Total)
	}
	p, err := svc.UpdatePerson(ctx, admin, c.ID, got.People[0].ID, owner, contacts.PersonChanges{Department: ptr("Finance")})
	if err != nil || p.Department != "Finance" {
		t.Fatalf("person = %+v, %v", p, err)
	}
	if err := svc.DeleteNote(ctx, admin, c.ID, n.ID, owner); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeletePerson(ctx, admin, c.ID, p.ID, owner); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, admin, c.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Contact(ctx, admin, c.ID, owner); !errors.Is(err, contacts.ErrNotFound) {
		t.Errorf("deleted = %v", err)
	}
}
