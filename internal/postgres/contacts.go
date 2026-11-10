package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ContactStore implements contacts.Store.
type ContactStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewContactStore returns a ContactStore on pool.
func NewContactStore(pool *pgxpool.Pool) *ContactStore {
	return &ContactStore{pool: pool, q: sqlc.New(pool)}
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func idOf(v pgtype.Int8) *resource.ID {
	if !v.Valid {
		return nil
	}
	id := resource.ID(v.Int64)
	return &id
}

func listParams(admin resource.ID, q contacts.Query) sqlc.ListContactsParams {
	p := sqlc.ListContactsParams{
		AdministrationID: int64(admin), IncludeArchived: q.IncludeArchived, CreatedAfter: optionalTime(q.CreatedAfter), UpdatedAfter: optionalTime(q.UpdatedAfter),
		FirstName: q.FirstName, LastName: q.LastName, InvoiceWorkflowID: optionalID(q.InvoiceWorkflowID), EstimateWorkflowID: optionalID(q.EstimateWorkflowID),
		ContactType: q.ContactType, DeliveryMethod: q.DeliveryMethod, Trusted: q.Trusted, Mandate: q.Mandate, Field: q.Field, Value: q.Value,
		OpenTodos: q.OpenTodos, Search: likeEscaper.Replace(strings.TrimSpace(q.Search)),
	}
	if q.PerPage > 0 {
		p.MaxRows = pgtype.Int4{Int32: int32(q.PerPage), Valid: true}
		p.Skip = int32((max(q.Page, 1) - 1) * q.PerPage)
	}
	return p
}

// Contacts implements contacts.Store.
func (s *ContactStore) Contacts(ctx context.Context, admin resource.ID, q contacts.Query) ([]contacts.Contact, int, error) {
	rows, err := s.q.ListContacts(ctx, listParams(admin, q))
	if err != nil {
		return nil, 0, err
	}
	list := make([]sqlc.Contact, len(rows))
	total := 0
	for i, r := range rows {
		list[i] = r.Contact
		total = int(r.Total)
	}
	if len(rows) == 0 && q.PerPage > 0 && q.Page > 1 {
		// Past the last page the window has no rows to count with.
		all := listParams(admin, q)
		all.MaxRows, all.Skip = pgtype.Int4{Int32: 1, Valid: true}, 0
		if first, err := s.q.ListContacts(ctx, all); err == nil && len(first) > 0 {
			total = int(first[0].Total)
		}
	}
	out, err := s.complete(ctx, admin, list)
	return out, total, err
}

// Contact implements contacts.Store.
func (s *ContactStore) Contact(ctx context.Context, admin, id resource.ID) (contacts.Contact, error) {
	return s.one(ctx, admin, func() (sqlc.Contact, error) {
		return s.q.ContactByID(ctx, sqlc.ContactByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	})
}

// ContactByCustomerID implements contacts.Store.
func (s *ContactStore) ContactByCustomerID(ctx context.Context, admin resource.ID, customerID string) (contacts.Contact, error) {
	return s.one(ctx, admin, func() (sqlc.Contact, error) {
		return s.q.ContactByCustomerID(ctx, sqlc.ContactByCustomerIDParams{AdministrationID: int64(admin), CustomerID: customerID})
	})
}

func (s *ContactStore) one(ctx context.Context, admin resource.ID, load func() (sqlc.Contact, error)) (contacts.Contact, error) {
	c, err := load()
	if errors.Is(err, pgx.ErrNoRows) {
		return contacts.Contact{}, contacts.ErrNotFound
	}
	if err != nil {
		return contacts.Contact{}, err
	}
	out, err := s.complete(ctx, admin, []sqlc.Contact{c})
	if err != nil {
		return contacts.Contact{}, err
	}
	return out[0], nil
}

// ContactsByID implements contacts.Store.
func (s *ContactStore) ContactsByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]contacts.Contact, error) {
	rows, err := s.q.ContactsByIDs(ctx, sqlc.ContactsByIDsParams{AdministrationID: int64(admin), Ids: ids64(ids)})
	if err != nil {
		return nil, err
	}
	return s.complete(ctx, admin, rows)
}

