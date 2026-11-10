package invoicing

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// FileKind names the files a document style can hold.
type FileKind string

// The files of a document style.
const (
	Logo       FileKind = "logo"
	Background FileKind = "background"
)

// Upload limits per file kind.
var fileRules = map[FileKind]struct {
	maxBytes int
	types    []string
}{
	Logo:       {maxBytes: 2 << 20, types: []string{"image/png", "image/jpeg", "image/gif"}},
	Background: {maxBytes: 10 << 20, types: []string{"image/png", "image/jpeg", "application/pdf"}},
}

// Choices for document style fields.
var (
	LogoPositions    = []string{"left", "center", "right"}
	AddressPositions = []string{"left", "right"}
	PaperSizes       = []string{"A4", "letter"}
	FontFamilies     = []string{"lucida_grande", "helvetica", "arial", "verdana", "georgia", "times_new_roman", "courier"}
	// SenderFields are the parts of the sender block on a document.
	SenderFields = []string{"company_name", "address", "newline", "email", "phone", "website", "chamber_of_commerce", "tax_number", "bank_account_number", "bank_account_bic", "bank_account_name"}
	// InvoiceMetadataFields and EstimateMetadataFields are the details shown
	// beside the address.
	InvoiceMetadataFields  = []string{"invoice_id", "invoice_date", "due_date", "reference", "customer_id"}
	EstimateMetadataFields = []string{"estimate_id", "estimate_date", "due_date", "reference", "customer_id"}
)

const (
	minFontSize, maxFontSize   = 8, 14
	minLogoWidth, maxLogoWidth = 10, 100
	maxLines                   = 20
	maxCustomCSS               = 20000
)

// Line is one entry of a block on a document, such as the company name in
// bold or the VAT number with its label.
type Line struct {
	Field string
	Bold  bool
	Label bool
}

// Layout is where the parts of invoices and estimates go.
type Layout struct {
	InvoiceSenderAddress  []Line
	InvoiceMetadataLeft   []Line
	InvoiceMetadataRight  []Line
	EstimateSenderAddress []Line
	EstimateMetadataLeft  []Line
	EstimateMetadataRight []Line
}

// StoredFile is an uploaded file, named by the SHA-256 of its content.
type StoredFile struct {
	Hash        string
	ContentType string
}

// DocumentStyle is the look of an administration's invoices and estimates.
type DocumentStyle struct {
	ID                     resource.ID
	AdministrationID       resource.ID
	Name                   string
	IdentityID             resource.ID
	Default                bool
	Logo                   *StoredFile
	LogoContainerFullWidth bool
	LogoDisplayWidth       int
	LogoPosition           string
	Background             *StoredFile
	PrintOnStationery      bool
	PaperSize              string
	AddressPosition        string
	FontSize               int
	FontFamily             string
	CustomCSS              string
	Layout
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Ref locates the document style.
func (d DocumentStyle) Ref() resource.Ref {
	return resource.Ref{ID: d.ID, Type: resource.TypeDocumentStyle, AdministrationID: d.AdministrationID}
}

// DocumentStyleChanges holds the fields to set. Nil fields stay as they are.
type DocumentStyleChanges struct {
	Name                   *string
	IdentityID             *resource.ID
	Default                *bool
	LogoContainerFullWidth *bool
	LogoDisplayWidth       *int
	LogoPosition           *string
	PrintOnStationery      *bool
	PaperSize              *string
	AddressPosition        *string
	FontSize               *int
	FontFamily             *string
	CustomCSS              *string

	InvoiceSenderAddress  *[]Line
	InvoiceMetadataLeft   *[]Line
	InvoiceMetadataRight  *[]Line
	EstimateSenderAddress *[]Line
	EstimateMetadataLeft  *[]Line
	EstimateMetadataRight *[]Line
}

// DocumentStyles lists the administration's document styles, the default
// first, creating the default style on first use.
func (s *Service) DocumentStyles(ctx context.Context, admin, user resource.ID) ([]DocumentStyle, error) {
	if err := s.ensureDocumentStyle(ctx, admin, user); err != nil {
		return nil, err
	}
	list, err := s.store.DocumentStyles(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("list document styles: %w", err)
	}
	slices.SortStableFunc(list, func(x, y DocumentStyle) int {
		return cmp.Or(-boolCompare(x.Default, y.Default), cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name)), cmp.Compare(x.ID, y.ID))
	})
	return list, nil
}

