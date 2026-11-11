package purchases

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// General is a general document: a record kept for the books that books
// nothing, such as a contract, with an optional reminder.
type General struct {
	ID               resource.ID
	AdministrationID resource.ID
	ContactID        resource.ID
	EntryNumber      int
	Reference        string
	Date             time.Time
	DueDate          *time.Time
	ReminderDate     *time.Time
	ReminderMessage  string
	State            string
	Notes            []notes.Note
	Attachments      []attachments.Attachment
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Ref locates the general document.
func (g General) Ref() resource.Ref {
	return resource.Ref{ID: g.ID, Type: resource.TypeGeneralDocument, AdministrationID: g.AdministrationID}
}

// Version changes whenever the document does.
func (g General) Version() int64 { return g.UpdatedAt.Unix() }

// GeneralChanges holds the fields to set; nil fields stay as they are.
type GeneralChanges struct {
	// ContactID of zero, or RemoveContact, removes the contact.
	ContactID       *resource.ID
	RemoveContact   bool
	Reference       *string
	Date            *string
	DueDate         *string
	ReminderDate    *string
	ReminderMessage *string
}

// CreateGeneral adds a general document, dated today unless given.
func (s *Service) CreateGeneral(ctx context.Context, admin, user resource.ID, c GeneralChanges) (General, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return General{}, err
	}
	number, err := s.store.NextEntryNumber(ctx, admin)
	if err != nil {
		return General{}, wrap("number document", err)
	}
	now := s.now()
	g := General{ID: s.ids.Next(), AdministrationID: admin, EntryNumber: number, Date: s.today(a), State: Saved, CreatedAt: now, UpdatedAt: now}
	if err := s.applyGeneral(ctx, a, user, &g, c); err != nil {
		return General{}, err
	}
	if err := s.store.SaveGeneralDocument(ctx, g, true); err != nil {
		return General{}, wrap("store general document", err)
	}
	return s.GeneralDocument(ctx, admin, g.ID, user)
}

// UpdateGeneral changes a general document.
func (s *Service) UpdateGeneral(ctx context.Context, admin, id, user resource.ID, c GeneralChanges) (General, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return General{}, err
	}
	g, err := s.store.GeneralDocument(ctx, admin, id)
	if err != nil {
		return General{}, err
	}
	if err := s.applyGeneral(ctx, a, user, &g, c); err != nil {
		return General{}, err
	}
	g.UpdatedAt = s.now()
	if err := s.store.SaveGeneralDocument(ctx, g, false); err != nil {
		return General{}, wrap("store general document", err)
	}
	return s.GeneralDocument(ctx, admin, id, user)
}

func parseDay(verr *resource.ValidationError, field string, raw *string, dst **time.Time) {
	if raw == nil {
		return
	}
	*dst = nil
	if v := strings.TrimSpace(*raw); v != "" {
		d, err := time.Parse(time.DateOnly, v)
		if err != nil {
			verr.Add(field, "is not a date")
			return
		}
		*dst = &d
	}
}

func (s *Service) applyGeneral(ctx context.Context, a administration.Listed, user resource.ID, g *General, c GeneralChanges) error {
	var verr resource.ValidationError
	if c.ContactID != nil {
		g.ContactID = *c.ContactID
	}
	if c.RemoveContact {
		g.ContactID = 0
	}
	if g.ContactID != 0 && c.ContactID != nil {
		if _, err := s.contacts.Contact(ctx, a.ID, g.ContactID, user); err != nil {
			verr.Add("contact_id", "does not exist")
		}
	}
	if c.Reference != nil {
		if g.Reference = strings.TrimSpace(*c.Reference); len(g.Reference) > 1000 {
			verr.Add("reference", "is too long (maximum is 1000 characters)")
		}
	}
	if c.Date != nil {
		if d, err := time.Parse(time.DateOnly, strings.TrimSpace(*c.Date)); err != nil {
			verr.Add("date", "is not a date")
		} else {
			g.Date = d
		}
	}
	parseDay(&verr, "due_date", c.DueDate, &g.DueDate)
	parseDay(&verr, "reminder_date", c.ReminderDate, &g.ReminderDate)
	if c.ReminderMessage != nil {
		if g.ReminderMessage = strings.TrimSpace(*c.ReminderMessage); len(g.ReminderMessage) > 10000 {
			verr.Add("reminder.message", "is too long (maximum is 10000 characters)")
		}
	}
	return verr.OrNil()
}

// DeleteGeneral removes a general document with its files and notes.
func (s *Service) DeleteGeneral(ctx context.Context, admin, id, user resource.ID) error {
	if _, err := s.GeneralDocument(ctx, admin, id, user); err != nil {
		return err
	}
	if err := s.attachments.DeleteAll(ctx, admin, GeneralDocument, id); err != nil {
		return wrap("delete attachments", err)
	}
	return wrap("delete general document", s.store.DeleteGeneralDocument(ctx, admin, id))
}

