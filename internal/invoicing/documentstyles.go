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

// The files of a document style: the logo and background of its documents,
// and the images of its emails and public view.
const (
	Logo                      FileKind = "logo"
	Background                FileKind = "background"
	EmailLogo                 FileKind = "email_logo"
	EmailHeaderBackground     FileKind = "email_header_background"
	EmailFooterProfilePicture FileKind = "email_footer_profile_picture"
	EmailFooterBackground     FileKind = "email_footer_background"
	PublicViewLogo            FileKind = "public_view_logo"
)

// FileKinds lists every kind of file a document style holds.
var FileKinds = []FileKind{Logo, Background, EmailLogo, EmailHeaderBackground, EmailFooterProfilePicture, EmailFooterBackground, PublicViewLogo}

var imageTypes = []string{"image/png", "image/jpeg", "image/gif"}

// Upload limits per file kind.
var fileRules = map[FileKind]struct {
	maxBytes int
	types    []string
}{
	Logo:                      {maxBytes: 2 << 20, types: imageTypes},
	Background:                {maxBytes: 10 << 20, types: []string{"image/png", "image/jpeg", "application/pdf"}},
	EmailLogo:                 {maxBytes: 2 << 20, types: imageTypes},
	EmailHeaderBackground:     {maxBytes: 5 << 20, types: imageTypes},
	EmailFooterProfilePicture: {maxBytes: 2 << 20, types: imageTypes},
	EmailFooterBackground:     {maxBytes: 5 << 20, types: imageTypes},
	PublicViewLogo:            {maxBytes: 2 << 20, types: imageTypes},
}

// maxImages is how many images a style's custom CSS can use; each is at most
// imageBytes.
const (
	maxImages  = 20
	imageBytes = 2 << 20
)

// Choices for document style fields.
var (
	LogoPositions    = []string{"left", "center", "right"}
	AddressPositions = []string{"left", "right"}
	PaperSizes       = []string{"A4", "letter"}
	FontFamilies     = []string{"lucida_grande", "helvetica", "arial", "verdana", "georgia", "times_new_roman", "courier"}
	// SenderFields are the parts of the sender block on a document, besides
	// the identity's custom fields.
	SenderFields = []string{"company_name", "address", "newline", "email", "phone", "company_website_url", "chamber_of_commerce", "tax_number", "bank_account_number", "bank_account_bic", "bank_account_name"}
	// CustomerFields are the customer's details a document can show under
	// its address or among its metadata, besides the contact's custom fields.
	CustomerFields = []string{"customer_tax_number", "customer_chamber_of_commerce", "customer_id", "customer_email", "customer_phone"}
	// InvoiceMetadataFields and EstimateMetadataFields are the details shown
	// beside the address.
	InvoiceMetadataFields  = append([]string{"invoice_id", "invoice_date", "due_date", "reference"}, CustomerFields...)
	EstimateMetadataFields = append([]string{"estimate_id", "estimate_date", "due_date", "reference"}, CustomerFields...)
)

// CustomFieldPrefix starts the field name of a custom field in a layout:
// custom_field_ followed by its id, as Moneybird names them.
const CustomFieldPrefix = "custom_field_"

const (
	minFontSize, maxFontSize   = 8, 14
	minLogoWidth, maxLogoWidth = 10, 100
	maxLines                   = 20
	maxCustomCSS               = 20000
)

// Line is one entry of a block on a document, such as the company name in
// bold or the VAT number with its label.
type Line struct {
	Field  string
	Bold   bool
	Italic bool
	Label  bool
}

// Layout is where the parts of invoices and estimates go.
type Layout struct {
	InvoiceSenderAddress  []Line
	InvoiceMetadataLeft   []Line
	InvoiceMetadataRight  []Line
	EstimateSenderAddress []Line
	EstimateMetadataLeft  []Line
	EstimateMetadataRight []Line
	// InvoiceRecipientAddress and EstimateRecipientAddress are the details
	// printed under the customer's address.
	InvoiceRecipientAddress  []Line
	EstimateRecipientAddress []Line
	// HideSenderAddress leaves the sender out, for a background that has it.
	HideSenderAddress bool
	// SenderAddressWidth is the width of the sender block in pixels.
	SenderAddressWidth int
	// ShowEPCCode prints a QR code customers can pay invoices with.
	ShowEPCCode bool
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
	Email      EmailStyle
	PublicView PublicViewStyle
	Checkout   CheckoutStyle
	// Files holds the files of the kinds other than Logo and Background.
	Files map[FileKind]StoredFile
	// Images are the files custom CSS can show, in upload order.
	Images    []StoredFile
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

	InvoiceRecipientAddress  *[]Line
	EstimateRecipientAddress *[]Line
	HideSenderAddress        *bool
	SenderAddressWidth       *int
	ShowEPCCode              *bool

	Email      EmailChanges
	PublicView PublicViewChanges
	Checkout   CheckoutChanges
}