// Versions implements contacts.Store.
func (s *ContactStore) Versions(ctx context.Context, admin resource.ID, q contacts.Query) ([]contacts.Version, error) {
	q.PerPage = 0
	rows, err := s.q.ListContacts(ctx, listParams(admin, q))
	if err != nil {
		return nil, err
	}
	out := make([]contacts.Version, len(rows))
	for i, r := range rows {
		out[i] = contacts.Version{ID: resource.ID(r.Contact.ID), Version: r.Contact.UpdatedAt.Time.Unix()}
	}
	return out, nil
}

// MaxCustomerNumber implements contacts.Store.
func (s *ContactStore) MaxCustomerNumber(ctx context.Context, admin resource.ID) (int64, error) {
	return s.q.MaxCustomerNumber(ctx, int64(admin))
}

// CustomerIDTaken implements contacts.Store.
func (s *ContactStore) CustomerIDTaken(ctx context.Context, admin resource.ID, customerID string, except resource.ID) (bool, error) {
	return s.q.CustomerIDTaken(ctx, sqlc.CustomerIDTakenParams{AdministrationID: int64(admin), CustomerID: customerID, ID: int64(except)})
}

// CreateContact implements contacts.Store.
func (s *ContactStore) CreateContact(ctx context.Context, c contacts.Contact) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.InsertContact(ctx, sqlc.InsertContactParams{
			ID: int64(c.ID), AdministrationID: int64(c.AdministrationID), CompanyName: c.CompanyName, Firstname: c.Firstname, Lastname: c.Lastname,
			Address1: c.Address1, Address2: c.Address2, Zipcode: c.Zipcode, City: c.City, Country: c.Country, Phone: c.Phone, CustomerID: c.CustomerID,
			TaxNumber: c.TaxNumber, ChamberOfCommerce: c.ChamberOfCommerce, BankAccount: c.BankAccount, DeliveryMethod: c.DeliveryMethod,
			SendInvoicesToAttention: c.SendInvoicesToAttention, SendInvoicesToEmail: c.SendInvoicesToEmail, SendEstimatesToAttention: c.SendEstimatesToAttention,
			SendEstimatesToEmail: c.SendEstimatesToEmail, EmailUbl: c.EmailUBL, DirectDebit: c.DirectDebit, SepaActive: c.SepaActive, SepaIban: c.SepaIBAN,
			SepaIbanAccountName: c.SepaIBANAccountName, SepaBic: c.SepaBIC, SepaMandateID: c.SepaMandateID, SepaMandateDate: optionalDate(c.SepaMandateDate),
			SepaSequenceType: c.SepaSequenceType, SiIdentifierType: c.SIIdentifierType, SiIdentifier: c.SIIdentifier,
			InvoiceWorkflowID: optionalID(c.InvoiceWorkflowID), EstimateWorkflowID: optionalID(c.EstimateWorkflowID), Archived: c.Archived,
			CreatedAt: timestamp(c.CreatedAt), UpdatedAt: timestamp(c.UpdatedAt),
		}); err != nil {
			return err
		}
		if err := q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(c.ID), Type: string(resource.TypeContact), AdministrationID: int64(c.AdministrationID)}); err != nil {
			return err
		}
		for _, p := range c.People {
			if err := upsertPerson(ctx, q, p); err != nil {
				return err
			}
		}
		return saveContactFields(ctx, q, c)
	})
}

// UpdateContact implements contacts.Store.
func (s *ContactStore) UpdateContact(ctx context.Context, c contacts.Contact) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpdateContact(ctx, sqlc.UpdateContactParams{
			AdministrationID: int64(c.AdministrationID), ID: int64(c.ID), CompanyName: c.CompanyName, Firstname: c.Firstname, Lastname: c.Lastname,
			Address1: c.Address1, Address2: c.Address2, Zipcode: c.Zipcode, City: c.City, Country: c.Country, Phone: c.Phone, CustomerID: c.CustomerID,
			TaxNumber: c.TaxNumber, ChamberOfCommerce: c.ChamberOfCommerce, BankAccount: c.BankAccount, DeliveryMethod: c.DeliveryMethod,
			SendInvoicesToAttention: c.SendInvoicesToAttention, SendInvoicesToEmail: c.SendInvoicesToEmail, SendEstimatesToAttention: c.SendEstimatesToAttention,
			SendEstimatesToEmail: c.SendEstimatesToEmail, EmailUbl: c.EmailUBL, DirectDebit: c.DirectDebit, SepaActive: c.SepaActive, SepaIban: c.SepaIBAN,
			SepaIbanAccountName: c.SepaIBANAccountName, SepaBic: c.SepaBIC, SepaMandateID: c.SepaMandateID, SepaMandateDate: optionalDate(c.SepaMandateDate),
			SepaSequenceType: c.SepaSequenceType, SiIdentifierType: c.SIIdentifierType, SiIdentifier: c.SIIdentifier,
			InvoiceWorkflowID: optionalID(c.InvoiceWorkflowID), EstimateWorkflowID: optionalID(c.EstimateWorkflowID), Archived: c.Archived,
			UpdatedAt: timestamp(c.UpdatedAt),
		}); err != nil {
			return err
		}
		return saveContactFields(ctx, q, c)
	})
}

