package recognize

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/iban"
	"github.com/adirkuhn/moneyharpy/internal/ocr"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/settings"
)

// Documents are the incoming documents Scan and Recognise reads and fills
// in; purchases.Service implements it.
type Documents interface {
	IncomingDocument(ctx context.Context, admin, id, user resource.ID) (purchases.Incoming, error)
	TypelessDocument(ctx context.Context, admin, id, user resource.ID) (purchases.Typeless, error)
	OpenTypelessAttachment(ctx context.Context, admin, id, attachment, user resource.ID) (attachments.Attachment, io.ReadCloser, error)
	Document(ctx context.Context, admin resource.ID, kind string, id, user resource.ID) (purchases.Document, error)
	OpenAttachment(ctx context.Context, admin, user resource.ID, kind string, id, attachment resource.ID) (attachments.Attachment, io.ReadCloser, error)
	Propose(ctx context.Context, admin, id, user resource.ID, kind string, c purchases.Changes) (purchases.Document, error)
}

// Contacts finds and adds suppliers; contacts.Service implements it.
type Contacts interface {
	List(ctx context.Context, admin, user resource.ID, q contacts.Query) (contacts.Page, error)
	Create(ctx context.Context, admin, user resource.ID, c contacts.Changes) (contacts.Contact, error)
}

// Ledger gives the VAT rates lines get, and the category of expenses still
// to categorise, which lines without one book on.
type Ledger interface {
	TaxRates(ctx context.Context, admin, user resource.ID, f bookkeeping.TaxRateFilter) ([]bookkeeping.TaxRate, error)
	SystemLedgerAccount(ctx context.Context, admin, user resource.ID, role string) (resource.ID, error)
}

// Settings give the instance's AI model settings.
type Settings interface {
	Instance(ctx context.Context) (settings.Instance, error)
}

// Parsers read documents by rules: e-invoice XML exactly, and text as well
// as they can. today places dates written without a year.
type Parsers struct {
	EInvoice func(xml []byte) (Extraction, error)
	Text     func(text string, today time.Time) Extraction
}

// ModelFactory makes the AI model for the saved settings: one that asks the
// enabled connections in order.
type ModelFactory func(settings.Recognition) (Model, error)

// Notifier tells others, such as webhooks, about what Scan and Recognise
// changed.
type Notifier interface {
	DocumentUpdated(ctx context.Context, admin resource.ID, d purchases.Document)
	ContactCreated(ctx context.Context, admin resource.ID, c contacts.Contact)
}

// Store keeps recognitions and supplier memories.
type Store interface {
	// Recognition returns ErrNotFound for a document never read.
	Recognition(ctx context.Context, admin, id resource.ID) (Recognition, error)
	Recognitions(ctx context.Context, admin resource.ID, ids []resource.ID) (map[resource.ID]Recognition, error)
	SaveRecognition(ctx context.Context, r Recognition) error
	DeleteRecognition(ctx context.Context, admin, id resource.ID) error
	// LeaseDue takes up to max pending recognitions due at now, moving
	// their next attempt to lease so no other instance takes them.
	LeaseDue(ctx context.Context, now, lease time.Time, max int) ([]Recognition, error)
	// Memory returns the memory of a supplier, empty when nothing was
	// learned yet.
	Memory(ctx context.Context, admin, contact resource.ID) (Memory, error)
	SaveMemory(ctx context.Context, m Memory) error
	// History returns up to limit of the supplier's most recent booked
	// documents of a kind (purchases.PurchaseInvoice or Receipt), oldest
	// first, with what Memory.Learn reads of them.
	History(ctx context.Context, admin, contact resource.ID, kind string, limit int) ([]purchases.Document, error)
}

// Deps are what a Service needs.
type Deps struct {
	Documents Documents
	Contacts  Contacts
	Ledger    Ledger
	Settings  Settings
	Store     Store
	// OCR may be nil: only e-invoices can then be read.
	OCR     ocr.Service
	Parsers Parsers
	// Models may be nil: the AI model is then never asked.
	Models ModelFactory
	// Registry may be nil: new suppliers then keep the name read from the
	// document.
	Registry Registry
	Notifier Notifier
	Logger   *slog.Logger
	Now      func() time.Time
}