// File returns the style's file of a kind, or nil.
func (d DocumentStyle) File(kind FileKind) *StoredFile {
	switch kind {
	case Logo:
		return d.Logo
	case Background:
		return d.Background
	}
	if f, ok := d.Files[kind]; ok {
		return &f
	}
	return nil
}

func (d *DocumentStyle) setFile(kind FileKind, f *StoredFile) {
	switch kind {
	case Logo:
		d.Logo = f
	case Background:
		d.Background = f
	default:
		files := make(map[FileKind]StoredFile, len(d.Files)+1)
		for k, v := range d.Files {
			if k != kind {
				files[k] = v
			}
		}
		if f != nil {
			files[kind] = *f
		}
		d.Files = files
	}
}

// StoredFiles lists every file the style uses.
func (d DocumentStyle) StoredFiles() []StoredFile {
	var out []StoredFile
	for _, kind := range FileKinds {
		if f := d.File(kind); f != nil {
			out = append(out, *f)
		}
	}
	return append(out, d.Images...)
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
	return s.releaseFiles(ctx, admin, d.StoredFiles()...)
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
	file, err := s.putFile(ctx, admin, contentType, data)
	if err != nil {
		return DocumentStyle{}, fmt.Errorf("store %s: %w", kind, err)
	}
	previous := d.File(kind)
	d.setFile(kind, &file)
	d.UpdatedAt = s.now()
	if err := s.store.UpdateDocumentStyle(ctx, d); err != nil {
		return DocumentStyle{}, fmt.Errorf("store document style: %w", err)
	}
	if previous != nil && previous.Hash != file.Hash {
		if err := s.releaseFiles(ctx, admin, *previous); err != nil {
			return DocumentStyle{}, err
		}
	}
	return d, nil
}

func (s *Service) putFile(ctx context.Context, admin resource.ID, contentType string, data []byte) (StoredFile, error) {
	sum := sha256.Sum256(data)
	file := StoredFile{Hash: hex.EncodeToString(sum[:]), ContentType: contentType}
	return file, s.files.Put(ctx, fileKey(admin, file.Hash), contentType, data)
}

// RemoveFile drops a logo or background from a document style.
func (s *Service) RemoveFile(ctx context.Context, admin, id, user resource.ID, kind FileKind) (DocumentStyle, error) {
	d, err := s.DocumentStyle(ctx, admin, id, user)
	if err != nil {
		return DocumentStyle{}, err
	}
	if _, ok := fileRules[kind]; !ok {
		return DocumentStyle{}, ErrFileNotFound
	}
	previous := d.File(kind)
	d.setFile(kind, nil)
	d.UpdatedAt = s.now()
	if err := s.store.UpdateDocumentStyle(ctx, d); err != nil {
		return DocumentStyle{}, fmt.Errorf("store document style: %w", err)
	}
	if previous == nil {
		return d, nil
	}
	return d, s.releaseFiles(ctx, admin, *previous)
}

// OpenFile returns a document style's logo or background. The caller closes
// the reader.
func (s *Service) OpenFile(ctx context.Context, admin, id, user resource.ID, kind FileKind) (io.ReadCloser, string, error) {
	d, err := s.DocumentStyle(ctx, admin, id, user)
	if err != nil {
		return nil, "", err
	}
	file := d.File(kind)
	if file == nil {
		return nil, "", ErrFileNotFound
	}
	body, _, err := s.files.Open(ctx, fileKey(admin, file.Hash))
	if err != nil {
		return nil, "", fmt.Errorf("open %s: %w", kind, err)
	}
	return body, file.ContentType, nil
}

