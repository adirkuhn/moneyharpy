// Package invoicing holds how an administration invoices and quotes: number
// templates, copies by email, workflows and document styles.
package invoicing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Errors returned by Service.
var (
	ErrWorkflowNotFound      = errors.New("workflow not found")
	ErrDocumentStyleNotFound = errors.New("document style not found")
	// ErrDefaultInUse reports an attempt to delete or deactivate a default.
	ErrDefaultInUse = errors.New("the default cannot be deleted or deactivated")
	// ErrFileNotFound reports a document style without the requested file,
	// or a file missing from storage. Files implementations return it.
	ErrFileNotFound = errors.New("file not found")
)

// Administrations gives access to administrations and their identities. The
// methods return administration.ErrNotFound when the user has no access.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
	DefaultIdentity(ctx context.Context, admin, user resource.ID) (administration.Identity, error)
	Identity(ctx context.Context, admin, id, user resource.ID) (administration.Identity, error)
	CustomFields(ctx context.Context, admin, user resource.ID) ([]administration.CustomField, error)
}

// Files keeps uploaded files.
type Files interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	// Open returns the file and its content type, or ErrFileNotFound. The
	// caller closes it.
	Open(ctx context.Context, key string) (io.ReadCloser, string, error)
	Delete(ctx context.Context, key string) error
}

// Store persists invoicing data. Methods taking an administration trust the
// caller to have checked access.
type Store interface {
	Settings(ctx context.Context, admin resource.ID) (Settings, error)
	SaveSettings(ctx context.Context, admin resource.ID, s Settings) error

	Workflows(ctx context.Context, admin resource.ID) ([]Workflow, error)
	Workflow(ctx context.Context, admin, id resource.ID) (Workflow, error)
	// CreateWorkflow stores w. When w is a default and the administration
	// already has a default of that type, it stores nothing and returns
	// false, so concurrent first visits create one default.
	CreateWorkflow(ctx context.Context, w Workflow) (bool, error)
	// UpdateWorkflow stores w, replacing its steps. When w is the default, the
	// previous default of its type stops being one.
	UpdateWorkflow(ctx context.Context, w Workflow) error
	DeleteWorkflow(ctx context.Context, admin, id resource.ID) error

	DocumentStyles(ctx context.Context, admin resource.ID) ([]DocumentStyle, error)
	DocumentStyle(ctx context.Context, admin, id resource.ID) (DocumentStyle, error)
	// CreateDocumentStyle behaves like CreateWorkflow for defaults.
	CreateDocumentStyle(ctx context.Context, d DocumentStyle) (bool, error)
	UpdateDocumentStyle(ctx context.Context, d DocumentStyle) error
	DeleteDocumentStyle(ctx context.Context, admin, id resource.ID) error
	// FileInUse reports whether any document style of admin uses the file.
	FileInUse(ctx context.Context, admin resource.ID, hash string) (bool, error)
}

// Service implements the invoicing use cases.
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

// Settings are an administration's numbering and document-copy preferences.
type Settings struct {
	InvoiceIDTemplate  string
	InvoiceSequenceID  string
	EstimateIDTemplate string
	EstimateSequenceID string
	// InvoiceBCC, EstimateBCC and OrderBCC receive a copy of every document
	// of their kind. Each holds addresses separated by ", ".
	InvoiceBCC          string
	InvoiceRemindersBCC bool
	EstimateBCC         string
	OrderBCC            string
	// DefaultEmailUBL attaches a UBL file to invoices sent to new contacts.
	DefaultEmailUBL bool
	// PaymentsWithoutProof allows marking invoices paid without a bank
	// transaction.
	PaymentsWithoutProof bool
}

// SettingsChanges holds the settings to set. Nil fields stay as they are.
type SettingsChanges struct {
	InvoiceIDTemplate    *string
	InvoiceSequenceID    *string
	EstimateIDTemplate   *string
	EstimateSequenceID   *string
	InvoiceBCC           *string
	InvoiceRemindersBCC  *bool
	EstimateBCC          *string
	OrderBCC             *string
	DefaultEmailUBL      *bool
	PaymentsWithoutProof *bool
}