// Service is Scan and Recognise.
type Service struct {
	Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	return &Service{Deps: d}
}

// Tuning of the background work.
const (
	// MaxAttempts is how often a document is tried while the OCR service
	// or another part is unavailable.
	MaxAttempts = 3
	// retryDelay is the wait after the first failed attempt; it grows
	// fourfold with each next one.
	retryDelay = time.Minute
	// lease is how long a document being read is kept from other
	// instances.
	lease = 10 * time.Minute
	// batch is how many documents one round reads.
	batch = 10
	// maxPages is how many page images the AI model gets.
	maxPages = 5
	// maxFileSize is the largest attachment read.
	maxFileSize = 32 << 20
	// historySize is how many booked documents of a kind a supplier's
	// memory starts from.
	historySize = 20
)

var (
	// errNothingToRead means a document has nothing to read: no
	// attachments, or it is no longer of a type that can be filled in.
	errNothingToRead = errors.New("nothing to read")
	// errUnreadable means the attachments gave nothing useful.
	errUnreadable = errors.New("nothing could be read from the attachments")
)

// Queue has the document's attachments read in the background, unless that
// is already to happen. What was filled in before stays marked, so reading
// again replaces it.
func (s *Service) Queue(ctx context.Context, admin, id, user resource.ID) error {
	r, err := s.Store.Recognition(ctx, admin, id)
	switch {
	case errors.Is(err, ErrNotFound):
		r = Recognition{DocumentID: id, AdministrationID: admin, CreatedAt: s.Now()}
	case err != nil:
		return err
	case r.State == StatePending:
		return nil
	}
	now := s.Now()
	r.UserID, r.State, r.Error, r.Attempts, r.NextAttemptAt, r.UpdatedAt = user, StatePending, "", 0, &now, now
	return s.Store.SaveRecognition(ctx, r)
}

// Recognize has a document read again: an upload, or a purchase invoice or
// receipt that books nothing, with attachments.
func (s *Service) Recognize(ctx context.Context, admin, id, user resource.ID) (purchases.Incoming, error) {
	d, err := s.Documents.IncomingDocument(ctx, admin, id, user)
	if err != nil {
		return purchases.Incoming{}, err
	}
	var verr resource.ValidationError
	switch {
	case d.Kind != purchases.TypelessDocument && d.Kind != purchases.PurchaseInvoice && d.Kind != purchases.Receipt:
		verr.Add("base", "Only uploads, purchase invoices and receipts can be read")
	case d.Kind != purchases.TypelessDocument && d.State != purchases.New:
		verr.Add("base", "A booked document cannot be read again")
	case d.AttachmentsCount == 0:
		verr.Add("base", "The document has no attachment to read")
	}
	if err := verr.OrNil(); err != nil {
		return purchases.Incoming{}, err
	}
	if err := s.Queue(ctx, admin, id, user); err != nil {
		return purchases.Incoming{}, err
	}
	return d, nil
}

// Recognition returns what was read of a document; StateNone when nothing.
func (s *Service) Recognition(ctx context.Context, admin, id resource.ID) (Recognition, error) {
	r, err := s.Store.Recognition(ctx, admin, id)
	if errors.Is(err, ErrNotFound) {
		return Recognition{DocumentID: id, AdministrationID: admin, State: StateNone}, nil
	}
	return r, err
}

// Recognitions returns what was read of the documents; those never read
// are left out.
func (s *Service) Recognitions(ctx context.Context, admin resource.ID, ids []resource.ID) (map[resource.ID]Recognition, error) {
	return s.Store.Recognitions(ctx, admin, ids)
}

// Forget drops what was read of a removed document.
func (s *Service) Forget(ctx context.Context, admin, id resource.ID) error {
	return s.Store.DeleteRecognition(ctx, admin, id)
}

// Edited unmarks the fields a user changed, so they no longer show as
// filled in, marks those Remember filled in, and learns from a booked
// document what its supplier's next document holds.
func (s *Service) Edited(ctx context.Context, admin, user resource.ID, before, after purchases.Document, remembered []purchases.Remembered) {
	if err := s.edited(ctx, admin, user, before, after, remembered); err != nil {
		s.Logger.ErrorContext(ctx, "scan and recognise: learn from a saved document", "document", after.ID, "error", err)
	}
}