// AddImage stores an image for the style's custom CSS. The same image twice
// is kept once.
func (s *Service) AddImage(ctx context.Context, admin, id, user resource.ID, data []byte) (DocumentStyle, error) {
	d, err := s.DocumentStyle(ctx, admin, id, user)
	if err != nil {
		return DocumentStyle{}, err
	}
	var verr resource.ValidationError
	contentType := http.DetectContentType(data)
	switch {
	case len(data) == 0:
		verr.Add("image", "can't be blank")
	case len(data) > imageBytes:
		verr.Add("image", fmt.Sprintf("must be at most %d MB", imageBytes>>20))
	case !slices.Contains(imageTypes, contentType):
		verr.Add("image", "must be a "+strings.Join(fileTypeNames(imageTypes), ", ")+" file")
	case len(d.Images) >= maxImages:
		verr.Add("image", fmt.Sprintf("can't be added; a style holds at most %d images", maxImages))
	}
	if err := verr.OrNil(); err != nil {
		return DocumentStyle{}, err
	}
	file, err := s.putFile(ctx, admin, contentType, data)
	if err != nil {
		return DocumentStyle{}, fmt.Errorf("store image: %w", err)
	}
	if slices.ContainsFunc(d.Images, func(f StoredFile) bool { return f.Hash == file.Hash }) {
		return d, nil
	}
	d.Images = append(slices.Clone(d.Images), file)
	d.UpdatedAt = s.now()
	if err := s.store.UpdateDocumentStyle(ctx, d); err != nil {
		return DocumentStyle{}, fmt.Errorf("store document style: %w", err)
	}
	return d, nil
}

// OpenImage returns one of the style's CSS images by its hash. The caller
// closes the reader.
func (s *Service) OpenImage(ctx context.Context, admin, id, user resource.ID, hash string) (io.ReadCloser, string, error) {
	d, err := s.DocumentStyle(ctx, admin, id, user)
	if err != nil {
		return nil, "", err
	}
	i := slices.IndexFunc(d.Images, func(f StoredFile) bool { return f.Hash == hash })
	if i < 0 {
		return nil, "", ErrFileNotFound
	}
	body, _, err := s.files.Open(ctx, fileKey(admin, hash))
	if err != nil {
		return nil, "", fmt.Errorf("open image: %w", err)
	}
	return body, d.Images[i].ContentType, nil
}

// RemoveImage drops one of the style's CSS images.
func (s *Service) RemoveImage(ctx context.Context, admin, id, user resource.ID, hash string) (DocumentStyle, error) {
	d, err := s.DocumentStyle(ctx, admin, id, user)
	if err != nil {
		return DocumentStyle{}, err
	}
	i := slices.IndexFunc(d.Images, func(f StoredFile) bool { return f.Hash == hash })
	if i < 0 {
		return DocumentStyle{}, ErrFileNotFound
	}
	removed := d.Images[i]
	d.Images = slices.Delete(slices.Clone(d.Images), i, i+1)
	d.UpdatedAt = s.now()
	if err := s.store.UpdateDocumentStyle(ctx, d); err != nil {
		return DocumentStyle{}, fmt.Errorf("store document style: %w", err)
	}
	return d, s.releaseFiles(ctx, admin, removed)
}

