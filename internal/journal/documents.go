package journal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ErrDocumentNotFound reports a journal entry outside the administration.
var ErrDocumentNotFound = errors.New("general journal document not found")

// JournalTypes are the kinds of journal entry besides an ordinary one.
var JournalTypes = []string{"fiscal_year_ending"}

// Notes keeps notes on journal entries.
type Notes interface {
	Add(ctx context.Context, admin resource.ID, entityType string, entity, user resource.ID, in notes.Input) (notes.Note, error)
	Delete(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) error
	Of(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]notes.Note, error)
}

// Attachments keeps files on journal entries.
type Attachments interface {
	Add(ctx context.Context, admin resource.ID, entityType string, entity resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error)
	Open(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) (attachments.Attachment, io.ReadCloser, error)
	Delete(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) error
	DeleteAll(ctx context.Context, admin resource.ID, entityType string, entity resource.ID) error
	Of(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]attachments.Attachment, error)
}

// Document is a journal entry made by hand, such as a correction or the
// closing of a fiscal year; Moneybird calls it a general journal document.
type Document struct {
	ID               resource.ID
	AdministrationID resource.ID
	Reference        string
	Date             time.Time
	// JournalType is "" or fiscal_year_ending.
	JournalType string
	Rows        []Row
	Notes       []notes.Note
	Attachments []attachments.Attachment
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Ref locates the journal entry.
func (d Document) Ref() resource.Ref {
	return resource.Ref{ID: d.ID, Type: resource.TypeGeneralJournalDocument, AdministrationID: d.AdministrationID}
}

// Version changes whenever the journal entry does.
func (d Document) Version() int64 { return d.UpdatedAt.Unix() }

// Entries are what the journal entry posts.
func (d Document) Entries() []Entry {
	out := make([]Entry, len(d.Rows))
	for i, r := range d.Rows {
		out[i] = Entry{
			ID: r.ID, AdministrationID: d.AdministrationID, DocumentType: GeneralJournalDocument, DocumentID: d.ID, Date: d.Date,
			LedgerAccountID: r.LedgerAccountID, ContactID: r.ContactID, ProjectID: r.ProjectID, Amount: r.Debit - r.Credit,
			Description: r.Description, RowOrder: r.RowOrder,
		}
	}
	return out
}

// Row books an amount on one category, on its debit or its credit side.
type Row struct {
	ID              resource.ID
	LedgerAccountID resource.ID
	ContactID       *resource.ID
	ProjectID       *resource.ID
	Debit, Credit   money.Amount
	Description     string
	RowOrder        int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// RowInput changes, adds or removes a row. A row without ID is new.
type RowInput struct {
	ID              *resource.ID
	LedgerAccountID *resource.ID
	// ContactID of zero removes the contact.
	ContactID     *resource.ID
	Debit, Credit *string
	Description   *string
	RowOrder      *int
	Destroy       bool
}

// DocumentChanges holds the fields to set; nil fields stay as they are.
type DocumentChanges struct {
	Reference   *string
	Date        *string
	JournalType *string
	Rows        []RowInput
}

// DocumentQuery selects journal entries, after Moneybird's filter.
type DocumentQuery struct {
	Period          *money.Period
	Reference       string
	ContactID       *resource.ID
	LedgerAccountID *resource.ID
	UpdatedAfter    *time.Time
	// Attachment is with or without; empty means all.
	Attachment string
	// Ascending lists the oldest first; by default the newest come first.
	Ascending     bool
	Page, PerPage int
}

// DocumentStore persists journal entries with what they post.
type DocumentStore interface {
	Documents(ctx context.Context, admin resource.ID, q DocumentQuery) ([]Document, int, error)
	Document(ctx context.Context, admin, id resource.ID) (Document, error)
	DocumentsByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]Document, error)
	// CreateDocument stores the document and posts its entries at once.
	CreateDocument(ctx context.Context, d Document) error
	// UpdateDocument replaces the document's rows and entries at once.
	UpdateDocument(ctx context.Context, d Document) error
	DeleteDocument(ctx context.Context, admin, id resource.ID) error
	TouchDocument(ctx context.Context, admin, id resource.ID, at time.Time) error
}

