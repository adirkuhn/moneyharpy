package contacts

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func ptr[T any](v T) *T { return &v }

func fields(t *testing.T, err error) map[string][]string {
	t.Helper()
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want a validation error, got %v", err)
	}
	return verr.Fields
}

func create(t *testing.T, s *Service, c Changes) Contact {
	t.Helper()
	got, err := s.Create(context.Background(), adminID, owner, c)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return got
}

func TestCreateNumbersContactsAndTakesDefaults(t *testing.T) {
	s, _ := newService()
	first := create(t, s, Changes{CompanyName: ptr(" Example Shop B.V. ")})
	if first.CustomerID != "1" || first.Country != "NL" || first.DeliveryMethod != "Email" || first.SepaSequenceType != "RCUR" {
		t.Fatalf("defaults = %+v", first)
	}
	if first.CompanyName != "Example Shop B.V." || first.Name() != "Example Shop B.V." {
		t.Errorf("name = %q", first.Name())
	}
	create(t, s, Changes{Firstname: ptr("Ann"), CustomerID: ptr("42")})
	third := create(t, s, Changes{Lastname: ptr("Jansen")})
	if third.CustomerID != "43" {
		t.Errorf("next customer id = %q, want 43", third.CustomerID)
	}
	if first.Ref().URI() != "/100/contacts/"+first.ID.String() {
		t.Errorf("uri = %s", first.Ref().URI())
	}
}

func TestNextCustomerIDIsTheNumberCreateWouldGive(t *testing.T) {
	s, _ := newService()
	ctx := context.Background()
	next := func() string {
		t.Helper()
		got, err := s.NextCustomerID(ctx, adminID, owner)
		if err != nil {
			t.Fatalf("NextCustomerID: %v", err)
		}
		return got
	}
	if got := next(); got != "1" {
		t.Errorf("first = %q, want 1", got)
	}
	create(t, s, Changes{Firstname: ptr("Ann"), CustomerID: ptr("42")})
	want := next()
	if got := create(t, s, Changes{Lastname: ptr("Jansen")}); want != "43" || got.CustomerID != want {
		t.Errorf("next = %q, created %q", want, got.CustomerID)
	}
	if _, err := s.NextCustomerID(ctx, adminID, stranger); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("stranger = %v", err)
	}
}

func TestCreateNeedsAName(t *testing.T) {
	s, _ := newService()
	_, err := s.Create(context.Background(), adminID, owner, Changes{City: ptr("Utrecht")})
	if f := fields(t, err); len(f["base"]) != 1 {
		t.Errorf("errors = %v", f)
	}
}

func TestCreateValidatesFields(t *testing.T) {
	s, _ := newService()
	_, err := s.Create(context.Background(), adminID, owner, Changes{
		CompanyName:         ptr("X"),
		Country:             ptr("XX"),
		DeliveryMethod:      ptr("fax"),
		SendInvoicesToEmail: ptr("billing@example.com, not-an-email"),
		SepaIBAN:            ptr("NL00ABNA0000000000"),
		SepaBIC:             ptr("bad"),
		SepaSequenceType:    ptr("NOPE"),
		SepaMandateDate:     ptr("10-11-2026"),
		InvoiceWorkflowID:   ptr(estimateFlow),
		EstimateWorkflowID:  ptr(resource.ID(999)),
	})
	f := fields(t, err)
	for _, field := range []string{"country", "delivery_method", "send_invoices_to_email", "sepa_iban", "sepa_bic", "sepa_sequence_type", "sepa_mandate_date", "invoice_workflow_id", "estimate_workflow_id"} {
		if len(f[field]) == 0 {
			t.Errorf("no error for %s in %v", field, f)
		}
	}
}

func TestCreateAcceptsAFullContact(t *testing.T) {
	s, _ := newService()
	c := create(t, s, Changes{
		CompanyName: ptr("Example Shop B.V."), Country: ptr("be"), DeliveryMethod: ptr("peppol"), SIIdentifierType: ptr("0208"), SIIdentifier: ptr("0123456789"),
		SendInvoicesToEmail: ptr("billing@example.com,books@example.com"), SepaActive: ptr(true), SepaIBAN: ptr("nl91 abna 0417 1643 00"),
		SepaIBANAccountName: ptr("Example Shop"), SepaMandateID: ptr("MANDATE-1"), SepaMandateDate: ptr("2026-11-01"), SepaBIC: ptr("abnanl2a"),
		InvoiceWorkflowID: ptr(invoiceFlow), EstimateWorkflowID: ptr(estimateFlow),
		Person: &PersonChanges{Firstname: ptr("Ann"), Email: ptr("ann@example.com")},
	})
	if c.Country != "BE" || c.DeliveryMethod != "Peppol" || c.SepaIBAN != "NL91ABNA0417164300" || c.SepaBIC != "ABNANL2A" {
		t.Errorf("normalized = %+v", c)
	}
	if len(c.People) != 1 || c.People[0].Firstname != "Ann" || c.People[0].ContactID != c.ID {
		t.Errorf("people = %+v", c.People)
	}
}

