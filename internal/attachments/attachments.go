// Package attachments keeps the files people attach to records such as
// invoices and journal entries. The record's own service checks access
// before it calls this package.
package attachments

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ErrNotFound reports an attachment that is not on the record.
var ErrNotFound = errors.New("attachment not found")

// MaxSize caps one attachment, as at Moneybird.
const MaxSize = 10 << 20

// ContentTypes are the kinds of file people may attach.
var ContentTypes = []string{"application/pdf", "image/png", "image/jpeg", "image/gif", "image/webp", "application/xml", "text/xml", "text/plain", "text/csv"}

// Attachment is a file on a record.
type Attachment struct {
	ID               resource.ID
	AdministrationID resource.ID
	// EntityType names the kind of record, as Moneybird does: SalesInvoice,
	// GeneralJournalDocument.
	EntityType  string
	EntityID    resource.ID
	Filename    string
	ContentType string
	Size        int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Key is where the file is kept.
func (a Attachment) Key() string {
	return fmt.Sprintf("administrations/%d/attachments/%d", a.AdministrationID, a.ID)
}

// Files keeps file contents.
type Files interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	Open(ctx context.Context, key string) (io.ReadCloser, string, error)
	Delete(ctx context.Context, key string) error
}

// Store persists attachment records.
type Store interface {
	CreateAttachment(ctx context.Context, a Attachment) error
	DeleteAttachment(ctx context.Context, admin, id resource.ID) error
	// AttachmentsOf returns the attachments on the records, oldest first.
	AttachmentsOf(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) ([]Attachment, error)
}

// Service implements the attachment use cases.
type Service struct {
	store Store
	files Files
	ids   *resource.Generator
	now   func() time.Time
}

// NewService returns a Service.
func NewService(store Store, files Files, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{store: store, files: files, ids: ids, now: now}
}

// Add attaches a file to a record. The content type is sniffed when the
// sender gives none or a generic one.
func (s *Service) Add(ctx context.Context, admin resource.ID, entityType string, entity resource.ID, filename, contentType string, data []byte) (Attachment, error) {
	var verr resource.ValidationError
	filename = strings.TrimSpace(path.Base(strings.ReplaceAll(filename, `\`, "/")))
	if filename == "" || filename == "." || filename == "/" {
		filename = "attachment"
	}
	if len(filename) > 255 {
		verr.Add("file", "name is too long (maximum is 255 characters)")
	}
	contentType, _, _ = strings.Cut(strings.ToLower(strings.TrimSpace(contentType)), ";")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType, _, _ = strings.Cut(http.DetectContentType(data), ";")
	}
	switch {
	case len(data) == 0:
		verr.Add("file", "can't be blank")
	case len(data) > MaxSize:
		verr.Add("file", "is too large (maximum is 10 MB)")
	case !slices.Contains(ContentTypes, contentType):
		verr.Add("file", "is not a PDF, image or text file")
	}
	if err := verr.OrNil(); err != nil {
		return Attachment{}, err
	}
	now := s.now()
	a := Attachment{ID: s.ids.Next(), AdministrationID: admin, EntityType: entityType, EntityID: entity, Filename: filename, ContentType: contentType, Size: int64(len(data)), CreatedAt: now, UpdatedAt: now}
	if err := s.files.Put(ctx, a.Key(), contentType, data); err != nil {
		return Attachment{}, fmt.Errorf("store file: %w", err)
	}
	if err := s.store.CreateAttachment(ctx, a); err != nil {
		_ = s.files.Delete(ctx, a.Key())
		return Attachment{}, fmt.Errorf("store attachment: %w", err)
	}
	return a, nil
}

func (s *Service) find(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) (Attachment, error) {
	list, err := s.store.AttachmentsOf(ctx, admin, entityType, []resource.ID{entity})
	if err != nil {
		return Attachment{}, err
	}
	i := slices.IndexFunc(list, func(a Attachment) bool { return a.ID == id })
	if i < 0 {
		return Attachment{}, ErrNotFound
	}
	return list[i], nil
}

// Open returns an attachment and its contents; the caller closes them.
func (s *Service) Open(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) (Attachment, io.ReadCloser, error) {
	a, err := s.find(ctx, admin, entityType, entity, id)
	if err != nil {
		return Attachment{}, nil, err
	}
	body, _, err := s.files.Open(ctx, a.Key())
	if err != nil {
		return Attachment{}, nil, fmt.Errorf("open file: %w", err)
	}
	return a, body, nil
}

// Delete removes an attachment and its file.
func (s *Service) Delete(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) error {
	a, err := s.find(ctx, admin, entityType, entity, id)
	if err != nil {
		return err
	}
	if err := s.store.DeleteAttachment(ctx, admin, a.ID); err != nil {
		return err
	}
	// The record is gone; a file left behind only takes space.
	_ = s.files.Delete(ctx, a.Key())
	return nil
}

// DeleteAll removes every attachment of a record, as when it is deleted.
func (s *Service) DeleteAll(ctx context.Context, admin resource.ID, entityType string, entity resource.ID) error {
	list, err := s.store.AttachmentsOf(ctx, admin, entityType, []resource.ID{entity})
	if err != nil {
		return err
	}
	for _, a := range list {
		if err := s.Delete(ctx, admin, entityType, entity, a.ID); err != nil {
			return err
		}
	}
	return nil
}

// Of returns the attachments on each record.
func (s *Service) Of(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]Attachment, error) {
	out := map[resource.ID][]Attachment{}
	if len(entities) == 0 {
		return out, nil
	}
	list, err := s.store.AttachmentsOf(ctx, admin, entityType, entities)
	if err != nil {
		return nil, fmt.Errorf("load attachments: %w", err)
	}
	for _, a := range list {
		out[a.EntityID] = append(out[a.EntityID], a)
	}
	return out, nil
}
