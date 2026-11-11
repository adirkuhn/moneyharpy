// Package downloads keeps the files exports make, after Moneybird's
// downloads: an export registers its file here and people fetch it later
// through a temporary link. Who may see a download depends on its type.
package downloads

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Errors returned by Service.
var (
	ErrNotFound = errors.New("download not found")
	// ErrUnknownType reports a download type no export registers.
	ErrUnknownType = errors.New("download type is not known")
	// ErrFailed reports a download whose export failed; it has no file.
	ErrFailed = errors.New("download failed")
)

// Paging, as at Moneybird.
const (
	DefaultPerPage = 25
	MaxPerPage     = 100
)

// Download types MoneyHarpy knows, with Moneybird's names.
const (
	Auditfile                 = "auditfile"
	Brugstaat                 = "brugstaat"
	ExportLedgerAccountReport = "export_ledger_account_report"
	ExportDocuments           = "export_documents"
	ExportSalesInvoices       = "export_sales_invoices"
	ExportContacts            = "export_contacts"
)

var (
	typesMu sync.RWMutex
	// types maps each download type to the permissions that open it: any
	// one of them does, as for the operation that made it.
	types = map[string][]administration.Permission{
		Auditfile:                 {administration.Bank, administration.Documents, administration.Estimates, administration.SalesInvoices, administration.Settings},
		Brugstaat:                 {administration.Bank, administration.Documents, administration.SalesInvoices},
		ExportLedgerAccountReport: {administration.Documents, administration.SalesInvoices},
		ExportDocuments:           {administration.Documents},
		ExportSalesInvoices:       {administration.SalesInvoices},
		ExportContacts:            {administration.Estimates, administration.SalesInvoices, administration.Documents, administration.Bank, administration.Settings},
	}
)

// RegisterType adds a download type, opened by any of the permissions, for
// exports that come later, such as report exports.
func RegisterType(name string, perms ...administration.Permission) {
	typesMu.Lock()
	defer typesMu.Unlock()
	types[name] = slices.Clone(perms)
}

// Permissions returns the permissions that open downloads of the type.
func Permissions(name string) ([]administration.Permission, bool) {
	typesMu.RLock()
	defer typesMu.RUnlock()
	p, ok := types[name]
	return slices.Clone(p), ok
}

// Types lists every download type, sorted.
func Types() []string {
	typesMu.RLock()
	defer typesMu.RUnlock()
	out := make([]string, 0, len(types))
	for t := range types {
		out = append(out, t)
	}
	slices.Sort(out)
	return out
}

// Download is a file an export made.
type Download struct {
	ID               resource.ID
	AdministrationID resource.ID
	// UserID is who asked for the export; nil for the system.
	UserID      *resource.ID
	Type        string
	Filename    string
	ContentType string
	Size        int64
	Failed      bool
	Downloaded  bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Key is where the file is kept.
func (d Download) Key() string {
	return fmt.Sprintf("administrations/%d/downloads/%d", d.AdministrationID, d.ID)
}

// Query selects downloads.
type Query struct {
	// Types keeps the downloads of these types: those the user may open.
	Types      []string
	Type       string
	Downloaded *bool
	Failed     *bool
	Page       int
	PerPage    int
}

// Matches reports whether the query selects the download; the reference
// for stores without a query language of their own.
func (q Query) Matches(d Download) bool {
	switch {
	case !slices.Contains(q.Types, d.Type),
		q.Type != "" && d.Type != q.Type,
		q.Downloaded != nil && d.Downloaded != *q.Downloaded,
		q.Failed != nil && d.Failed != *q.Failed:
		return false
	}
	return true
}

// Administrations checks access to an administration.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Files keeps file contents.
type Files interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	Open(ctx context.Context, key string) (io.ReadCloser, string, error)
	Delete(ctx context.Context, key string) error
}