// releaseFiles deletes files that no document style uses any more.
func (s *Service) releaseFiles(ctx context.Context, admin resource.ID, files ...StoredFile) error {
	var errs []error
	for _, f := range files {
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
	custom, err := s.customFieldSources(ctx, d.AdministrationID, user, c)
	if err != nil {
		return err
	}
	identity := []administration.Source{administration.SourceIdentity}
	invoice := []administration.Source{administration.SourceSalesInvoice, administration.SourceContact}
	estimate := []administration.Source{administration.SourceEstimate, administration.SourceContact}
	contact := []administration.Source{administration.SourceContact}
	for _, f := range []struct {
		field   string
		dst     *[]Line
		src     *[]Line
		choices []string
		sources []administration.Source
	}{
		{"invoice_sender_address", &d.InvoiceSenderAddress, c.InvoiceSenderAddress, SenderFields, identity},
		{"invoice_metadata_left", &d.InvoiceMetadataLeft, c.InvoiceMetadataLeft, InvoiceMetadataFields, invoice},
		{"invoice_metadata_right", &d.InvoiceMetadataRight, c.InvoiceMetadataRight, InvoiceMetadataFields, invoice},
		{"estimate_sender_address", &d.EstimateSenderAddress, c.EstimateSenderAddress, SenderFields, identity},
		{"estimate_metadata_left", &d.EstimateMetadataLeft, c.EstimateMetadataLeft, EstimateMetadataFields, estimate},
		{"estimate_metadata_right", &d.EstimateMetadataRight, c.EstimateMetadataRight, EstimateMetadataFields, estimate},
		{"invoice_recipient_address", &d.InvoiceRecipientAddress, c.InvoiceRecipientAddress, CustomerFields, contact},
		{"estimate_recipient_address", &d.EstimateRecipientAddress, c.EstimateRecipientAddress, CustomerFields, contact},
	} {
		if f.src == nil {
			continue
		}
		known := func(l Line) bool {
			source, ok := custom[l.Field]
			return slices.Contains(f.choices, l.Field) || (ok && slices.Contains(f.sources, source))
		}
		if len(*f.src) > maxLines || slices.ContainsFunc(*f.src, func(l Line) bool { return !known(l) }) {
			verr.Add(f.field, fmt.Sprintf("must list at most %d known fields", maxLines))
			continue
		}
		*f.dst = slices.Clone(*f.src)
	}
	applyChannels(d, c, &verr)
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

// customFieldSources maps the layout name of each custom field to the kind
// of record it belongs to, reading the fields only when the changes use one.
func (s *Service) customFieldSources(ctx context.Context, admin, user resource.ID, c DocumentStyleChanges) (map[string]administration.Source, error) {
	used := false
	for _, ls := range []*[]Line{c.InvoiceSenderAddress, c.InvoiceMetadataLeft, c.InvoiceMetadataRight, c.EstimateSenderAddress,
		c.EstimateMetadataLeft, c.EstimateMetadataRight, c.InvoiceRecipientAddress, c.EstimateRecipientAddress} {
		if ls != nil && slices.ContainsFunc(*ls, func(l Line) bool { return strings.HasPrefix(l.Field, CustomFieldPrefix) }) {
			used = true
		}
	}
	if !used {
		return nil, nil
	}
	fields, err := s.admins.CustomFields(ctx, admin, user)
	if err != nil {
		return nil, fmt.Errorf("list custom fields: %w", err)
	}
	out := make(map[string]administration.Source, len(fields))
	for _, f := range fields {
		out[CustomFieldPrefix+f.ID.String()] = f.Source
	}
	return out, nil
}

// DefaultLayout is the layout a new document style starts with, as
// Moneybird's does.
func DefaultLayout() Layout {
	sender := []Line{
		{Field: "company_name", Bold: true}, {Field: "address"}, {Field: "newline"}, {Field: "email"}, {Field: "phone"},
		{Field: "newline"}, {Field: "chamber_of_commerce", Label: true}, {Field: "tax_number", Label: true},
		{Field: "bank_account_number", Label: true},
	}
	return Layout{
		InvoiceSenderAddress:    sender,
		InvoiceMetadataLeft:     []Line{{Field: "reference"}},
		InvoiceMetadataRight:    []Line{{Field: "invoice_date"}, {Field: "due_date"}},
		EstimateSenderAddress:   slices.Clone(sender),
		EstimateMetadataLeft:    []Line{{Field: "reference"}},
		EstimateMetadataRight:   []Line{{Field: "estimate_date"}, {Field: "due_date"}},
		InvoiceRecipientAddress: []Line{}, EstimateRecipientAddress: []Line{},
		SenderAddressWidth: 200, ShowEPCCode: true,
	}
}

func defaultDocumentStyle(admin, identity resource.ID, name string) DocumentStyle {
	return DocumentStyle{
		AdministrationID: admin, Name: name, IdentityID: identity, Default: true,
		LogoDisplayWidth: 50, LogoPosition: "left", PaperSize: "A4", AddressPosition: "left",
		FontSize: 10, FontFamily: "lucida_grande", Layout: DefaultLayout(),
		Email: DefaultEmailStyle(), PublicView: DefaultPublicViewStyle(), Checkout: DefaultCheckoutStyle(),
	}
}

// PurgeFiles deletes every stored file of the administration's document
// styles, for when the administration itself goes. Callers check access.
func (s *Service) PurgeFiles(ctx context.Context, admin resource.ID) error {
	styles, err := s.store.DocumentStyles(ctx, admin)
	if err != nil {
		return fmt.Errorf("list document styles: %w", err)
	}
	seen := map[string]bool{}
	var errs []error
	for _, d := range styles {
		for _, f := range d.StoredFiles() {
			if !seen[f.Hash] {
				seen[f.Hash] = true
				errs = append(errs, s.files.Delete(ctx, fileKey(admin, f.Hash)))
			}
		}
	}
	return errors.Join(errs...)
}