func TestEInvoicingNeedsAnIdentifier(t *testing.T) {
	s, _ := newService()
	_, err := s.Create(context.Background(), adminID, owner, Changes{CompanyName: ptr("X"), DeliveryMethod: ptr("Peppol")})
	if f := fields(t, err); len(f["si_identifier"]) == 0 {
		t.Errorf("errors = %v", f)
	}
}

func TestMandateNeedsItsDetails(t *testing.T) {
	s, _ := newService()
	_, err := s.Create(context.Background(), adminID, owner, Changes{CompanyName: ptr("X"), SepaActive: ptr(true)})
	f := fields(t, err)
	for _, field := range []string{"sepa_iban", "sepa_iban_account_name", "sepa_mandate_id", "sepa_mandate_date"} {
		if len(f[field]) == 0 {
			t.Errorf("no error for %s", field)
		}
	}
}

func TestWorkflowMustBeActive(t *testing.T) {
	s, _ := newService()
	_, err := s.Create(context.Background(), adminID, owner, Changes{CompanyName: ptr("X"), InvoiceWorkflowID: ptr(closedFlow)})
	if f := fields(t, err); !reflect.DeepEqual(f["invoice_workflow_id"], []string{"is deactivated"}) {
		t.Errorf("errors = %v", f)
	}
}

func TestCustomerIDIsUnique(t *testing.T) {
	s, _ := newService()
	a := create(t, s, Changes{CompanyName: ptr("A"), CustomerID: ptr("C-1")})
	_, err := s.Create(context.Background(), adminID, owner, Changes{CompanyName: ptr("B"), CustomerID: ptr("C-1")})
	if f := fields(t, err); len(f["customer_id"]) == 0 {
		t.Errorf("errors = %v", f)
	}
	if _, err := s.Update(context.Background(), adminID, a.ID, owner, Changes{CustomerID: ptr("C-1"), City: ptr("Delft")}); err != nil {
		t.Errorf("keeping its own id: %v", err)
	}
}

func TestCustomFieldsBelongToContacts(t *testing.T) {
	s, _ := newService()
	c := create(t, s, Changes{CompanyName: ptr("A"), CustomFields: []FieldValue{{FieldID: contactField, Value: " Ann "}}})
	if !reflect.DeepEqual(c.CustomFields, []FieldValue{{FieldID: contactField, Name: "Account manager", Value: "Ann"}}) {
		t.Errorf("custom fields = %+v", c.CustomFields)
	}
	cleared, err := s.Update(context.Background(), adminID, c.ID, owner, Changes{CustomFields: []FieldValue{{FieldID: contactField, Value: ""}}})
	if err != nil || len(cleared.CustomFields) != 0 {
		t.Errorf("cleared = %+v, %v", cleared.CustomFields, err)
	}
	_, err = s.Update(context.Background(), adminID, c.ID, owner, Changes{CustomFields: []FieldValue{{FieldID: identityField, Value: "x"}}})
	if f := fields(t, err); len(f["custom_fields_attributes.0"]) == 0 {
		t.Errorf("errors = %v", f)
	}
}

func TestUpdateClearsWorkflow(t *testing.T) {
	s, _ := newService()
	c := create(t, s, Changes{CompanyName: ptr("A"), InvoiceWorkflowID: ptr(invoiceFlow)})
	got, err := s.Update(context.Background(), adminID, c.ID, owner, Changes{InvoiceWorkflowID: ptr(resource.ID(0))})
	if err != nil || got.InvoiceWorkflowID != nil {
		t.Errorf("workflow = %v, %v", got.InvoiceWorkflowID, err)
	}
	if !got.UpdatedAt.After(c.UpdatedAt) || got.Version() <= c.Version() {
		t.Errorf("version did not move")
	}
}

func TestStrangersSeeNothing(t *testing.T) {
	s, _ := newService()
	c := create(t, s, Changes{CompanyName: ptr("A")})
	if _, err := s.Contact(context.Background(), adminID, c.ID, stranger); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("Contact = %v", err)
	}
	if _, err := s.List(context.Background(), adminID, stranger, Query{}); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("List = %v", err)
	}
	if _, err := s.Contact(context.Background(), adminID, 12345, owner); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing = %v", err)
	}
}