// DocumentStyle returns one document style.
func (s *Service) DocumentStyle(ctx context.Context, admin, id, user resource.ID) (DocumentStyle, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return DocumentStyle{}, err
	}
	return s.store.DocumentStyle(ctx, admin, id)
}

// CreateDocumentStyle adds a document style, starting from the defaults.
func (s *Service) CreateDocumentStyle(ctx context.Context, admin, user resource.ID, c DocumentStyleChanges) (DocumentStyle, error) {
	if err := s.ensureDocumentStyle(ctx, admin, user); err != nil {
		return DocumentStyle{}, err
	}
	def, err := s.admins.DefaultIdentity(ctx, admin, user)
	if err != nil {
		return DocumentStyle{}, err
	}
	d := defaultDocumentStyle(admin, def.ID, "")
	d.Default = false
	if err := s.applyStyle(ctx, &d, user, c); err != nil {
		return DocumentStyle{}, err
	}
	now := s.now()
	d.ID, d.CreatedAt, d.UpdatedAt = s.ids.Next(), now, now
	makeDefault := d.Default
	d.Default = false
	if _, err := s.store.CreateDocumentStyle(ctx, d); err != nil {
		return DocumentStyle{}, fmt.Errorf("store document style: %w", err)
	}
	if makeDefault {
		d.Default = true
		if err := s.store.UpdateDocumentStyle(ctx, d); err != nil {
			return DocumentStyle{}, fmt.Errorf("make document style default: %w", err)
		}
	}
	return d, nil
}

// UpdateDocumentStyle changes a document style.
func (s *Service) UpdateDocumentStyle(ctx context.Context, admin, id, user resource.ID, c DocumentStyleChanges) (DocumentStyle, error) {
	d, err := s.DocumentStyle(ctx, admin, id, user)
	if err != nil {
		return DocumentStyle{}, err
	}
	wasDefault := d.Default
	if err := s.applyStyle(ctx, &d, user, c); err != nil {
		return DocumentStyle{}, err
	}
	if wasDefault && !d.Default {
		return DocumentStyle{}, ErrDefaultInUse
	}
	d.UpdatedAt = s.now()
	if err := s.store.UpdateDocumentStyle(ctx, d); err != nil {
		return DocumentStyle{}, fmt.Errorf("store document style: %w", err)
	}
	return d, nil
}

// DeleteDocumentStyle removes a document style that is not the default.
func (s *Service) DeleteDocumentStyle(ctx context.Context, admin, id, user resource.ID) error {
	d, err := s.DocumentStyle(ctx, admin, id, user)
	if err != nil {
		return err
	}
	if d.Default {
		return ErrDefaultInUse
	}
	if err := s.store.DeleteDocumentStyle(ctx, admin, id); err != nil {
		return err
	}
	return s.releaseFiles(ctx, admin, d.Logo, d.Background)
}

// UploadFile stores a logo or background for a document style, replacing
// the previous one.
func (s *Service) UploadFile(ctx context.Context, admin, id, user resource.ID, kind FileKind, data []byte) (DocumentStyle, error) {
	d, err := s.DocumentStyle(ctx, admin, id, user)
	if err != nil {
		return DocumentStyle{}, err
	}
	rule, ok := fileRules[kind]
	if !ok {
		return DocumentStyle{}, fmt.Errorf("unknown file kind %q", kind)
	}
	var verr resource.ValidationError
	contentType := http.DetectContentType(data)
	switch {
	case len(data) == 0:
		verr.Add(string(kind), "can't be blank")
	case len(data) > rule.maxBytes:
		verr.Add(string(kind), fmt.Sprintf("must be at most %d MB", rule.maxBytes>>20))
	case !slices.Contains(rule.types, contentType):
		verr.Add(string(kind), "must be a "+strings.Join(fileTypeNames(rule.types), ", ")+" file")
	}
	if err := verr.OrNil(); err != nil {
		return DocumentStyle{}, err
	}
	sum := sha256.Sum256(data)
	file := &StoredFile{Hash: hex.EncodeToString(sum[:]), ContentType: contentType}
	if err := s.files.Put(ctx, fileKey(admin, file.Hash), contentType, data); err != nil {
		return DocumentStyle{}, fmt.Errorf("store %s: %w", kind, err)
	}
	previous := d.Logo
	if kind == Logo {
		d.Logo = file
	} else {
		previous, d.Background = d.Background, file
	}
	d.UpdatedAt = s.now()
	if err := s.store.UpdateDocumentStyle(ctx, d); err != nil {
		return DocumentStyle{}, fmt.Errorf("store document style: %w", err)
	}
	if previous != nil && previous.Hash != file.Hash {
		if err := s.releaseFiles(ctx, admin, previous); err != nil {
			return DocumentStyle{}, err
		}
	}
	return d, nil
}