// ParseDocumentFilter reads Moneybird's filter for journal entries. Without
// a filter the list holds this year's entries.
func ParseDocumentFilter(filter string, now time.Time) (DocumentQuery, error) {
	var q DocumentQuery
	if strings.TrimSpace(filter) == "" {
		filter = "period:this_year"
	}
	var verr resource.ValidationError
	for term := range strings.SplitSeq(filter, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(term), ":")
		if term = strings.TrimSpace(term); term == "" {
			continue
		}
		if !ok {
			verr.Add("filter", "has a term without a value: "+term)
			continue
		}
		switch key {
		case "period":
			p, err := money.ParsePeriod(value, now)
			if err != nil {
				verr.Add("filter", "period "+err.Error())
				continue
			}
			q.Period = &p
		case "reference":
			q.Reference = value
		case "contact_id", "ledger_account_id":
			id, err := resource.ParseID(value)
			if err != nil {
				verr.Add("filter", key+" is not an id")
				continue
			}
			if key == "contact_id" {
				q.ContactID = &id
			} else {
				q.LedgerAccountID = &id
			}
		case "updated_after":
			t, err := time.Parse(time.RFC3339, value)
			if err != nil {
				verr.Add("filter", "updated_after is not an ISO 8601 time")
				continue
			}
			q.UpdatedAfter = &t
		case "attachment":
			switch value {
			case "all":
			case "with", "without":
				q.Attachment = value
			default:
				verr.Add("filter", "attachment is not one of all, with, without")
			}
		case "state", "recurring":
			// Journal entries have no states and do not recur; all of them match.
		default:
			verr.Add("filter", "has an unknown key: "+key)
		}
	}
	return q, verr.OrNil()
}

// Documents lists the journal entries the query selects.
func (s *Service) Documents(ctx context.Context, admin, user resource.ID, q DocumentQuery) ([]Document, int, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, 0, err
	}
	q.Page = max(q.Page, 1)
	if q.PerPage <= 0 {
		q.PerPage = 50
	}
	q.PerPage = min(q.PerPage, MaxPerPage)
	list, total, err := s.documents.Documents(ctx, admin, q)
	if err != nil {
		return nil, 0, fmt.Errorf("list journal entries: %w", err)
	}
	list, err = s.complete(ctx, admin, list)
	return list, total, err
}

// DocumentVersions lists the ids and versions of the journal entries the
// query selects.
func (s *Service) DocumentVersions(ctx context.Context, admin, user resource.ID, q DocumentQuery) ([]Document, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	list, _, err := s.documents.Documents(ctx, admin, q)
	return list, err
}

// DocumentsByID returns the journal entries with these ids.
func (s *Service) DocumentsByID(ctx context.Context, admin, user resource.ID, ids []resource.ID) ([]Document, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	if len(ids) > MaxPerPage {
		return nil, invalid("ids", fmt.Sprintf("can hold at most %d ids", MaxPerPage))
	}
	list, err := s.documents.DocumentsByID(ctx, admin, ids)
	if err != nil {
		return nil, err
	}
	return s.complete(ctx, admin, list)
}

// Document returns one journal entry.
func (s *Service) Document(ctx context.Context, admin, id, user resource.ID) (Document, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Document{}, err
	}
	d, err := s.documents.Document(ctx, admin, id)
	if err != nil {
		return Document{}, err
	}
	list, err := s.complete(ctx, admin, []Document{d})
	if err != nil {
		return Document{}, err
	}
	return list[0], nil
}

func (s *Service) complete(ctx context.Context, admin resource.ID, list []Document) ([]Document, error) {
	ids := make([]resource.ID, len(list))
	for i, d := range list {
		ids[i] = d.ID
	}
	byDoc, err := s.notes.Of(ctx, admin, GeneralJournalDocument, ids)
	if err != nil {
		return nil, err
	}
	files, err := s.attachments.Of(ctx, admin, GeneralJournalDocument, ids)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Notes, list[i].Attachments = byDoc[list[i].ID], files[list[i].ID]
	}
	return list, nil
}

// CreateDocument adds a journal entry and posts it.
func (s *Service) CreateDocument(ctx context.Context, admin, user resource.ID, c DocumentChanges) (Document, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Document{}, err
	}
	now := s.now()
	d := Document{ID: s.ids.Next(), AdministrationID: admin, CreatedAt: now, UpdatedAt: now}
	if err := s.apply(ctx, admin, user, &d, c, nil); err != nil {
		return Document{}, err
	}
	if err := s.documents.CreateDocument(ctx, d); err != nil {
		return Document{}, fmt.Errorf("store journal entry: %w", err)
	}
	return d, nil
}

// UpdateDocument changes a journal entry and reposts it.
func (s *Service) UpdateDocument(ctx context.Context, admin, id, user resource.ID, c DocumentChanges) (Document, error) {
	d, err := s.Document(ctx, admin, id, user)
	if err != nil {
		return Document{}, err
	}
	was := d.Date
	if err := s.apply(ctx, admin, user, &d, c, &was); err != nil {
		return Document{}, err
	}
	d.UpdatedAt = s.now()
	if err := s.documents.UpdateDocument(ctx, d); err != nil {
		return Document{}, fmt.Errorf("store journal entry: %w", err)
	}
	return d, nil
}