func TestArchivedContactsLeaveLists(t *testing.T) {
	s, _ := newService()
	a := create(t, s, Changes{CompanyName: ptr("Alpha")})
	create(t, s, Changes{CompanyName: ptr("Beta")})
	if _, err := s.Archive(context.Background(), adminID, a.ID, owner); err != nil {
		t.Fatal(err)
	}
	page, _ := s.List(context.Background(), adminID, owner, Query{})
	if page.Total != 1 || page.Contacts[0].CompanyName != "Beta" {
		t.Errorf("active = %+v", page)
	}
	page, _ = s.List(context.Background(), adminID, owner, Query{IncludeArchived: true})
	if page.Total != 2 {
		t.Errorf("with archived = %d", page.Total)
	}
	back, err := s.Update(context.Background(), adminID, a.ID, owner, Changes{Archived: ptr(false)})
	if err != nil || back.Archived {
		t.Errorf("restore = %v, %v", back.Archived, err)
	}
}

func TestListPagesAndSearches(t *testing.T) {
	s, _ := newService()
	for _, name := range []string{"Delta", "alpha", "Charlie", "Bravo"} {
		create(t, s, Changes{CompanyName: ptr(name)})
	}
	create(t, s, Changes{CompanyName: ptr("Echo"), Person: &PersonChanges{Firstname: ptr("Zoë"), Email: ptr("zoe@example.com")}})
	page, err := s.List(context.Background(), adminID, owner, Query{Page: 2, PerPage: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 5 || len(page.Contacts) != 2 || page.Contacts[0].CompanyName != "Charlie" {
		t.Errorf("page 2 = %+v", page)
	}
	page, _ = s.List(context.Background(), adminID, owner, Query{Search: "ZOE@EXAMPLE"})
	if page.Total != 1 || page.Contacts[0].CompanyName != "Echo" {
		t.Errorf("search = %+v", page)
	}
	page, _ = s.List(context.Background(), adminID, owner, Query{PerPage: 1000})
	if page.PerPage != MaxPerPage {
		t.Errorf("per page = %d", page.PerPage)
	}
}

func TestParseFilter(t *testing.T) {
	q, err := ParseFilter("first_name:Ann,contact_type:company,delivery_method:peppol,updated_after:2026-11-01T00:00:00Z,invoice_workflow_id:500,mandate_type:all")
	if err != nil {
		t.Fatal(err)
	}
	if q.FirstName != "Ann" || q.ContactType != "company" || q.DeliveryMethod != "Peppol" || q.UpdatedAfter == nil || *q.InvoiceWorkflowID != invoiceFlow || q.Mandate != "" {
		t.Errorf("query = %+v", q)
	}
	if _, err := ParseFilter("colour:red"); err == nil {
		t.Error("unknown key accepted")
	}
	if _, err := ParseFilter("contact_type:robot"); err == nil {
		t.Error("unknown contact type accepted")
	}
	if _, err := ParseFilter("updated_after:yesterday"); err == nil {
		t.Error("bad time accepted")
	}
}

func TestQueryMatches(t *testing.T) {
	company := Contact{CompanyName: "A", DeliveryMethod: "Email"}
	person := Contact{Firstname: "Ann", Lastname: "Jansen", DeliveryMethod: "Post", SepaActive: true}
	cases := []struct {
		q    Query
		want [2]bool
	}{
		{Query{ContactType: "company"}, [2]bool{true, false}},
		{Query{ContactType: "private_individual"}, [2]bool{false, true}},
		{Query{DeliveryMethod: "Post"}, [2]bool{false, true}},
		{Query{Mandate: "manual"}, [2]bool{false, true}},
		{Query{Mandate: "none"}, [2]bool{true, false}},
		{Query{LastName: "jansen"}, [2]bool{false, true}},
		{Query{Trusted: "not_trusted"}, [2]bool{true, true}},
		{Query{Field: "lastname", Value: "JANSEN"}, [2]bool{false, true}},
	}
	for _, c := range cases {
		if got := [2]bool{c.q.Matches(company), c.q.Matches(person)}; got != c.want {
			t.Errorf("%+v matches %v, want %v", c.q, got, c.want)
		}
	}
}

func TestDeleteArchivesContactsInUse(t *testing.T) {
	s, store := newService()
	used := create(t, s, Changes{CompanyName: ptr("Used")})
	free := create(t, s, Changes{CompanyName: ptr("Free")})
	s.references = fakeReferences{used: map[resource.ID]bool{used.ID: true}}
	if kept, err := s.Delete(context.Background(), adminID, used.ID, owner); err != nil || !kept {
		t.Errorf("delete used = %v, %v", kept, err)
	}
	if got, ok := store.contacts[used.ID]; !ok || !got.Archived {
		t.Errorf("used contact = %+v, want it kept, archived", got)
	}
	if kept, err := s.Delete(context.Background(), adminID, free.ID, owner); err != nil || kept {
		t.Errorf("delete free = %v, %v", kept, err)
	}
	if _, ok := store.contacts[free.ID]; ok {
		t.Error("free contact still stored")
	}
}

func TestDoublesGroupLookalikes(t *testing.T) {
	s, _ := newService()
	a := create(t, s, Changes{CompanyName: ptr("Example Shop"), ChamberOfCommerce: ptr("12345678")})
	b := create(t, s, Changes{CompanyName: ptr("Example Shop B.V."), ChamberOfCommerce: ptr("12345678"), SendInvoicesToEmail: ptr("billing@example.com")})
	c := create(t, s, Changes{CompanyName: ptr("Other"), SendInvoicesToEmail: ptr("BILLING@example.com")})
	create(t, s, Changes{CompanyName: ptr("Alone")})
	groups, err := s.Doubles(context.Background(), adminID, owner, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0]) != 3 {
		t.Fatalf("groups = %+v", groups)
	}
	ids := map[resource.ID]bool{}
	for _, g := range groups[0] {
		ids[g.ID] = true
	}
	if !ids[a.ID] || !ids[b.ID] || !ids[c.ID] {
		t.Errorf("group = %v", ids)
	}
}

func TestContactPeople(t *testing.T) {
	s, _ := newService()
	ctx := context.Background()
	c := create(t, s, Changes{CompanyName: ptr("A")})
	if _, err := s.AddPerson(ctx, adminID, c.ID, owner, PersonChanges{Email: ptr("x@example.com")}); err == nil {
		t.Error("person without a name accepted")
	}
	p, err := s.AddPerson(ctx, adminID, c.ID, owner, PersonChanges{Firstname: ptr("Ann"), Email: ptr("ann@example.com")})
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.UpdatePerson(ctx, adminID, c.ID, p.ID, owner, PersonChanges{Department: ptr("Finance")})
	if err != nil || p.Department != "Finance" || p.Firstname != "Ann" {
		t.Fatalf("update = %+v, %v", p, err)
	}
	got, _ := s.Contact(ctx, adminID, c.ID, owner)
	if len(got.People) != 1 || got.Version() <= c.Version() {
		t.Errorf("contact = %+v", got)
	}
	if err := s.DeletePerson(ctx, adminID, c.ID, p.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Person(ctx, adminID, c.ID, p.ID, owner); !errors.Is(err, ErrPersonNotFound) {
		t.Errorf("deleted person = %v", err)
	}
}

func TestNotesAndTodos(t *testing.T) {
	s, _ := newService()
	ctx := context.Background()
	c := create(t, s, Changes{CompanyName: ptr("A")})
	if _, err := s.AddNote(ctx, adminID, c.ID, owner, NoteInput{Note: " "}); err == nil {
		t.Error("blank note accepted")
	}
	if _, err := s.AddNote(ctx, adminID, c.ID, owner, NoteInput{Note: "Call", Todo: true, AssigneeID: ptr(stranger)}); err == nil {
		t.Error("stranger assigned")
	}
	n, err := s.AddNote(ctx, adminID, c.ID, owner, NoteInput{Note: "Call back", Todo: true, AssigneeID: ptr(colleague)})
	if err != nil {
		t.Fatal(err)
	}
	if n.EntityType != "Contact" || n.EntityID != c.ID || n.UserID != owner {
		t.Errorf("note = %+v", n)
	}
	page, _ := s.List(ctx, adminID, owner, Query{OpenTodos: true})
	if page.Total != 1 {
		t.Errorf("contacts with to-dos = %d", page.Total)
	}
	if err := s.DeleteNote(ctx, adminID, c.ID, n.ID, owner); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteNote(ctx, adminID, c.ID, n.ID, owner); !errors.Is(err, ErrNoteNotFound) {
		t.Errorf("second delete = %v", err)
	}
}

func TestFetchByIDIsCapped(t *testing.T) {
	s, _ := newService()
	ids := make([]resource.ID, MaxFetch+1)
	if _, err := s.ContactsByID(context.Background(), adminID, owner, ids); err == nil {
		t.Error("too many ids accepted")
	}
}