// RemoveFile drops a logo or background from a document style.
func (s *Service) RemoveFile(ctx context.Context, admin, id, user resource.ID, kind FileKind) (DocumentStyle, error) {
	d, err := s.DocumentStyle(ctx, admin, id, user)
	if err != nil {
		return DocumentStyle{}, err
	}
	var previous *StoredFile
	if kind == Logo {
		previous, d.Logo = d.Logo, nil
	} else {
		previous, d.Background = d.Background, nil
	}
	d.UpdatedAt = s.now()
	if err := s.store.UpdateDocumentStyle(ctx, d); err != nil {
		return DocumentStyle{}, fmt.Errorf("store document style: %w", err)
	}
	return d, s.releaseFiles(ctx, admin, previous)
}

// OpenFile returns a document style's logo or background. The caller closes
// the reader.
func (s *Service) OpenFile(ctx context.Context, admin, id, user resource.ID, kind FileKind) (io.ReadCloser, string, error) {
	d, err := s.DocumentStyle(ctx, admin, id, user)
	if err != nil {
		return nil, "", err
	}
	file := d.Logo
	if kind == Background {
		file = d.Background
	}
	if file == nil {
		return nil, "", ErrFileNotFound
	}
	body, _, err := s.files.Open(ctx, fileKey(admin, file.Hash))
	if err != nil {
		return nil, "", fmt.Errorf("open %s: %w", kind, err)
	}
	return body, file.ContentType, nil
}

// releaseFiles deletes files that no document style uses any more.
func (s *Service) releaseFiles(ctx context.Context, admin resource.ID, files ...*StoredFile) error {
	var errs []error
	for _, f := range files {
		if f == nil {
			continue
		}
		used, err := s.store.FileInUse(ctx, admin, f.Hash)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !used {
			errs = append(errs, s.files.Delete(ctx, fileKey(admin, f.Hash)))
		}
	}
	return errors.Join(errs...)
}

func fileKey(admin resource.ID, hash string) string {
	return fmt.Sprintf("administrations/%s/document_styles/%s", admin, hash)
}

func fileTypeNames(types []string) []string {
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = strings.ToUpper(strings.TrimPrefix(strings.TrimPrefix(t, "image/"), "application/"))
	}
	return names
}