func (s *Service) edited(ctx context.Context, admin, user resource.ID, before, after purchases.Document, remembered []purchases.Remembered) error {
	if before.ID != 0 || len(remembered) > 0 {
		if err := s.mark(ctx, admin, user, before, after, remembered); err != nil {
			return err
		}
	}
	if !after.Booked() || after.ContactID == 0 {
		return nil
	}
	m, err := s.memory(ctx, admin, user, after.ContactID, after.ID)
	if err != nil {
		return err
	}
	uncategorized, err := s.Ledger.SystemLedgerAccount(ctx, admin, user, uncategorizedExpenses)
	if err != nil {
		return err
	}
	m.Learn(categorized(after, uncategorized), s.Now())
	return s.Store.SaveMemory(ctx, m)
}

// mark unmarks the fields the user changed but those Remember filled in,
// which it marks.
func (s *Service) mark(ctx context.Context, admin, user resource.ID, before, after purchases.Document, remembered []purchases.Remembered) error {
	r, err := s.Store.Recognition(ctx, admin, after.ID)
	switch {
	case errors.Is(err, ErrNotFound) && len(remembered) > 0:
		// Filled in from memory only, as for a document entered by hand.
		now := s.Now()
		r = Recognition{DocumentID: after.ID, AdministrationID: admin, UserID: user, State: StateDone, CreatedAt: now}
	case errors.Is(err, ErrNotFound):
		return nil
	case err != nil:
		return err
	}
	names := make([]string, len(remembered))
	for i, f := range remembered {
		names[i] = f.Field
	}
	var drop []string
	if before.ID != 0 {
		drop = slices.DeleteFunc(ChangedFields(before, after), func(name string) bool { return slices.Contains(names, name) })
	}
	changed := r.Drop(drop...)
	if len(remembered) > 0 {
		r.Drop(names...)
		for _, f := range remembered {
			conf := Uncertain
			if f.Certain {
				conf = Certain
			}
			r.Fields = append(r.Fields, Field{Name: f.Field, Confidence: conf, Reason: ReasonPreviousDocument, SourceDocumentID: f.SourceDocumentID})
		}
		r.Fields = s.describeSources(ctx, admin, user, r.Fields)
		changed = true
	}
	if !changed {
		return nil
	}
	r.UpdatedAt = s.Now()
	return s.Store.SaveRecognition(ctx, r)
}

// describeSources gives the remembered fields the reference of the
// document they came from, and forgets documents removed since.
func (s *Service) describeSources(ctx context.Context, admin, user resource.ID, fields []Field) []Field {
	refs := map[resource.ID]*string{}
	for i, f := range fields {
		if f.SourceDocumentID == 0 || f.SourceReference != "" {
			continue
		}
		ref, ok := refs[f.SourceDocumentID]
		if !ok {
			if d, err := s.Documents.IncomingDocument(ctx, admin, f.SourceDocumentID, user); err == nil {
				ref = &d.Reference
			} else if !purchases.IsNotFound(err) {
				s.Logger.WarnContext(ctx, "scan and recognise: look up a remembered document", "document", f.SourceDocumentID, "error", err)
			}
			refs[f.SourceDocumentID] = ref
		}
		if ref == nil {
			fields[i].SourceDocumentID = 0
			continue
		}
		fields[i].SourceReference = *ref
	}
	return fields
}

// uncategorizedExpenses is the role of the category lines book on until
// they get one.
const uncategorizedExpenses = "uncategorized_expenses"

// categorized leaves the categories of lines booked as still to
// categorise out of a document, so they teach nothing.
func categorized(d purchases.Document, uncategorized resource.ID) purchases.Document {
	d.Lines = slices.Clone(d.Lines)
	for i, l := range d.Lines {
		if l.LedgerAccountID != nil && *l.LedgerAccountID == uncategorized {
			d.Lines[i].LedgerAccountID = nil
		}
	}
	return d
}