// Store persists download records.
type Store interface {
	// Downloads lists the downloads the query selects, the newest first,
	// with the number selected in all.
	Downloads(ctx context.Context, admin resource.ID, q Query) ([]Download, int, error)
	Download(ctx context.Context, admin, id resource.ID) (Download, error)
	SaveDownload(ctx context.Context, d Download) error
}

// Service implements the download use cases.
type Service struct {
	admins Administrations
	store  Store
	files  Files
	ids    *resource.Generator
	now    func() time.Time
}

// NewService returns a Service.
func NewService(admins Administrations, store Store, files Files, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, store: store, files: files, ids: ids, now: now}
}

// List returns the page of downloads the query selects.
func (s *Service) List(ctx context.Context, admin, user resource.ID, q Query) ([]Download, int, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, 0, err
	}
	if q.Type != "" {
		if _, ok := Permissions(q.Type); !ok {
			return nil, 0, ErrUnknownType
		}
	}
	q.Page = max(q.Page, 1)
	if q.PerPage <= 0 {
		q.PerPage = DefaultPerPage
	}
	q.PerPage = min(q.PerPage, MaxPerPage)
	list, total, err := s.store.Downloads(ctx, admin, q)
	if err != nil {
		return nil, 0, fmt.Errorf("list downloads: %w", err)
	}
	return list, total, nil
}

// Download returns one download.
func (s *Service) Download(ctx context.Context, admin, id, user resource.ID) (Download, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Download{}, err
	}
	return s.store.Download(ctx, admin, id)
}

// Open returns a download's file, and marks it downloaded; the caller
// closes the file.
func (s *Service) Open(ctx context.Context, admin, id, user resource.ID) (Download, io.ReadCloser, error) {
	d, err := s.Download(ctx, admin, id, user)
	if err != nil {
		return Download{}, nil, err
	}
	if d.Failed {
		return Download{}, nil, ErrFailed
	}
	body, _, err := s.files.Open(ctx, d.Key())
	if err != nil {
		return Download{}, nil, fmt.Errorf("open file: %w", err)
	}
	if !d.Downloaded {
		d.Downloaded, d.UpdatedAt = true, s.now()
		if err := s.store.SaveDownload(ctx, d); err != nil {
			_ = body.Close()
			return Download{}, nil, fmt.Errorf("store download: %w", err)
		}
	}
	return d, body, nil
}

// Add stores a finished export's file as a download of the type; user is
// who asked for it, nil for the system. Exports call it.
func (s *Service) Add(ctx context.Context, admin resource.ID, user *resource.ID, downloadType, filename, contentType string, data []byte) (Download, error) {
	d, err := s.record(admin, user, downloadType, filename, contentType)
	if err != nil {
		return Download{}, err
	}
	d.Size = int64(len(data))
	if err := s.files.Put(ctx, d.Key(), d.ContentType, data); err != nil {
		return Download{}, fmt.Errorf("store file: %w", err)
	}
	if err := s.store.SaveDownload(ctx, d); err != nil {
		_ = s.files.Delete(ctx, d.Key())
		return Download{}, fmt.Errorf("store download: %w", err)
	}
	return d, nil
}

// AddFailed records an export that failed, so people see it did.
func (s *Service) AddFailed(ctx context.Context, admin resource.ID, user *resource.ID, downloadType, filename string) (Download, error) {
	d, err := s.record(admin, user, downloadType, filename, "")
	if err != nil {
		return Download{}, err
	}
	d.Failed = true
	if err := s.store.SaveDownload(ctx, d); err != nil {
		return Download{}, fmt.Errorf("store download: %w", err)
	}
	return d, nil
}

func (s *Service) record(admin resource.ID, user *resource.ID, downloadType, filename, contentType string) (Download, error) {
	if _, ok := Permissions(downloadType); !ok {
		return Download{}, ErrUnknownType
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "Unknown"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	now := s.now()
	return Download{
		ID: s.ids.Next(), AdministrationID: admin, UserID: user, Type: downloadType, Filename: filename, ContentType: contentType,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}