func (s *Service) applyStyle(ctx context.Context, d *DocumentStyle, user resource.ID, c DocumentStyleChanges) error {
	var verr resource.ValidationError
	if c.Name != nil {
		d.Name = strings.TrimSpace(*c.Name)
	}
	if c.IdentityID != nil {
		_, err := s.admins.Identity(ctx, d.AdministrationID, *c.IdentityID, user)
		switch {
		case errors.Is(err, administration.ErrIdentityNotFound):
			verr.Add("identity_id", "must be one of the administration's sender addresses")
		case err != nil:
			return err
		default:
			d.IdentityID = *c.IdentityID
		}
	}
	setBool := func(dst *bool, src *bool) {
		if src != nil {
			*dst = *src
		}
	}
	setBool(&d.Default, c.Default)
	setBool(&d.LogoContainerFullWidth, c.LogoContainerFullWidth)
	setBool(&d.PrintOnStationery, c.PrintOnStationery)
	setInt := func(dst *int, src *int) {
		if src != nil {
			*dst = *src
		}
	}
	setInt(&d.LogoDisplayWidth, c.LogoDisplayWidth)
	setInt(&d.FontSize, c.FontSize)
	for _, f := range []struct {
		field   string
		dst     *string
		src     *string
		choices []string
	}{
		{"logo_position", &d.LogoPosition, c.LogoPosition, LogoPositions},
		{"paper_size", &d.PaperSize, c.PaperSize, PaperSizes},
		{"address_position", &d.AddressPosition, c.AddressPosition, AddressPositions},
		{"font_family", &d.FontFamily, c.FontFamily, FontFamilies},
	} {
		if f.src == nil {
			continue
		}
		if !slices.Contains(f.choices, *f.src) {
			verr.Add(f.field, "is not included in the list")
			continue
		}
		*f.dst = *f.src
	}
	if c.CustomCSS != nil {
		d.CustomCSS = *c.CustomCSS
	}
	for _, f := range []struct {
		field   string
		dst     *[]Line
		src     *[]Line
		choices []string
	}{
		{"invoice_sender_address", &d.InvoiceSenderAddress, c.InvoiceSenderAddress, SenderFields},
		{"invoice_metadata_left", &d.InvoiceMetadataLeft, c.InvoiceMetadataLeft, InvoiceMetadataFields},
		{"invoice_metadata_right", &d.InvoiceMetadataRight, c.InvoiceMetadataRight, InvoiceMetadataFields},
		{"estimate_sender_address", &d.EstimateSenderAddress, c.EstimateSenderAddress, SenderFields},
		{"estimate_metadata_left", &d.EstimateMetadataLeft, c.EstimateMetadataLeft, EstimateMetadataFields},
		{"estimate_metadata_right", &d.EstimateMetadataRight, c.EstimateMetadataRight, EstimateMetadataFields},
	} {
		if f.src == nil {
			continue
		}
		if len(*f.src) > maxLines || slices.ContainsFunc(*f.src, func(l Line) bool { return !slices.Contains(f.choices, l.Field) }) {
			verr.Add(f.field, fmt.Sprintf("must list at most %d known fields", maxLines))
			continue
		}
		*f.dst = slices.Clone(*f.src)
	}
	if d.Name == "" || len(d.Name) > 255 {
		verr.Add("name", "can't be blank")
	}
	if d.FontSize < minFontSize || d.FontSize > maxFontSize {
		verr.Add("font_size", fmt.Sprintf("must be between %d and %d", minFontSize, maxFontSize))
	}
	if d.LogoDisplayWidth < minLogoWidth || d.LogoDisplayWidth > maxLogoWidth {
		verr.Add("logo_display_width", fmt.Sprintf("must be between %d and %d", minLogoWidth, maxLogoWidth))
	}
	if len(d.CustomCSS) > maxCustomCSS {
		verr.Add("custom_css", fmt.Sprintf("must be at most %d characters", maxCustomCSS))
	}
	return verr.OrNil()
}

// ensureDocumentStyle creates the default document style for an
// administration that has none yet.
func (s *Service) ensureDocumentStyle(ctx context.Context, admin, user resource.ID) error {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return err
	}
	def, err := s.admins.DefaultIdentity(ctx, admin, user)
	if err != nil {
		return fmt.Errorf("default identity: %w", err)
	}
	name := "Default"
	if strings.HasPrefix(a.Language, "nl") {
		name = "Standaard"
	}
	d := defaultDocumentStyle(admin, def.ID, name)
	now := s.now()
	d.ID, d.CreatedAt, d.UpdatedAt = s.ids.Next(), now, now
	if _, err := s.store.CreateDocumentStyle(ctx, d); err != nil {
		return fmt.Errorf("create default document style: %w", err)
	}
	return nil
}

func defaultDocumentStyle(admin, identity resource.ID, name string) DocumentStyle {
	sender := []Line{
		{Field: "company_name", Bold: true}, {Field: "address"}, {Field: "newline"}, {Field: "email"}, {Field: "phone"},
		{Field: "newline"}, {Field: "chamber_of_commerce", Label: true}, {Field: "tax_number", Label: true},
		{Field: "bank_account_number", Label: true},
	}
	return DocumentStyle{
		AdministrationID: admin, Name: name, IdentityID: identity, Default: true,
		LogoDisplayWidth: 50, LogoPosition: "left", PaperSize: "A4", AddressPosition: "left",
		FontSize: 10, FontFamily: "lucida_grande",
		Layout: Layout{
			InvoiceSenderAddress:  sender,
			InvoiceMetadataLeft:   []Line{{Field: "reference"}},
			InvoiceMetadataRight:  []Line{{Field: "invoice_date"}, {Field: "due_date"}},
			EstimateSenderAddress: slices.Clone(sender),
			EstimateMetadataLeft:  []Line{{Field: "reference"}},
			EstimateMetadataRight: []Line{{Field: "estimate_date"}, {Field: "due_date"}},
		},
	}
}
