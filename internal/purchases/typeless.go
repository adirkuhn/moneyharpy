package purchases

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Typeless is a typeless document: an upload, such as a scanned bill,
// still to be processed into a purchase invoice, receipt or other
// document.
type Typeless struct {
	ID               resource.ID
	AdministrationID resource.ID
	ContactID        resource.ID
	Reference        string
	Date             time.Time
	Origin           string
	State            string
	Attachments      []attachments.Attachment
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Ref locates the typeless document.
func (t Typeless) Ref() resource.Ref {
	return resource.Ref{ID: t.ID, Type: resource.TypeTypelessDocument, AdministrationID: t.AdministrationID}
}

// Version changes whenever the document does.
func (t Typeless) Version() int64 { return t.UpdatedAt.Unix() }

// TypelessChanges holds the fields of a new typeless document.
type TypelessChanges struct {
	ContactID *resource.ID
	Reference *string
	Date      *string
}

// CreateTypeless adds a typeless document, dated today unless given.
func (s *Service) CreateTypeless(ctx context.Context, admin, user resource.ID, c TypelessChanges) (Typeless, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Typeless{}, err
	}
	now := s.now()
	t := Typeless{ID: s.ids.Next(), AdministrationID: admin, Date: s.today(a), Origin: OriginAPI, State: New, CreatedAt: now, UpdatedAt: now}
	var verr resource.ValidationError
	if c.ContactID != nil && *c.ContactID != 0 {
		if _, err := s.contacts.Contact(ctx, admin, *c.ContactID, user); err != nil {
			verr.Add("contact_id", "does not exist")
		}
		t.ContactID = *c.ContactID
	}
	if c.Reference != nil {
		if t.Reference = strings.TrimSpace(*c.Reference); len(t.Reference) > 1000 {
			verr.Add("reference", "is too long (maximum is 1000 characters)")
		}
	}
	if c.Date != nil {
		if d, err := time.Parse(time.DateOnly, strings.TrimSpace(*c.Date)); err != nil {
			verr.Add("date", "is not a date")
		} else {
			t.Date = d
		}
	}
	if err := verr.OrNil(); err != nil {
		return Typeless{}, err
	}
	if err := s.store.SaveTypelessDocument(ctx, t, true); err != nil {
		return Typeless{}, wrap("store typeless document", err)
	}
	return s.TypelessDocument(ctx, admin, t.ID, user)
}

// Upload adds a typeless document holding the file, to process later.
func (s *Service) Upload(ctx context.Context, admin, user resource.ID, filename, contentType string, data []byte) (Typeless, error) {
	t, err := s.CreateTypeless(ctx, admin, user, TypelessChanges{})
	if err != nil {
		return Typeless{}, err
	}
	t.Origin = OriginUpload
	if err := s.store.SaveTypelessDocument(ctx, t, false); err != nil {
		return Typeless{}, wrap("store typeless document", err)
	}
	if _, err := s.attachments.Add(ctx, admin, TypelessDocument, t.ID, filename, contentType, data); err != nil {
		if derr := s.store.DeleteTypelessDocument(ctx, admin, t.ID); derr != nil {
			return Typeless{}, fmt.Errorf("%w; remove the document again: %w", err, derr)
		}
		return Typeless{}, err
	}
	if err := s.queue(ctx, admin, t.ID, user); err != nil {
		return Typeless{}, err
	}
	return s.TypelessDocument(ctx, admin, t.ID, user)
}

// DeleteTypeless removes a typeless document with its files.
func (s *Service) DeleteTypeless(ctx context.Context, admin, id, user resource.ID) error {
	if _, err := s.TypelessDocument(ctx, admin, id, user); err != nil {
		return err
	}
	if err := s.attachments.DeleteAll(ctx, admin, TypelessDocument, id); err != nil {
		return wrap("delete attachments", err)
	}
	if err := s.store.DeleteTypelessDocument(ctx, admin, id); err != nil {
		return wrap("delete typeless document", err)
	}
	return s.forget(ctx, admin, id)
}