func saveContactFields(ctx context.Context, q *sqlc.Queries, c contacts.Contact) error {
	if err := q.DeleteContactCustomFieldValues(ctx, int64(c.ID)); err != nil {
		return err
	}
	for _, v := range c.CustomFields {
		if err := q.InsertContactCustomFieldValue(ctx, sqlc.InsertContactCustomFieldValueParams{ContactID: int64(c.ID), CustomFieldID: int64(v.FieldID), Value: v.Value}); err != nil {
			return err
		}
	}
	return nil
}

// DeleteContact implements contacts.Store.
func (s *ContactStore) DeleteContact(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteNotesOf(ctx, sqlc.DeleteNotesOfParams{AdministrationID: int64(admin), EntityID: int64(id)}); err != nil {
			return err
		}
		if err := q.DeleteContact(ctx, sqlc.DeleteContactParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

func upsertPerson(ctx context.Context, q *sqlc.Queries, p contacts.Person) error {
	return q.UpsertContactPerson(ctx, sqlc.UpsertContactPersonParams{
		ID: int64(p.ID), AdministrationID: int64(p.AdministrationID), ContactID: int64(p.ContactID), Firstname: p.Firstname, Lastname: p.Lastname,
		Phone: p.Phone, Email: p.Email, Department: p.Department, CreatedAt: timestamp(p.CreatedAt), UpdatedAt: timestamp(p.UpdatedAt),
	})
}

// SavePerson implements contacts.Store.
func (s *ContactStore) SavePerson(ctx context.Context, p contacts.Person) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := upsertPerson(ctx, q, p); err != nil {
			return err
		}
		return q.TouchContact(ctx, sqlc.TouchContactParams{AdministrationID: int64(p.AdministrationID), ID: int64(p.ContactID), UpdatedAt: timestamp(p.UpdatedAt)})
	})
}

// DeletePerson implements contacts.Store.
func (s *ContactStore) DeletePerson(ctx context.Context, admin, contact, id resource.ID, at time.Time) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteContactPerson(ctx, sqlc.DeleteContactPersonParams{AdministrationID: int64(admin), ContactID: int64(contact), ID: int64(id)}); err != nil {
			return err
		}
		return q.TouchContact(ctx, sqlc.TouchContactParams{AdministrationID: int64(admin), ID: int64(contact), UpdatedAt: timestamp(at)})
	})
}

// CreateNote implements contacts.Store.
func (s *ContactStore) CreateNote(ctx context.Context, n contacts.Note) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.InsertNote(ctx, sqlc.InsertNoteParams{
			ID: int64(n.ID), AdministrationID: int64(n.AdministrationID), EntityID: int64(n.EntityID), EntityType: n.EntityType,
			UserID: pgtype.Int8{Int64: int64(n.UserID), Valid: n.UserID != 0}, AssigneeID: optionalID(n.AssigneeID), Todo: n.Todo, Note: n.Note,
			CompletedAt: optionalTime(n.CompletedAt), CompletedByID: optionalID(n.CompletedByID), CreatedAt: timestamp(n.CreatedAt), UpdatedAt: timestamp(n.UpdatedAt),
		}); err != nil {
			return err
		}
		return q.TouchContact(ctx, sqlc.TouchContactParams{AdministrationID: int64(n.AdministrationID), ID: int64(n.EntityID), UpdatedAt: timestamp(n.CreatedAt)})
	})
}