// GeneralList returns the page of general documents the query selects.
func (s *Service) GeneralList(ctx context.Context, admin, user resource.ID, q Query) ([]General, int, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, 0, err
	}
	q.Today = s.today(a)
	pageOf(&q)
	list, total, err := s.store.GeneralDocuments(ctx, admin, q)
	if err != nil {
		return nil, 0, fmt.Errorf("list general documents: %w", err)
	}
	list, err = s.completeGeneral(ctx, admin, list)
	return list, total, err
}

// GeneralVersions lists the general documents the query selects, for
// synchronization.
func (s *Service) GeneralVersions(ctx context.Context, admin, user resource.ID, q Query) ([]General, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	q.Today, q.PerPage = s.today(a), 0
	list, _, err := s.store.GeneralDocuments(ctx, admin, q)
	return list, err
}

// GeneralDocumentsByID returns the general documents with these ids.
func (s *Service) GeneralDocumentsByID(ctx context.Context, admin, user resource.ID, ids []resource.ID) ([]General, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	if err := checkIDs(ids); err != nil {
		return nil, err
	}
	list, err := s.store.GeneralDocumentsByID(ctx, admin, ids)
	if err != nil {
		return nil, err
	}
	return s.completeGeneral(ctx, admin, list)
}

// GeneralDocument returns one general document.
func (s *Service) GeneralDocument(ctx context.Context, admin, id, user resource.ID) (General, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return General{}, err
	}
	g, err := s.store.GeneralDocument(ctx, admin, id)
	if err != nil {
		return General{}, err
	}
	list, err := s.completeGeneral(ctx, admin, []General{g})
	if err != nil {
		return General{}, err
	}
	return list[0], nil
}

func (s *Service) completeGeneral(ctx context.Context, admin resource.ID, list []General) ([]General, error) {
	ids := make([]resource.ID, len(list))
	for i, g := range list {
		ids[i] = g.ID
	}
	byDoc, err := s.notes.Of(ctx, admin, GeneralDocument, ids)
	if err != nil {
		return nil, err
	}
	files, err := s.attachments.Of(ctx, admin, GeneralDocument, ids)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Notes, list[i].Attachments = byDoc[list[i].ID], files[list[i].ID]
	}
	return list, nil
}

// AddGeneralNote adds a note to a general document.
func (s *Service) AddGeneralNote(ctx context.Context, admin, id, user resource.ID, in notes.Input) (notes.Note, error) {
	if _, err := s.GeneralDocument(ctx, admin, id, user); err != nil {
		return notes.Note{}, err
	}
	n, err := s.notes.Add(ctx, admin, GeneralDocument, id, user, in)
	if err != nil {
		return notes.Note{}, err
	}
	return n, s.store.TouchGeneralDocument(ctx, admin, id, n.CreatedAt)
}

// DeleteGeneralNote removes a note from a general document.
func (s *Service) DeleteGeneralNote(ctx context.Context, admin, id, noteID, user resource.ID) error {
	if _, err := s.GeneralDocument(ctx, admin, id, user); err != nil {
		return err
	}
	if err := s.notes.Delete(ctx, admin, GeneralDocument, id, noteID); err != nil {
		return err
	}
	return s.store.TouchGeneralDocument(ctx, admin, id, s.now())
}

// AddGeneralAttachment attaches a file to a general document.
func (s *Service) AddGeneralAttachment(ctx context.Context, admin, id, user resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error) {
	if _, err := s.GeneralDocument(ctx, admin, id, user); err != nil {
		return attachments.Attachment{}, err
	}
	a, err := s.attachments.Add(ctx, admin, GeneralDocument, id, filename, contentType, data)
	if err != nil {
		return attachments.Attachment{}, err
	}
	return a, s.store.TouchGeneralDocument(ctx, admin, id, a.CreatedAt)
}

// OpenGeneralAttachment returns an attachment of a general document; the
// caller closes it.
func (s *Service) OpenGeneralAttachment(ctx context.Context, admin, id, attachment, user resource.ID) (attachments.Attachment, io.ReadCloser, error) {
	if _, err := s.GeneralDocument(ctx, admin, id, user); err != nil {
		return attachments.Attachment{}, nil, err
	}
	return s.attachments.Open(ctx, admin, GeneralDocument, id, attachment)
}

// DeleteGeneralAttachment removes an attachment from a general document.
func (s *Service) DeleteGeneralAttachment(ctx context.Context, admin, id, attachment, user resource.ID) error {
	if _, err := s.GeneralDocument(ctx, admin, id, user); err != nil {
		return err
	}
	if err := s.attachments.Delete(ctx, admin, GeneralDocument, id, attachment); err != nil {
		return err
	}
	return s.store.TouchGeneralDocument(ctx, admin, id, s.now())
}