// TypelessList returns the page of typeless documents the query selects.
func (s *Service) TypelessList(ctx context.Context, admin, user resource.ID, q Query) ([]Typeless, int, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, 0, err
	}
	q.Today = s.today(a)
	pageOf(&q)
	list, total, err := s.store.TypelessDocuments(ctx, admin, q)
	if err != nil {
		return nil, 0, fmt.Errorf("list typeless documents: %w", err)
	}
	list, err = s.completeTypeless(ctx, admin, list)
	return list, total, err
}

// TypelessVersions lists the typeless documents the query selects, for
// synchronization.
func (s *Service) TypelessVersions(ctx context.Context, admin, user resource.ID, q Query) ([]Typeless, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	q.Today, q.PerPage = s.today(a), 0
	list, _, err := s.store.TypelessDocuments(ctx, admin, q)
	return list, err
}

// TypelessDocumentsByID returns the typeless documents with these ids.
func (s *Service) TypelessDocumentsByID(ctx context.Context, admin, user resource.ID, ids []resource.ID) ([]Typeless, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	if err := checkIDs(ids); err != nil {
		return nil, err
	}
	list, err := s.store.TypelessDocumentsByID(ctx, admin, ids)
	if err != nil {
		return nil, err
	}
	return s.completeTypeless(ctx, admin, list)
}

// TypelessDocument returns one typeless document.
func (s *Service) TypelessDocument(ctx context.Context, admin, id, user resource.ID) (Typeless, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Typeless{}, err
	}
	t, err := s.store.TypelessDocument(ctx, admin, id)
	if err != nil {
		return Typeless{}, err
	}
	list, err := s.completeTypeless(ctx, admin, []Typeless{t})
	if err != nil {
		return Typeless{}, err
	}
	return list[0], nil
}

func (s *Service) completeTypeless(ctx context.Context, admin resource.ID, list []Typeless) ([]Typeless, error) {
	ids := make([]resource.ID, len(list))
	for i, t := range list {
		ids[i] = t.ID
	}
	files, err := s.attachments.Of(ctx, admin, TypelessDocument, ids)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Attachments = files[list[i].ID]
	}
	return list, nil
}

// AddTypelessAttachment attaches a file to a typeless document, to be read
// by Scan and Recognise.
func (s *Service) AddTypelessAttachment(ctx context.Context, admin, id, user resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error) {
	if _, err := s.TypelessDocument(ctx, admin, id, user); err != nil {
		return attachments.Attachment{}, err
	}
	a, err := s.attachments.Add(ctx, admin, TypelessDocument, id, filename, contentType, data)
	if err != nil {
		return attachments.Attachment{}, err
	}
	if err := s.store.TouchTypelessDocument(ctx, admin, id, a.CreatedAt); err != nil {
		return a, err
	}
	return a, s.queue(ctx, admin, id, user)
}

// OpenTypelessAttachment returns an attachment of a typeless document; the
// caller closes it.
func (s *Service) OpenTypelessAttachment(ctx context.Context, admin, id, attachment, user resource.ID) (attachments.Attachment, io.ReadCloser, error) {
	if _, err := s.TypelessDocument(ctx, admin, id, user); err != nil {
		return attachments.Attachment{}, nil, err
	}
	return s.attachments.Open(ctx, admin, TypelessDocument, id, attachment)
}

// DeleteTypelessAttachment removes an attachment from a typeless document.
func (s *Service) DeleteTypelessAttachment(ctx context.Context, admin, id, attachment, user resource.ID) error {
	if _, err := s.TypelessDocument(ctx, admin, id, user); err != nil {
		return err
	}
	if err := s.attachments.Delete(ctx, admin, TypelessDocument, id, attachment); err != nil {
		return err
	}
	return s.store.TouchTypelessDocument(ctx, admin, id, s.now())
}