// DeleteDocument removes a journal entry with what it posted.
func (s *Service) DeleteDocument(ctx context.Context, admin, id, user resource.ID) error {
	d, err := s.Document(ctx, admin, id, user)
	if err != nil {
		return err
	}
	if err := s.checkUnlocked(ctx, admin, user, d.Date, nil); err != nil {
		return err
	}
	if err := s.attachments.DeleteAll(ctx, admin, GeneralJournalDocument, id); err != nil {
		return fmt.Errorf("delete attachments: %w", err)
	}
	return s.documents.DeleteDocument(ctx, admin, id)
}

// AddDocumentNote adds a note to a journal entry.
func (s *Service) AddDocumentNote(ctx context.Context, admin, id, user resource.ID, in notes.Input) (notes.Note, error) {
	if _, err := s.Document(ctx, admin, id, user); err != nil {
		return notes.Note{}, err
	}
	n, err := s.notes.Add(ctx, admin, GeneralJournalDocument, id, user, in)
	if err != nil {
		return notes.Note{}, err
	}
	return n, s.documents.TouchDocument(ctx, admin, id, n.CreatedAt)
}

// DeleteDocumentNote removes a note from a journal entry.
func (s *Service) DeleteDocumentNote(ctx context.Context, admin, id, noteID, user resource.ID) error {
	if _, err := s.Document(ctx, admin, id, user); err != nil {
		return err
	}
	if err := s.notes.Delete(ctx, admin, GeneralJournalDocument, id, noteID); err != nil {
		return err
	}
	return s.documents.TouchDocument(ctx, admin, id, s.now())
}

// AddDocumentAttachment attaches a file to a journal entry.
func (s *Service) AddDocumentAttachment(ctx context.Context, admin, id, user resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error) {
	if _, err := s.Document(ctx, admin, id, user); err != nil {
		return attachments.Attachment{}, err
	}
	a, err := s.attachments.Add(ctx, admin, GeneralJournalDocument, id, filename, contentType, data)
	if err != nil {
		return attachments.Attachment{}, err
	}
	return a, s.documents.TouchDocument(ctx, admin, id, a.CreatedAt)
}

// OpenDocumentAttachment returns an attachment of a journal entry; the
// caller closes it.
func (s *Service) OpenDocumentAttachment(ctx context.Context, admin, id, attachment, user resource.ID) (attachments.Attachment, io.ReadCloser, error) {
	if _, err := s.Document(ctx, admin, id, user); err != nil {
		return attachments.Attachment{}, nil, err
	}
	return s.attachments.Open(ctx, admin, GeneralJournalDocument, id, attachment)
}

// DeleteDocumentAttachment removes an attachment from a journal entry.
func (s *Service) DeleteDocumentAttachment(ctx context.Context, admin, id, attachment, user resource.ID) error {
	if _, err := s.Document(ctx, admin, id, user); err != nil {
		return err
	}
	if err := s.attachments.Delete(ctx, admin, GeneralJournalDocument, id, attachment); err != nil {
		return err
	}
	return s.documents.TouchDocument(ctx, admin, id, s.now())
}

// checkUnlocked refuses days in the locked period: the new date and, when
// a document moves or goes, its old one.
func (s *Service) checkUnlocked(ctx context.Context, admin, user resource.ID, day time.Time, was *time.Time) error {
	until, err := s.ledger.LockedUntil(ctx, admin, user)
	if err != nil || until == "" {
		return err
	}
	lock, err := time.Parse(time.DateOnly, until)
	if err != nil {
		return fmt.Errorf("read locked period: %w", err)
	}
	for _, d := range []*time.Time{&day, was} {
		if d != nil && !d.After(lock) {
			return invalid("date", "is in the locked period, which ends on "+until)
		}
	}
	return nil
}

const rowsField = "general_journal_document_entries_attributes"