// SettingsView is Settings with example document numbers for today.
type SettingsView struct {
	Settings
	InvoiceIDExample  string
	EstimateIDExample string
}

const maxCopyAddresses = 5

// Settings returns the administration's invoicing settings.
func (s *Service) Settings(ctx context.Context, admin, user resource.ID) (SettingsView, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return SettingsView{}, err
	}
	st, err := s.store.Settings(ctx, admin)
	if err != nil {
		return SettingsView{}, fmt.Errorf("load settings: %w", err)
	}
	return s.view(a.Administration, st), nil
}

// UpdateSettings changes the administration's invoicing settings.
func (s *Service) UpdateSettings(ctx context.Context, admin, user resource.ID, c SettingsChanges) (SettingsView, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return SettingsView{}, err
	}
	st, err := s.store.Settings(ctx, admin)
	if err != nil {
		return SettingsView{}, fmt.Errorf("load settings: %w", err)
	}
	var verr resource.ValidationError
	setText := func(dst *string, src *string) {
		if src != nil {
			*dst = strings.TrimSpace(*src)
		}
	}
	setText(&st.InvoiceIDTemplate, c.InvoiceIDTemplate)
	setText(&st.InvoiceSequenceID, c.InvoiceSequenceID)
	setText(&st.EstimateIDTemplate, c.EstimateIDTemplate)
	setText(&st.EstimateSequenceID, c.EstimateSequenceID)
	for _, f := range []struct {
		field string
		dst   *string
		src   *string
	}{
		{"invoice_bcc_email_address", &st.InvoiceBCC, c.InvoiceBCC},
		{"estimate_bcc_email_address", &st.EstimateBCC, c.EstimateBCC},
		{"order_bcc_email_address", &st.OrderBCC, c.OrderBCC},
	} {
		if f.src == nil {
			continue
		}
		list, err := addressList(*f.src)
		if err != nil {
			verr.Add(f.field, err.Error())
		}
		*f.dst = list
	}
	for _, f := range []struct {
		dst *bool
		src *bool
	}{
		{&st.InvoiceRemindersBCC, c.InvoiceRemindersBCC},
		{&st.DefaultEmailUBL, c.DefaultEmailUBL},
		{&st.PaymentsWithoutProof, c.PaymentsWithoutProof},
	} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if st.InvoiceBCC == "" {
		st.InvoiceRemindersBCC = false
	}
	for _, f := range []struct {
		field string
		err   error
	}{
		{"invoice_id_template", validateTemplate(st.InvoiceIDTemplate)},
		{"invoice_sequence_id", validateSequence(st.InvoiceSequenceID)},
		{"estimate_id_template", validateTemplate(st.EstimateIDTemplate)},
		{"estimate_sequence_id", validateSequence(st.EstimateSequenceID)},
	} {
		if f.err != nil {
			verr.Add(f.field, f.err.Error())
		}
	}
	if err := verr.OrNil(); err != nil {
		return SettingsView{}, err
	}
	if err := s.store.SaveSettings(ctx, admin, st); err != nil {
		return SettingsView{}, fmt.Errorf("save settings: %w", err)
	}
	return s.view(a.Administration, st), nil
}

func (s *Service) view(a administration.Administration, st Settings) SettingsView {
	at := s.now()
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		at = at.In(loc)
	}
	return SettingsView{
		Settings:          st,
		InvoiceIDExample:  RenderNumber(st.InvoiceIDTemplate, st.InvoiceSequenceID, at, a.Language, "1"),
		EstimateIDExample: RenderNumber(st.EstimateIDTemplate, st.EstimateSequenceID, at, a.Language, "1"),
	}
}

// addressList normalizes comma-separated email addresses to "a, b".
func addressList(raw string) (string, error) {
	var out []string
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if a, err := mail.ParseAddress(part); err != nil || a.Address != part {
			return raw, fmt.Errorf("contains %q, which is not a valid email address", part)
		}
		out = append(out, part)
	}
	if len(out) > maxCopyAddresses {
		return raw, fmt.Errorf("can hold at most %d addresses", maxCopyAddresses)
	}
	return strings.Join(out, ", "), nil
}