// DeleteNote implements contacts.Store.
func (s *ContactStore) DeleteNote(ctx context.Context, admin, entity, id resource.ID, at time.Time) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteNote(ctx, sqlc.DeleteNoteParams{AdministrationID: int64(admin), EntityID: int64(entity), ID: int64(id)}); err != nil {
			return err
		}
		return q.TouchContact(ctx, sqlc.TouchContactParams{AdministrationID: int64(admin), ID: int64(entity), UpdatedAt: timestamp(at)})
	})
}

// IsMember implements contacts.Store.
func (s *ContactStore) IsMember(ctx context.Context, admin, user resource.ID) (bool, error) {
	return s.q.IsMember(ctx, sqlc.IsMemberParams{AdministrationID: int64(admin), UserID: int64(user)})
}

// complete converts rows and loads their people, custom fields and notes.
func (s *ContactStore) complete(ctx context.Context, admin resource.ID, rows []sqlc.Contact) ([]contacts.Contact, error) {
	out := make([]contacts.Contact, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, len(rows))
	index := map[int64]int{}
	for i, r := range rows {
		ids[i] = r.ID
		index[r.ID] = i
		out[i] = toContact(r)
	}
	people, err := s.q.ContactPeopleOf(ctx, sqlc.ContactPeopleOfParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, p := range people {
		c := &out[index[p.ContactID]]
		c.People = append(c.People, contacts.Person{
			ID: resource.ID(p.ID), AdministrationID: resource.ID(p.AdministrationID), ContactID: resource.ID(p.ContactID), Firstname: p.Firstname,
			Lastname: p.Lastname, Phone: p.Phone, Email: p.Email, Department: p.Department, CreatedAt: p.CreatedAt.Time, UpdatedAt: p.UpdatedAt.Time,
		})
	}
	values, err := s.q.ContactCustomFieldValuesOf(ctx, sqlc.ContactCustomFieldValuesOfParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, v := range values {
		c := &out[index[v.ContactID]]
		c.CustomFields = append(c.CustomFields, contacts.FieldValue{FieldID: resource.ID(v.ID), Name: v.Name, Value: v.Value})
	}
	notes, err := s.q.NotesOf(ctx, sqlc.NotesOfParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, n := range notes {
		i, ok := index[n.EntityID]
		if !ok || n.EntityType != contacts.NoteEntityType {
			continue
		}
		note := contacts.Note{
			ID: resource.ID(n.ID), AdministrationID: resource.ID(n.AdministrationID), EntityID: resource.ID(n.EntityID), EntityType: n.EntityType,
			AssigneeID: idOf(n.AssigneeID), Todo: n.Todo, Note: n.Note, CompletedAt: timeOf(n.CompletedAt), CompletedByID: idOf(n.CompletedByID),
			CreatedAt: n.CreatedAt.Time, UpdatedAt: n.UpdatedAt.Time,
		}
		if n.UserID.Valid {
			note.UserID = resource.ID(n.UserID.Int64)
		}
		out[i].Notes = append(out[i].Notes, note)
	}
	return out, nil
}

func toContact(r sqlc.Contact) contacts.Contact {
	return contacts.Contact{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), CompanyName: r.CompanyName, Firstname: r.Firstname, Lastname: r.Lastname,
		Address1: r.Address1, Address2: r.Address2, Zipcode: r.Zipcode, City: r.City, Country: r.Country, Phone: r.Phone, CustomerID: r.CustomerID,
		TaxNumber: r.TaxNumber, ChamberOfCommerce: r.ChamberOfCommerce, BankAccount: r.BankAccount, DeliveryMethod: r.DeliveryMethod,
		SendInvoicesToAttention: r.SendInvoicesToAttention, SendInvoicesToEmail: r.SendInvoicesToEmail, SendEstimatesToAttention: r.SendEstimatesToAttention,
		SendEstimatesToEmail: r.SendEstimatesToEmail, EmailUBL: r.EmailUbl, DirectDebit: r.DirectDebit, SepaActive: r.SepaActive, SepaIBAN: r.SepaIban,
		SepaIBANAccountName: r.SepaIbanAccountName, SepaBIC: r.SepaBic, SepaMandateID: r.SepaMandateID, SepaMandateDate: dateOf(r.SepaMandateDate),
		SepaSequenceType: r.SepaSequenceType, SIIdentifierType: r.SiIdentifierType, SIIdentifier: r.SiIdentifier,
		InvoiceWorkflowID: idOf(r.InvoiceWorkflowID), EstimateWorkflowID: idOf(r.EstimateWorkflowID), Archived: r.Archived,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}