func (s *Service) apply(ctx context.Context, admin, user resource.ID, d *Document, c DocumentChanges, was *time.Time) error {
	var verr resource.ValidationError
	if c.Reference != nil {
		d.Reference = strings.TrimSpace(*c.Reference)
	}
	if c.JournalType != nil {
		d.JournalType = strings.TrimSpace(*c.JournalType)
	}
	if c.Date != nil {
		day, err := time.Parse(time.DateOnly, strings.TrimSpace(*c.Date))
		if err != nil {
			verr.Add("date", "is not a date")
		} else {
			d.Date = day
		}
	}
	switch {
	case d.Reference == "":
		verr.Add("reference", "can't be blank")
	case len(d.Reference) > 255:
		verr.Add("reference", "is too long (maximum is 255 characters)")
	}
	if d.JournalType != "" && !slices.Contains(JournalTypes, d.JournalType) {
		verr.Add("journal_type", "is not included in the list")
	}
	if d.Date.IsZero() && verr.Fields["date"] == nil {
		verr.Add("date", "can't be blank")
	}
	if c.Rows != nil {
		s.applyRows(ctx, admin, user, d, c.Rows, &verr)
	}
	if len(d.Rows) == 0 {
		verr.Add(rowsField, "needs at least two rows")
	}
	if err := verr.OrNil(); err != nil {
		return err
	}
	var debit, credit money.Amount
	for _, r := range d.Rows {
		debit, credit = debit+r.Debit, credit+r.Credit
	}
	if debit != credit {
		return invalid("base", fmt.Sprintf("Debit (%s) and credit (%s) must be equal", debit, credit))
	}
	if err := Balance(d.Entries()); err != nil {
		return err
	}
	return s.checkUnlocked(ctx, admin, user, d.Date, was)
}

func (s *Service) applyRows(ctx context.Context, admin, user resource.ID, d *Document, inputs []RowInput, verr *resource.ValidationError) {
	now := s.now()
	rows := slices.Clone(d.Rows)
	for i, in := range inputs {
		field := fmt.Sprintf("%s.%d", rowsField, i)
		var r *Row
		if in.ID != nil {
			at := slices.IndexFunc(rows, func(r Row) bool { return r.ID == *in.ID })
			if at < 0 {
				verr.Add(field+".id", "is not a row of this journal entry")
				continue
			}
			if in.Destroy {
				rows = slices.Delete(rows, at, at+1)
				continue
			}
			r = &rows[at]
			r.UpdatedAt = now
		} else {
			if in.Destroy {
				continue
			}
			rows = append(rows, Row{ID: s.ids.Next(), RowOrder: len(rows) + 1, CreatedAt: now, UpdatedAt: now})
			r = &rows[len(rows)-1]
		}
		if in.LedgerAccountID != nil {
			r.LedgerAccountID = *in.LedgerAccountID
		}
		if in.ContactID != nil {
			r.ContactID = nil
			if *in.ContactID != 0 {
				id := *in.ContactID
				r.ContactID = &id
			}
		}
		if in.Description != nil {
			r.Description = strings.TrimSpace(*in.Description)
		}
		if in.RowOrder != nil {
			r.RowOrder = *in.RowOrder
		}
		for _, side := range []struct {
			in  *string
			out *money.Amount
			key string
		}{{in.Debit, &r.Debit, "debit"}, {in.Credit, &r.Credit, "credit"}} {
			if side.in == nil {
				continue
			}
			if strings.TrimSpace(*side.in) == "" {
				*side.out = 0
				continue
			}
			a, err := money.ParseAmount(*side.in)
			if err != nil || a < 0 {
				verr.Add(field+"."+side.key, "is not a positive amount")
				continue
			}
			*side.out = a
		}
	}
	for i, r := range rows {
		field := fmt.Sprintf("%s.%d", rowsField, i)
		switch {
		case r.Debit != 0 && r.Credit != 0:
			verr.Add(field, "has both a debit and a credit amount")
		case r.Debit == 0 && r.Credit == 0:
			verr.Add(field, "needs a debit or a credit amount")
		}
		if len(r.Description) > 1000 {
			verr.Add(field+".description", "is too long (maximum is 1000 characters)")
		}
		if r.LedgerAccountID == 0 {
			verr.Add(field+".ledger_account_id", "can't be blank")
		} else if problem, err := s.ledger.Bookable(ctx, admin, user, r.LedgerAccountID, "general_journal_document"); err != nil || problem != "" {
			verr.Add(field+".ledger_account_id", cmpString(problem, "could not be checked"))
		}
		if r.ContactID != nil {
			if err := s.contacts.ContactExists(ctx, admin, *r.ContactID, user); err != nil {
				verr.Add(field+".contact_id", "does not exist")
			}
		}
	}
	slices.SortStableFunc(rows, func(a, b Row) int { return a.RowOrder - b.RowOrder })
	d.Rows = rows
}

func cmpString(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// IsNotFound reports whether err means a journal resource is missing.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrDocumentNotFound) || errors.Is(err, notes.ErrNotFound) || errors.Is(err, attachments.ErrNotFound)
}