// memory returns what is remembered of a supplier. A kind of document
// nothing was learned of yet is learned from the supplier's most recent
// booked documents of that kind first, so documents booked before Scan and
// Recognise remembered them, or entered by hand, count; skip is left out,
// for the caller to learn.
func (s *Service) memory(ctx context.Context, admin, user, contact, skip resource.ID) (Memory, error) {
	m, err := s.Store.Memory(ctx, admin, contact)
	if err != nil {
		return Memory{}, err
	}
	m.AdministrationID, m.ContactID = admin, contact
	var history []purchases.Document
	for _, kind := range Kinds {
		if !m.Of(kind).Empty() {
			continue
		}
		docs, err := s.Store.History(ctx, admin, contact, DocumentKind(kind), historySize)
		if err != nil {
			return Memory{}, fmt.Errorf("supplier history: %w", err)
		}
		history = append(history, docs...)
	}
	history = slices.DeleteFunc(history, func(d purchases.Document) bool { return d.ID == skip })
	if len(history) == 0 {
		return m, nil
	}
	uncategorized, err := s.Ledger.SystemLedgerAccount(ctx, admin, user, uncategorizedExpenses)
	if err != nil {
		return Memory{}, err
	}
	slices.SortStableFunc(history, func(a, b purchases.Document) int {
		if c := a.Date.Compare(b.Date); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	for _, d := range history {
		m.Learn(categorized(d, uncategorized), d.UpdatedAt)
	}
	if err := s.Store.SaveMemory(ctx, m); err != nil {
		return Memory{}, err
	}
	return m, nil
}

// Confirmed unmarks the fields a user saved, changed or not: they checked
// what Scan and Recognise filled in.
func (s *Service) Confirmed(ctx context.Context, admin, id resource.ID, fields ...string) {
	if err := s.confirmed(ctx, admin, id, fields); err != nil {
		s.Logger.ErrorContext(ctx, "scan and recognise: unmark confirmed fields", "document", id, "error", err)
	}
}

func (s *Service) confirmed(ctx context.Context, admin, id resource.ID, fields []string) error {
	r, err := s.Store.Recognition(ctx, admin, id)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil
	case err != nil:
		return err
	}
	if !r.Drop(fields...) {
		return nil
	}
	r.UpdatedAt = s.Now()
	return s.Store.SaveRecognition(ctx, r)
}

// ChangedFields names the fields that differ between two versions of a
// document, as Recognition names them. Lines added, removed or reordered
// change every line field.
func ChangedFields(before, after purchases.Document) []string {
	var out []string
	add := func(changed bool, name string) {
		if changed {
			out = append(out, name)
		}
	}
	add(before.ContactID != after.ContactID, FieldContactID)
	add(before.Reference != after.Reference, FieldReference)
	add(!before.Date.Equal(after.Date), FieldDate)
	add(!sameDay(before.DueDate, after.DueDate), FieldDueDate)
	add(before.Currency != after.Currency, FieldCurrency)
	add(before.PricesAreInclTax != after.PricesAreInclTax, FieldPricesAreInclTax)
	add(before.Kind != after.Kind, FieldDocumentType)
	n := max(len(before.Lines), len(after.Lines))
	sameLines := len(before.Lines) == len(after.Lines)
	for i := 0; sameLines && i < n; i++ {
		sameLines = before.Lines[i].ID == after.Lines[i].ID
	}
	for i := range n {
		field := func(name string) string { return fmt.Sprintf("details.%d.%s", i, name) }
		if !sameLines {
			out = append(out, field("description"), field("amount"), field("price"), field("tax_rate_id"), field("ledger_account_id"))
			continue
		}
		b, a := before.Lines[i], after.Lines[i]
		add(b.Description != a.Description, field("description"))
		add(b.Amount != a.Amount, field("amount"))
		add(b.Price != a.Price, field("price"))
		add(b.TaxRateID != a.TaxRateID, field("tax_rate_id"))
		add(!sameID(b.LedgerAccountID, a.LedgerAccountID), field("ledger_account_id"))
	}
	return out
}

func sameDay(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func sameID(a, b *resource.ID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// Run reads pending documents every interval until ctx ends.
func (s *Service) Run(ctx context.Context, interval time.Duration, onError func(error)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := s.Work(ctx); err != nil {
			onError(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Work reads the pending documents that are due, a batch at a time, until
// none is left. Errors reading a document are kept on its recognition;
// only errors of the work itself are returned.
func (s *Service) Work(ctx context.Context) error {
	for ctx.Err() == nil {
		now := s.Now()
		due, err := s.Store.LeaseDue(ctx, now, now.Add(lease), batch)
		if err != nil {
			return fmt.Errorf("lease pending documents: %w", err)
		}
		var errs []error
		for _, r := range due {
			if err := s.process(ctx, r); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 || len(due) < batch {
			return errors.Join(errs...)
		}
	}
	return nil
}

// process reads one document and keeps how that went.
func (s *Service) process(ctx context.Context, r Recognition) error {
	r.Attempts++
	err := s.fill(ctx, &r)
	now := s.Now()
	r.UpdatedAt = now
	var verr *resource.ValidationError
	switch {
	case err == nil:
		r.State, r.Error, r.NextAttemptAt = StateDone, "", nil
	case purchases.IsNotFound(err):
		return s.Store.DeleteRecognition(ctx, r.AdministrationID, r.DocumentID)
	case errors.Is(err, errNothingToRead):
		r.State, r.Error, r.NextAttemptAt = StateNone, "", nil
	case errors.As(err, &verr), errors.Is(err, errUnreadable), errors.Is(err, purchases.ErrNotEditable), r.Attempts >= MaxAttempts:
		r.State, r.Error, r.NextAttemptAt = StateFailed, failure(err), nil
		s.Logger.WarnContext(ctx, "scan and recognise: reading failed", "document", r.DocumentID, "error", err)
	default:
		next := now.Add(retryDelay << (2 * (r.Attempts - 1)))
		r.Error, r.NextAttemptAt = failure(err), &next
	}
	return s.Store.SaveRecognition(ctx, r)
}

// failure says why reading failed, for the user.
func failure(err error) string {
	var verr *resource.ValidationError
	switch {
	case errors.Is(err, ocr.ErrUnavailable):
		return "OCR service unavailable"
	case errors.Is(err, errUnreadable):
		return "Nothing could be read from the attachments"
	case errors.Is(err, purchases.ErrNotEditable):
		return "The document was booked meanwhile"
	case errors.As(err, &verr):
		return "What was read could not be filled in: " + verr.Error()
	}
	return "The attachments could not be read"
}

// fill reads the document's attachments and fills it in.
func (s *Service) fill(ctx context.Context, r *Recognition) error {
	admin, id, user := r.AdministrationID, r.DocumentID, r.UserID
	inc, err := s.Documents.IncomingDocument(ctx, admin, id, user)
	if err != nil {
		return err
	}
	var t Target
	var files []attachments.Attachment
	switch inc.Kind {
	case purchases.TypelessDocument:
		doc, err := s.Documents.TypelessDocument(ctx, admin, id, user)
		if err != nil {
			return err
		}
		files = doc.Attachments
		t = Target{Kind: inc.Kind, ContactID: doc.ContactID, Reference: doc.Reference, Date: doc.Date}
	case purchases.PurchaseInvoice, purchases.Receipt:
		doc, err := s.Documents.Document(ctx, admin, inc.Kind, id, user)
		if err != nil {
			return err
		}
		if doc.Booked() {
			return errNothingToRead
		}
		files = doc.Attachments
		t = Target{
			Kind: inc.Kind, ContactID: doc.ContactID, Reference: doc.Reference, Date: doc.Date, DueDate: doc.DueDate,
			Currency: doc.Currency, PricesAreInclTax: doc.PricesAreInclTax, Lines: doc.Lines,
		}
	default:
		return errNothingToRead
	}
	if len(files) == 0 {
		return errNothingToRead
	}
	read, err := s.load(ctx, admin, user, inc.Kind, id, files)
	if err != nil {
		return err
	}
	e, err := s.extract(ctx, read)
	if err != nil {
		return err
	}
	supplier, err := s.supplier(ctx, admin, user, e)
	if err != nil {
		return err
	}
	var memory Memory
	if supplier.ContactID != 0 {
		if memory, err = s.memory(ctx, admin, user, supplier.ContactID, 0); err != nil {
			return err
		}
	}
	purchase, active := "purchase_invoice", true
	rates, err := s.Ledger.TaxRates(ctx, admin, user, bookkeeping.TaxRateFilter{Type: &purchase, Active: &active})
	if err != nil {
		return err
	}
	plan := MakePlan(PlanInput{Extraction: e, Target: t, Previous: *r, Supplier: supplier, Memory: memory, Rates: rates})
	d, err := s.Documents.Propose(ctx, admin, id, user, plan.Kind, plan.Changes)
	var verr *resource.ValidationError
	if errors.As(err, &verr) {
		// A remembered category may have been removed or deactivated since;
		// the lines then go uncategorised.
		plan = plan.withoutCategories()
		d, err = s.Documents.Propose(ctx, admin, id, user, plan.Kind, plan.Changes)
	}
	if err != nil {
		return err
	}
	r.Source, r.Fields = e.Source, s.describeSources(ctx, admin, user, plan.Fields)
	if s.Notifier != nil {
		s.Notifier.DocumentUpdated(ctx, admin, d)
	}
	return nil
}

// withoutCategories leaves the categories of the lines out.
func (p Plan) withoutCategories() Plan {
	details := slices.Clone(p.Changes.Details)
	for i := range details {
		details[i].LedgerAccountID = nil
	}
	p.Changes.Details = details
	p.Fields = slices.DeleteFunc(slices.Clone(p.Fields), func(f Field) bool { return strings.HasSuffix(f.Name, ".ledger_account_id") })
	return p
}

// load reads the files of the attachments.
func (s *Service) load(ctx context.Context, admin, user resource.ID, kind string, id resource.ID, list []attachments.Attachment) ([]ocr.File, error) {
	var out []ocr.File
	for _, a := range list {
		var rc io.ReadCloser
		var err error
		if kind == purchases.TypelessDocument {
			_, rc, err = s.Documents.OpenTypelessAttachment(ctx, admin, id, a.ID, user)
		} else {
			_, rc, err = s.Documents.OpenAttachment(ctx, admin, user, kind, id, a.ID)
		}
		if err != nil {
			return nil, fmt.Errorf("open attachment %s: %w", a.ID, err)
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxFileSize))
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read attachment %s: %w", a.ID, err)
		}
		out = append(out, ocr.File{Name: a.Filename, ContentType: a.ContentType, Data: data})
	}
	return out, nil
}

// supplier finds the contact the document is from, or adds one with what
// the document says of it.
func (s *Service) supplier(ctx context.Context, admin, user resource.ID, e Extraction) (SupplierMatch, error) {
	sup := e.Supplier
	var candidates []contacts.Contact
	seen := map[resource.ID]bool{}
	for _, term := range sup.SearchTerms() {
		page, err := s.Contacts.List(ctx, admin, user, contacts.Query{Search: term, PerPage: contacts.MaxPerPage})
		if err != nil {
			return SupplierMatch{}, fmt.Errorf("search contacts: %w", err)
		}
		for _, c := range page.Contacts {
			if !seen[c.ID] {
				seen[c.ID] = true
				candidates = append(candidates, c)
			}
		}
	}
	if c, by, ok := Match(sup, candidates); ok {
		return SupplierMatch{ContactID: c.ID, Name: c.Name(), Confidence: by.Confidence()}, nil
	}
	if e.Source != SourceEInvoice {
		sup = s.fromRegistry(ctx, sup)
	}
	name := strings.TrimSpace(sup.Name)
	if name == "" {
		return SupplierMatch{}, nil
	}
	c, err := s.Contacts.Create(ctx, admin, user, newContact(sup))
	var verr *resource.ValidationError
	if errors.As(err, &verr) {
		// What the document says may not pass as contact details, such as
		// a VAT number in a format of another country: keep the name.
		c, err = s.Contacts.Create(ctx, admin, user, contacts.Changes{CompanyName: &name})
	}
	if err != nil {
		return SupplierMatch{}, fmt.Errorf("add supplier: %w", err)
	}
	if s.Notifier != nil {
		s.Notifier.ContactCreated(ctx, admin, c)
	}
	conf := Uncertain
	if e.Source == SourceEInvoice {
		conf = Certain
	}
	return SupplierMatch{ContactID: c.ID, Name: c.Name(), Confidence: conf}, nil
}

// newContact holds what a document says of its supplier.
func newContact(sup Supplier) contacts.Changes {
	text := func(v string) *string {
		if v = strings.TrimSpace(v); v != "" {
			return &v
		}
		return nil
	}
	c := contacts.Changes{
		CompanyName: text(sup.Name), Address1: text(sup.Address1), Zipcode: text(sup.Zipcode), City: text(sup.City),
		Phone: text(sup.Phone), TaxNumber: text(NormalizeVAT(sup.VATNumber)), SendInvoicesToEmail: text(sup.Email),
	}
	if country := strings.ToUpper(strings.TrimSpace(sup.Country)); len(country) == 2 {
		c.Country = &country
	}
	if kvk := NormalizeKVK(sup.KVK); len(kvk) == 8 {
		c.ChamberOfCommerce = &kvk
	}
	if account := iban.Normalize(sup.IBAN); account != "" && iban.Valid(account) {
		c.SepaIBAN = &account
	}
	return c
}

// model is the AI model, asking the connections in order, when an
// administrator turned it on.
func (s *Service) model(ctx context.Context) (Model, error) {
	if s.Settings == nil || s.Models == nil {
		return nil, nil
	}
	inst, err := s.Settings.Instance(ctx)
	if err != nil || !inst.Recognition.AIEnabled {
		return nil, err
	}
	return s.Models(inst.Recognition)
}

// AI states of a check.
const (
	AIOff         = "off"
	AIOK          = "ok"
	AIUnavailable = "unavailable"
)

// Check tells what Scan and Recognise can do on this server.
type Check struct {
	OCRAvailable bool
	// AI is AIOff, AIOK (a connection answers) or AIUnavailable.
	AI          string
	Connections []ConnectionCheck
	Message     string
}

// Connection states of a check.
const (
	ConnectionOK          = "ok"
	ConnectionUnavailable = "unavailable"
	ConnectionDisabled    = "disabled"
)

// ConnectionCheck tells whether one AI connection answers.
type ConnectionCheck struct {
	ID, Name string
	// State is ConnectionOK, ConnectionUnavailable or ConnectionDisabled.
	State   string
	Message string
}

// Check checks whether the OCR service and each enabled AI connection
// answer. Connections are checked even while the AI model is off, so an
// administrator can try them before turning it on.
func (s *Service) Check(ctx context.Context) (Check, error) {
	var c Check
	var problems []string
	if s.OCR == nil {
		problems = append(problems, "The OCR service is not configured.")
	} else if err := s.OCR.Ping(ctx); err != nil {
		problems = append(problems, "The OCR service does not answer: "+err.Error())
	} else {
		c.OCRAvailable = true
	}
	inst, err := s.Settings.Instance(ctx)
	if err != nil {
		return Check{}, err
	}
	anyOK := false
	for _, conn := range inst.Recognition.Connections {
		cc := ConnectionCheck{ID: conn.ID, Name: conn.Name, State: ConnectionDisabled}
		if conn.Enabled {
			cc.State = ConnectionUnavailable
			if err := s.pingConnection(ctx, conn); err != nil {
				cc.Message = err.Error()
			} else {
				cc.State = ConnectionOK
				anyOK = true
			}
		}
		c.Connections = append(c.Connections, cc)
	}
	switch {
	case !inst.Recognition.AIEnabled:
		c.AI = AIOff
	case anyOK:
		c.AI = AIOK
	default:
		c.AI = AIUnavailable
		problems = append(problems, "No AI connection answers.")
		for _, cc := range c.Connections {
			if cc.State == ConnectionUnavailable {
				problems = append(problems, cc.Name+": "+cc.Message)
			}
		}
	}
	c.Message = strings.Join(problems, " ")
	return c, nil
}

func (s *Service) pingConnection(ctx context.Context, conn settings.AIConnection) error {
	if s.Models == nil {
		return errors.New("this server cannot ask an AI model")
	}
	m, err := s.Models(settings.Recognition{AIEnabled: true, Connections: []settings.AIConnection{conn}})
	if err != nil {
		return err
	}
	return m.Ping(ctx)
}
