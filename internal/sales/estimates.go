package sales

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// EstimateEntityType names estimates in notes, attachments and events.
const EstimateEntityType = "Estimate"

// Estimate states beyond the invoice ones, with Moneybird's names.
const (
	Accepted = "accepted"
	Rejected = "rejected"
	Billed   = "billed"
	Archived = "archived"
)

// EstimateStates are every estimate state, for filters. Late is worked out
// from the due date: an open estimate past it is late.
var EstimateStates = []string{Draft, Open, Late, Accepted, Rejected, Billed, Archived}

// estimateTransitions are the states change_state may move an estimate to,
// by its current state. A draft only leaves draft by being sent.
var estimateTransitions = map[string][]string{
	Open:     {Late, Accepted, Rejected, Billed, Archived},
	Late:     {Open, Accepted, Rejected, Billed, Archived},
	Accepted: {Open, Rejected, Billed, Archived},
	Rejected: {Open, Accepted, Archived},
	Billed:   {Open, Accepted, Archived},
	Archived: {Open, Accepted, Rejected, Billed},
}

// estimateTargets are the states change_state accepts.
var estimateTargets = []string{Accepted, Rejected, Open, Late, Billed, Archived}

// billable are the states an estimate can be billed in.
var billable = []string{Open, Late, Accepted}

// Languages are the languages a document can be in, after Moneybird.
var Languages = []string{"bg", "cs", "da", "de", "el", "en", "es", "fr", "hr", "hu", "it", "no", "nl", "nl-be", "pl", "pt", "ro", "sv", "tr", "uk"}

// EstimateDeliveryMethods are the ways an estimate can be sent.
var EstimateDeliveryMethods = []string{"Email", "Manual"}

// Errors about estimates.
var (
	ErrEstimateNotFound = errors.New("estimate not found")
	// ErrEstimateNotEditable reports a billed or archived estimate.
	ErrEstimateNotEditable = errors.New("estimate cannot be changed in this state")
	// ErrEstimateNoDetails reports sending an estimate without lines.
	ErrEstimateNoDetails = errors.New("details cannot be empty")
	// ErrEstimateNotBillable reports billing an estimate that is not open,
	// late or accepted, or whose contact is gone.
	ErrEstimateNotBillable = errors.New("estimate cannot be billed")
	// ErrEstimateNotSigned reports asking for the signed PDF of an estimate
	// the contact did not sign online; MoneyHarpy has no online signing yet.
	ErrEstimateNotSigned = errors.New("estimate is not signed")
	// ErrEstimateDelivery reports an estimate that was sent but could not
	// be emailed; it can be sent again.
	ErrEstimateDelivery = errors.New("estimate could not be delivered")
)

// StateError reports a state change_state cannot make: one that is no
// option at all (From empty), or not from the current state.
type StateError struct{ From, To string }

func (e *StateError) Error() string {
	if e.From == "" {
		return "state " + e.To + " isn't an option"
	}
	return "state cannot change from " + e.From + " to " + e.To
}

// Symbol is Moneybird's symbolic name of the problem.
func (e *StateError) Symbol() string {
	if e.From == "" {
		return "in"
	}
	return "can_not_change_" + e.From + "_to_" + e.To
}

// Estimate is a quote: lines and totals like a sales invoice, sent to a
// contact, who accepts or rejects it; an accepted one is billed.
type Estimate struct {
	ID               resource.ID
	AdministrationID resource.ID
	ContactID        resource.ID
	ContactPersonID  *resource.ID
	IdentityID       resource.ID
	DocumentStyleID  resource.ID
	WorkflowID       resource.ID
	// DraftID numbers drafts; EstimateID is the number given when sent.
	DraftID    int
	EstimateID string
	// SequenceID overrides the {id} part of the number when sending.
	SequenceID       string
	Reference        string
	Language         string
	Currency         string
	PricesAreInclTax bool
	ShowTax          bool
	SignOnline       bool
	// Discount is a percentage in hundredths.
	Discount     int64
	EstimateDate *time.Time
	// DueDate is when the estimate expires.
	DueDate          *time.Time
	FirstDueInterval int
	PreText          string
	PostText         string
	State            string
	SentAt           *time.Time
	AcceptedAt       *time.Time
	RejectedAt       *time.Time
	ArchivedAt       *time.Time
	// OriginalEstimateID is the estimate this one copies; OriginalSalesInvoiceID
	// the invoice it was made from.
	OriginalEstimateID     *resource.ID
	OriginalSalesInvoiceID *resource.ID
	// Delivery remembers how the estimate was last sent.
	Delivery     *Delivery
	Lines        []Detail
	CustomFields []FieldValue
	Notes        []notes.Note
	Attachments  []attachments.Attachment
	// Totals are the amounts worked out when the estimate was last saved.
	Totals    Totals
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Ref locates the estimate.
func (e Estimate) Ref() resource.Ref {
	return resource.Ref{ID: e.ID, Type: resource.TypeEstimate, AdministrationID: e.AdministrationID}
}

// Version changes whenever the estimate does.
func (e Estimate) Version() int64 { return e.UpdatedAt.Unix() }

// invoice is the estimate as the invoice code works on it.
func (e Estimate) invoice() Invoice {
	return Invoice{
		ID: e.ID, AdministrationID: e.AdministrationID, Type: EstimateEntityType, ContactID: e.ContactID, ContactPersonID: e.ContactPersonID,
		IdentityID: e.IdentityID, DocumentStyleID: e.DocumentStyleID, WorkflowID: e.WorkflowID, DraftID: e.DraftID, InvoiceID: e.EstimateID,
		SequenceID: e.SequenceID, Reference: e.Reference, Language: e.Language, Currency: e.Currency, PricesAreInclTax: e.PricesAreInclTax,
		Discount: e.Discount, InvoiceDate: e.EstimateDate, DueDate: e.DueDate, FirstDueInterval: e.FirstDueInterval, State: e.State,
		SentAt: e.SentAt, Lines: slices.Clone(e.Lines), CustomFields: slices.Clone(e.CustomFields), Notes: e.Notes, Attachments: e.Attachments,
		Totals: e.Totals, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}
}

// take copies what the invoice code changed of inv, as invoice() returned it.
func (e *Estimate) take(inv Invoice) {
	e.ContactID, e.ContactPersonID, e.IdentityID, e.DocumentStyleID, e.WorkflowID = inv.ContactID, inv.ContactPersonID, inv.IdentityID, inv.DocumentStyleID, inv.WorkflowID
	e.SequenceID, e.Reference, e.Language, e.Currency, e.PricesAreInclTax = inv.SequenceID, inv.Reference, inv.Language, inv.Currency, inv.PricesAreInclTax
	e.Discount, e.EstimateDate, e.FirstDueInterval = inv.Discount, inv.InvoiceDate, inv.FirstDueInterval
	e.Lines, e.CustomFields = inv.Lines, inv.CustomFields
}

// workflowType is the kind of workflow the document follows.
func workflowType(inv Invoice) invoicing.WorkflowType {
	if inv.Type == EstimateEntityType {
		return invoicing.EstimateWorkflow
	}
	return invoicing.InvoiceWorkflow
}

// effectiveEstimate is the state to show: an open estimate past its due
// date is late.
func effectiveEstimate(e Estimate, today time.Time) string {
	if e.State == Open && e.DueDate != nil && e.DueDate.Before(today) {
		return Late
	}
	return e.State
}

// EstimateStore persists estimates.
type EstimateStore interface {
	Estimates(ctx context.Context, admin resource.ID, q EstimateQuery) ([]Estimate, int, error)
	Estimate(ctx context.Context, admin, id resource.ID) (Estimate, error)
	EstimatesByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]Estimate, error)
	EstimateByNumber(ctx context.Context, admin resource.ID, number string) (Estimate, error)
	NextEstimateDraftID(ctx context.Context, admin resource.ID) (int, error)
	// ReserveEstimateNumber takes the next estimate sequence number and
	// moves the counter on, at once.
	ReserveEstimateNumber(ctx context.Context, admin resource.ID) (string, error)
	EstimateNumberTaken(ctx context.Context, admin resource.ID, number string, except resource.ID) (bool, error)
	SaveEstimate(ctx context.Context, e Estimate, created bool) error
	// DeleteEstimate removes the estimate with its notes; invoices billed
	// from it stay.
	DeleteEstimate(ctx context.Context, admin, id resource.ID) error
	TouchEstimate(ctx context.Context, admin, id resource.ID, at time.Time) error
	// SaveBilled stores, at once, the draft invoice billed from an estimate
	// and the estimate, now billed.
	SaveBilled(ctx context.Context, inv Invoice, e Estimate) error
}

// EstimateChanges holds the fields to set; nil fields stay as they are. A
// zero id clears an optional one.
type EstimateChanges struct {
	ContactID              *resource.ID
	ContactPersonID        *resource.ID
	DocumentStyleID        *resource.ID
	WorkflowID             *resource.ID
	IdentityID             *resource.ID
	Reference              *string
	EstimateDate           *string
	SequenceID             *string
	Language               *string
	Currency               *string
	PricesAreInclTax       *bool
	ShowTax                *bool
	FirstDueInterval       *int
	PreText                *string
	PostText               *string
	Discount               *string
	OriginalSalesInvoiceID *resource.ID
	Details                []DetailInput
	CustomFields           []FieldValue
}

func (c EstimateChanges) invoiceChanges() Changes {
	return Changes{
		ContactID: c.ContactID, ContactPersonID: c.ContactPersonID, DocumentStyleID: c.DocumentStyleID, WorkflowID: c.WorkflowID,
		IdentityID: c.IdentityID, Reference: c.Reference, InvoiceDate: c.EstimateDate, FirstDueInterval: c.FirstDueInterval,
		Currency: c.Currency, PricesAreInclTax: c.PricesAreInclTax, Discount: c.Discount, SequenceID: c.SequenceID,
		Details: c.Details, CustomFields: c.CustomFields,
	}
}

// estimateFields renames what the invoice code calls a field.
var estimateFields = map[string]string{"invoice_date": "estimate_date", "invoice_sequence_id": "estimate_sequence_id"}

// CreateEstimate adds a draft estimate. Without a workflow it follows the
// contact's estimate workflow, else the default one; the document style
// and identity are the defaults unless given.
func (s *Service) CreateEstimate(ctx context.Context, admin, user resource.ID, c EstimateChanges) (Estimate, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Estimate{}, err
	}
	if c.WorkflowID == nil && c.ContactID != nil {
		if contact, err := s.contacts.Contact(ctx, admin, *c.ContactID, user); err == nil && contact.EstimateWorkflowID != nil {
			c.WorkflowID = contact.EstimateWorkflowID
		}
	}
	now := s.now()
	e := Estimate{ID: s.ids.Next(), AdministrationID: admin, State: Draft, Currency: a.Currency, Language: a.Language, ShowTax: true, SignOnline: true, CreatedAt: now, UpdatedAt: now}
	inv := e.invoice()
	if err := s.defaults(ctx, a, user, &inv, c.invoiceChanges()); err != nil {
		return Estimate{}, err
	}
	e.take(inv)
	if e.DraftID, err = s.store.NextEstimateDraftID(ctx, admin); err != nil {
		return Estimate{}, wrap("number draft", err)
	}
	if err := s.applyEstimate(ctx, a, user, &e, c); err != nil {
		return Estimate{}, err
	}
	if err := s.saveEstimate(ctx, admin, user, &e, true); err != nil {
		return Estimate{}, err
	}
	return s.Estimate(ctx, admin, e.ID, user)
}

// UpdateEstimate changes an estimate that is not billed or archived. A
// sent estimate gets a new due date when its date or validity changes.
func (s *Service) UpdateEstimate(ctx context.Context, admin, id, user resource.ID, c EstimateChanges) (Estimate, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Estimate{}, err
	}
	e, err := s.store.Estimate(ctx, admin, id)
	if err != nil {
		return Estimate{}, err
	}
	if e.State == Billed || e.State == Archived {
		return Estimate{}, ErrEstimateNotEditable
	}
	if err := s.applyEstimate(ctx, a, user, &e, c); err != nil {
		return Estimate{}, err
	}
	e.UpdatedAt = s.now()
	if err := s.saveEstimate(ctx, admin, user, &e, false); err != nil {
		return Estimate{}, err
	}
	return s.Estimate(ctx, admin, id, user)
}

func (s *Service) applyEstimate(ctx context.Context, a administration.Listed, user resource.ID, e *Estimate, c EstimateChanges) error {
	inv := e.invoice()
	var verr resource.ValidationError
	if err := s.apply(ctx, a, user, &inv, c.invoiceChanges()); err != nil {
		var invalid *resource.ValidationError
		if !errors.As(err, &invalid) {
			return err
		}
		for field, problems := range invalid.Fields {
			for _, p := range problems {
				verr.Add(cmpString(estimateFields[field], field), p)
			}
		}
	}
	e.take(inv)
	if c.Language != nil {
		if l := strings.TrimSpace(*c.Language); slices.Contains(Languages, l) {
			e.Language = l
		} else {
			verr.Add("language", "is not included in the list")
		}
	}
	if c.ShowTax != nil {
		e.ShowTax = *c.ShowTax
	}
	for _, f := range []struct {
		field string
		in    *string
		out   *string
	}{{"pre_text", c.PreText, &e.PreText}, {"post_text", c.PostText, &e.PostText}} {
		if f.in == nil {
			continue
		}
		if len(*f.in) > 10000 {
			verr.Add(f.field, "is too long (maximum is 10000 characters)")
			continue
		}
		*f.out = strings.TrimSpace(*f.in)
	}
	if c.OriginalSalesInvoiceID != nil {
		e.OriginalSalesInvoiceID = optional(*c.OriginalSalesInvoiceID)
		if e.OriginalSalesInvoiceID != nil {
			if _, err := s.store.Invoice(ctx, a.ID, *e.OriginalSalesInvoiceID); err != nil {
				verr.Add("original_sales_invoice_id", "does not exist")
			}
		}
	}
	if e.State != Draft {
		if e.EstimateDate == nil {
			verr.Add("estimate_date", "can't be blank on a sent estimate")
		} else if e.DueDate == nil || c.EstimateDate != nil || c.FirstDueInterval != nil || c.WorkflowID != nil {
			due := e.EstimateDate.AddDate(0, 0, e.FirstDueInterval)
			e.DueDate = &due
		}
	}
	return verr.OrNil()
}

func (s *Service) saveEstimate(ctx context.Context, admin, user resource.ID, e *Estimate, created bool) error {
	tot, err := s.Totals(ctx, admin, user, e.invoice())
	if err != nil {
		return err
	}
	e.Totals = tot
	return wrap("store estimate", s.store.SaveEstimate(ctx, *e, created))
}

// DeleteEstimate removes an estimate with its notes and files; invoices
// billed from it stay.
func (s *Service) DeleteEstimate(ctx context.Context, admin, id, user resource.ID) error {
	if _, err := s.Estimate(ctx, admin, id, user); err != nil {
		return err
	}
	if err := s.attachments.DeleteAll(ctx, admin, EstimateEntityType, id); err != nil {
		return wrap("delete attachments", err)
	}
	return s.store.DeleteEstimate(ctx, admin, id)
}

// EstimateSendInput is how to send an estimate; empty fields take the
// previous sending's, then the contact's.
type EstimateSendInput struct {
	DeliveryMethod string
	EmailAddress   string
	EmailMessage   string
	SignOnline     *bool
}

// SendEstimate sends an estimate: a draft gets its number, date and due
// date and becomes open; a sent one goes out again in its state. Email
// sends the PDF; Manual leaves sending to the user.
func (s *Service) SendEstimate(ctx context.Context, admin, id, user resource.ID, in EstimateSendInput) (Estimate, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Estimate{}, err
	}
	e, err := s.store.Estimate(ctx, admin, id)
	if err != nil {
		return Estimate{}, err
	}
	if len(e.Lines) == 0 {
		return Estimate{}, ErrEstimateNoDetails
	}
	contact, err := s.contacts.Contact(ctx, admin, e.ContactID, user)
	if err != nil {
		return Estimate{}, invalid("contact_id", "does not exist")
	}
	settings, err := s.invoicing.Settings(ctx, admin, user)
	if err != nil {
		return Estimate{}, wrap("load invoice settings", err)
	}
	delivery, err := resolveEstimateDelivery(in, e.Delivery, contact)
	if err != nil {
		return Estimate{}, err
	}
	if delivery.Method == "Email" {
		if sender, err := s.admins.Identity(ctx, admin, e.IdentityID, user); err != nil || strings.TrimSpace(sender.Email) == "" {
			return Estimate{}, invalid("delivery_method", "The sender address must contain an email address")
		}
	}
	now := s.now()
	if e.State == Draft {
		day := s.today(a)
		if e.EstimateDate != nil {
			day = *e.EstimateDate
		}
		e.EstimateDate = &day
		if err := s.numberEstimate(ctx, admin, &e, contact, settings.Settings); err != nil {
			return Estimate{}, err
		}
		due := day.AddDate(0, 0, e.FirstDueInterval)
		e.DueDate, e.State, e.SentAt = &due, Open, &now
	}
	if in.SignOnline != nil {
		e.SignOnline = *in.SignOnline
	}
	e.Delivery, e.UpdatedAt = &delivery, now
	if err := s.saveEstimate(ctx, admin, user, &e, false); err != nil {
		return Estimate{}, err
	}
	if delivery.Method == "Email" {
		if err := s.deliverEstimate(ctx, a, user, e, delivery, settings.Settings); err != nil {
			return Estimate{}, fmt.Errorf("%w: %v", ErrEstimateDelivery, err)
		}
	}
	return s.Estimate(ctx, admin, id, user)
}

// resolveEstimateDelivery works out how to send: estimates go by email or
// by hand, so a contact that gets e-invoices or post gets email.
func resolveEstimateDelivery(in EstimateSendInput, previous *Delivery, contact contacts.Contact) (Delivery, error) {
	d := Delivery{Method: contact.DeliveryMethod, EmailAddress: cmpString(contact.SendEstimatesToEmail, contact.SendInvoicesToEmail)}
	if previous != nil {
		d = *previous
	}
	if d.Method != "Manual" {
		d.Method = "Email"
	}
	if in.DeliveryMethod != "" {
		if !slices.Contains(EstimateDeliveryMethods, in.DeliveryMethod) {
			return d, invalid("delivery_method", "is not included in the list")
		}
		d.Method = in.DeliveryMethod
	}
	if in.EmailAddress != "" {
		d.EmailAddress = strings.TrimSpace(in.EmailAddress)
	}
	d.EmailMessage, d.DeliverUBL, d.Mergeable = in.EmailMessage, false, false
	switch {
	case d.Method == "Email" && d.EmailAddress == "":
		return d, invalid("email_address", "can't be blank")
	case d.Method == "Email":
		for addr := range strings.SplitSeq(d.EmailAddress, ",") {
			if _, err := mail.ParseAddress(strings.TrimSpace(addr)); err != nil {
				return d, invalid("email_address", "is not a valid email address")
			}
		}
	}
	if len(d.EmailMessage) > 10000 {
		return d, invalid("email_message", "is too long (maximum is 10000 characters)")
	}
	return d, nil
}

// numberEstimate gives a draft estimate its number from the estimate
// numbering settings.
func (s *Service) numberEstimate(ctx context.Context, admin resource.ID, e *Estimate, contact contacts.Contact, settings invoicing.Settings) error {
	render := func(seq string) string {
		return invoicing.RenderNumber(settings.EstimateIDTemplate, seq, *e.EstimateDate, e.Language, contact.CustomerID)
	}
	try := func(seq string) (bool, error) {
		number := render(seq)
		taken, err := s.store.EstimateNumberTaken(ctx, admin, number, e.ID)
		if err != nil || taken {
			return false, wrap("check estimate number", err)
		}
		e.EstimateID = number
		return true, nil
	}
	if e.SequenceID != "" {
		ok, err := try(e.SequenceID)
		if err == nil && !ok {
			return invalid("estimate_sequence_id", "gives the number "+render(e.SequenceID)+", which another estimate has")
		}
		return err
	}
	for range 20 {
		seq, err := s.store.ReserveEstimateNumber(ctx, admin)
		if err != nil {
			return wrap("reserve estimate number", err)
		}
		if ok, err := try(seq); err != nil || ok {
			return err
		}
	}
	return invalid("estimate_sequence_id", "could not find a free estimate number; change the sequence in the estimate settings")
}

// deliverEstimate emails the estimate with its PDF.
func (s *Service) deliverEstimate(ctx context.Context, a administration.Listed, user resource.ID, e Estimate, d Delivery, settings invoicing.Settings) error {
	doc, err := s.estimateDocument(ctx, a, user, e)
	if err != nil {
		return err
	}
	pdf, err := s.renderer.PDF(doc)
	if err != nil {
		return wrap("render estimate", err)
	}
	subject, body := EstimateEmailText(doc)
	if strings.TrimSpace(d.EmailMessage) != "" {
		body = ExpandEstimate(d.EmailMessage, doc)
	}
	return s.mailer.SendDocument(ctx, Mail{
		To: splitAddresses(d.EmailAddress), BCC: splitAddresses(settings.EstimateBCC), ReplyTo: doc.Identity.Email, FromName: doc.Identity.CompanyName,
		Subject: subject, Body: body, Files: []File{{Name: estimateFileName(e, "pdf"), ContentType: "application/pdf", Data: pdf}},
		Look: s.emailLook(ctx, a.ID, user, doc, EstimateDetails(doc)),
	})
}

// DefaultEstimateEmail returns the estimate email MoneyHarpy sends when the
// workflow has none of its own.
func DefaultEstimateEmail(language string) (subject, body string) {
	if strings.HasPrefix(language, "nl") {
		return "Offerte {estimate_id} van {company_name}",
			"Beste {contact_name},\n\nIn de bijlage vind je offerte {estimate_id} van {estimate_date} voor {total}. " +
				"De offerte is geldig tot {due_date}.\n\nMet vriendelijke groet,\n{company_name}"
	}
	return "Quote {estimate_id} from {company_name}",
		"Dear {contact_name},\n\nPlease find attached quote {estimate_id} of {estimate_date} for {total}. " +
			"The quote is valid until {due_date}.\n\nKind regards,\n{company_name}"
}

// EstimateEmailText is the subject and body of the estimate email: the
// workflow's template, or the default for the estimate's language.
func EstimateEmailText(doc Document) (string, string) {
	subject, body := DefaultEstimateEmail(doc.Invoice.Language)
	if t := strings.TrimSpace(doc.Workflow.InvoiceEmailSubject); t != "" {
		subject = t
	}
	if t := strings.TrimSpace(doc.Workflow.InvoiceEmailBody); t != "" {
		body = t
	}
	return ExpandEstimate(subject, doc), ExpandEstimate(body, doc)
}

// ExpandEstimate fills in an estimate email's placeholders; the invoice
// ones work too, {invoice_id} naming the estimate number as at Moneybird.
func ExpandEstimate(template string, doc Document) string {
	date := ""
	if d := doc.Invoice.InvoiceDate; d != nil {
		date = d.Format("02-01-2006")
	}
	template = strings.NewReplacer("{estimate_id}", doc.Invoice.InvoiceID, "{estimate_date}", date).Replace(template)
	return Expand(template, doc)
}

func estimateFileName(e Estimate, ext string) string {
	name := e.EstimateID
	if name == "" {
		name = fmt.Sprintf("draft-%d", e.DraftID)
	}
	return "estimate-" + strings.NewReplacer("/", "-", `\`, "-", " ", "-").Replace(name) + "." + ext
}

// estimateDocument gathers what the estimate's PDF shows.
func (s *Service) estimateDocument(ctx context.Context, a administration.Listed, user resource.ID, e Estimate) (Document, error) {
	doc, err := s.document(ctx, a, user, e.invoice())
	if err != nil {
		return Document{}, err
	}
	doc.Estimate = &e
	return doc, nil
}

// EstimatePDF renders the estimate, with the file name to save it under.
func (s *Service) EstimatePDF(ctx context.Context, admin, id, user resource.ID, stationery bool) ([]byte, string, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, "", err
	}
	e, err := s.Estimate(ctx, admin, id, user)
	if err != nil {
		return nil, "", err
	}
	doc, err := s.estimateDocument(ctx, a, user, e)
	if err != nil {
		return nil, "", err
	}
	doc.Stationery = stationery
	data, err := s.renderer.PDF(doc)
	return data, estimateFileName(e, "pdf"), wrap("render estimate", err)
}

// SignedEstimatePDF is the PDF the contact signed online. MoneyHarpy has no
// online signing yet, so no estimate is signed.
func (s *Service) SignedEstimatePDF(ctx context.Context, admin, id, user resource.ID) ([]byte, string, error) {
	if _, err := s.Estimate(ctx, admin, id, user); err != nil {
		return nil, "", err
	}
	return nil, "", ErrEstimateNotSigned
}

// ChangeEstimateState moves an estimate to another state, without sending
// or billing it. Not every change is allowed: a draft only leaves draft by
// being sent.
func (s *Service) ChangeEstimateState(ctx context.Context, admin, id, user resource.ID, state string) (Estimate, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Estimate{}, err
	}
	e, err := s.store.Estimate(ctx, admin, id)
	if err != nil {
		return Estimate{}, err
	}
	state = strings.TrimSpace(state)
	if !slices.Contains(estimateTargets, state) {
		return Estimate{}, &StateError{To: state}
	}
	today := s.today(a)
	from := effectiveEstimate(e, today)
	if from == state {
		return s.Estimate(ctx, admin, id, user)
	}
	if !slices.Contains(estimateTransitions[from], state) {
		return Estimate{}, &StateError{From: from, To: state}
	}
	e.State = state
	switch state {
	case Accepted:
		e.AcceptedAt, e.RejectedAt = &today, nil
	case Rejected:
		e.RejectedAt, e.AcceptedAt = &today, nil
	case Archived:
		e.ArchivedAt = &today
	case Open:
		e.AcceptedAt, e.RejectedAt = nil, nil
	}
	if state != Archived {
		e.ArchivedAt = nil
	}
	e.UpdatedAt = s.now()
	if err := wrap("store estimate", s.store.SaveEstimate(ctx, e, false)); err != nil {
		return Estimate{}, err
	}
	return s.Estimate(ctx, admin, id, user)
}

// BillEstimate makes a draft sales invoice of an open, late or accepted
// estimate, linked to it, and marks the estimate billed. The invoice
// follows the default invoice workflow but keeps the estimate's language,
// currency and prices, so its amounts are the estimate's.
func (s *Service) BillEstimate(ctx context.Context, admin, id, user resource.ID) (Invoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Invoice{}, err
	}
	e, err := s.store.Estimate(ctx, admin, id)
	if err != nil {
		return Invoice{}, err
	}
	if !slices.Contains(billable, effectiveEstimate(e, s.today(a))) {
		return Invoice{}, ErrEstimateNotBillable
	}
	if _, err := s.contacts.Contact(ctx, admin, e.ContactID, user); err != nil {
		return Invoice{}, ErrEstimateNotBillable
	}
	now := s.now()
	inv := Invoice{
		ID: s.ids.Next(), AdministrationID: admin, State: Draft, ContactID: e.ContactID, ContactPersonID: e.ContactPersonID,
		IdentityID: e.IdentityID, DocumentStyleID: e.DocumentStyleID, Reference: e.Reference, Discount: e.Discount,
		OriginalEstimateID: &e.ID, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.defaults(ctx, a, user, &inv, Changes{IdentityID: &e.IdentityID, DocumentStyleID: &e.DocumentStyleID}); err != nil {
		return Invoice{}, err
	}
	inv.Language, inv.Currency, inv.PricesAreInclTax = e.Language, e.Currency, e.PricesAreInclTax
	for _, d := range e.Lines {
		d.ID, d.CreatedAt, d.UpdatedAt = s.ids.Next(), now, now
		inv.Lines = append(inv.Lines, d)
	}
	if inv.DraftID, err = s.store.NextDraftID(ctx, admin); err != nil {
		return Invoice{}, wrap("number draft", err)
	}
	if inv.Totals, err = s.Totals(ctx, admin, user, inv); err != nil {
		return Invoice{}, err
	}
	e.State, e.UpdatedAt = Billed, now
	if err := s.store.SaveBilled(ctx, inv, e); err != nil {
		return Invoice{}, wrap("store billed estimate", err)
	}
	return s.Invoice(ctx, admin, inv.ID, user)
}

// EstimateQuery selects estimates, after Moneybird's filter.
type EstimateQuery struct {
	// States keeps estimates in one of them; open estimates past their due
	// date count as late, not open.
	States        []string
	Period        *money.Period
	References    []string
	ContactID     *resource.ID
	WorkflowID    *resource.ID
	CreatedAfter  *time.Time
	UpdatedAfter  *time.Time
	Page, PerPage int
	// Today decides which open estimates are late.
	Today time.Time
}

// ParseEstimateFilter reads Moneybird's estimate filter: state (all by
// default; several separated by |), period (this year when no filter is
// given), contact_id and workflow_id; reference, created_after and
// updated_after are MoneyHarpy extensions, as for sales invoices.
func ParseEstimateFilter(filter string, today time.Time) (EstimateQuery, error) {
	if strings.TrimSpace(filter) == "" {
		filter = "period:this_year"
	}
	q, err := parseTerms(filter, today, EstimateStates)
	if q.RecurringSalesInvoiceID != nil {
		var verr resource.ValidationError
		var prev *resource.ValidationError
		if errors.As(err, &prev) {
			verr = *prev
		}
		verr.Add("filter", "has an unknown key: recurring_sales_invoice_id")
		err = &verr
	}
	return EstimateQuery{
		States: q.States, Period: q.Period, References: q.References, ContactID: q.ContactID, WorkflowID: q.WorkflowID,
		CreatedAfter: q.CreatedAfter, UpdatedAfter: q.UpdatedAfter, Today: today,
	}, err
}

// Matches reports whether the query selects the estimate; the reference for
// stores without a query language of their own.
func (q EstimateQuery) Matches(e Estimate) bool {
	day := e.EstimateDate
	if day == nil {
		day = &e.CreatedAt
	}
	switch {
	case len(q.States) > 0 && !slices.Contains(q.States, effectiveEstimate(e, q.Today)),
		q.Period != nil && !q.Period.Contains(*day),
		len(q.References) > 0 && !slices.Contains(q.References, e.Reference),
		q.ContactID != nil && e.ContactID != *q.ContactID,
		q.WorkflowID != nil && e.WorkflowID != *q.WorkflowID,
		q.CreatedAfter != nil && !e.CreatedAt.After(*q.CreatedAfter),
		q.UpdatedAfter != nil && !e.UpdatedAt.After(*q.UpdatedAfter):
		return false
	}
	return true
}

// Estimates returns the page of estimates the query selects, newest first.
func (s *Service) Estimates(ctx context.Context, admin, user resource.ID, q EstimateQuery) ([]Estimate, int, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, 0, err
	}
	q.Today, q.Page = s.today(a), max(q.Page, 1)
	if q.PerPage <= 0 {
		q.PerPage = DefaultPerPage
	}
	q.PerPage = min(q.PerPage, MaxPerPage)
	list, total, err := s.store.Estimates(ctx, admin, q)
	if err != nil {
		return nil, 0, fmt.Errorf("list estimates: %w", err)
	}
	list, err = s.completeEstimates(ctx, admin, list, q.Today)
	return list, total, err
}

// EstimateVersions lists the estimates the query selects, for
// synchronization.
func (s *Service) EstimateVersions(ctx context.Context, admin, user resource.ID, q EstimateQuery) ([]Estimate, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	q.Today, q.PerPage = s.today(a), 0
	list, _, err := s.store.Estimates(ctx, admin, q)
	return list, err
}

// EstimatesByID returns the estimates with these ids, at most 100.
func (s *Service) EstimatesByID(ctx context.Context, admin, user resource.ID, ids []resource.ID) ([]Estimate, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	if len(ids) > MaxPerPage {
		ids = ids[:MaxPerPage]
	}
	list, err := s.store.EstimatesByID(ctx, admin, ids)
	if err != nil {
		return nil, err
	}
	return s.completeEstimates(ctx, admin, list, s.today(a))
}

// Estimate returns one estimate.
func (s *Service) Estimate(ctx context.Context, admin, id, user resource.ID) (Estimate, error) {
	return s.oneEstimate(ctx, admin, user, func() (Estimate, error) { return s.store.Estimate(ctx, admin, id) })
}

// EstimateByNumber returns the estimate with an estimate number.
func (s *Service) EstimateByNumber(ctx context.Context, admin, user resource.ID, number string) (Estimate, error) {
	return s.oneEstimate(ctx, admin, user, func() (Estimate, error) { return s.store.EstimateByNumber(ctx, admin, number) })
}

func (s *Service) oneEstimate(ctx context.Context, admin, user resource.ID, load func() (Estimate, error)) (Estimate, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Estimate{}, err
	}
	e, err := load()
	if err != nil {
		return Estimate{}, err
	}
	list, err := s.completeEstimates(ctx, admin, []Estimate{e}, s.today(a))
	if err != nil {
		return Estimate{}, err
	}
	return list[0], nil
}

// completeEstimates adds notes and attachments and shows late estimates
// as late.
func (s *Service) completeEstimates(ctx context.Context, admin resource.ID, list []Estimate, today time.Time) ([]Estimate, error) {
	ids := make([]resource.ID, len(list))
	for i, e := range list {
		ids[i] = e.ID
	}
	byID, err := s.notes.Of(ctx, admin, EstimateEntityType, ids)
	if err != nil {
		return nil, err
	}
	files, err := s.attachments.Of(ctx, admin, EstimateEntityType, ids)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Notes, list[i].Attachments = byID[list[i].ID], files[list[i].ID]
		list[i].State = effectiveEstimate(list[i], today)
	}
	return list, nil
}

// AddEstimateNote adds a note to an estimate.
func (s *Service) AddEstimateNote(ctx context.Context, admin, id, user resource.ID, in notes.Input) (notes.Note, error) {
	if _, err := s.Estimate(ctx, admin, id, user); err != nil {
		return notes.Note{}, err
	}
	n, err := s.notes.Add(ctx, admin, EstimateEntityType, id, user, in)
	if err != nil {
		return notes.Note{}, err
	}
	return n, s.store.TouchEstimate(ctx, admin, id, n.CreatedAt)
}

// DeleteEstimateNote removes a note from an estimate.
func (s *Service) DeleteEstimateNote(ctx context.Context, admin, id, noteID, user resource.ID) error {
	if _, err := s.Estimate(ctx, admin, id, user); err != nil {
		return err
	}
	if err := s.notes.Delete(ctx, admin, EstimateEntityType, id, noteID); err != nil {
		return err
	}
	return s.store.TouchEstimate(ctx, admin, id, s.now())
}

// AddEstimateAttachment attaches a file to an estimate.
func (s *Service) AddEstimateAttachment(ctx context.Context, admin, id, user resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error) {
	if _, err := s.Estimate(ctx, admin, id, user); err != nil {
		return attachments.Attachment{}, err
	}
	a, err := s.attachments.Add(ctx, admin, EstimateEntityType, id, filename, contentType, data)
	if err != nil {
		return attachments.Attachment{}, err
	}
	return a, s.store.TouchEstimate(ctx, admin, id, a.CreatedAt)
}

// OpenEstimateAttachment returns an attachment of an estimate; the caller
// closes it.
func (s *Service) OpenEstimateAttachment(ctx context.Context, admin, id, attachment, user resource.ID) (attachments.Attachment, io.ReadCloser, error) {
	if _, err := s.Estimate(ctx, admin, id, user); err != nil {
		return attachments.Attachment{}, nil, err
	}
	return s.attachments.Open(ctx, admin, EstimateEntityType, id, attachment)
}

// DeleteEstimateAttachment removes an attachment from an estimate.
func (s *Service) DeleteEstimateAttachment(ctx context.Context, admin, id, attachment, user resource.ID) error {
	if _, err := s.Estimate(ctx, admin, id, user); err != nil {
		return err
	}
	if err := s.attachments.Delete(ctx, admin, EstimateEntityType, id, attachment); err != nil {
		return err
	}
	return s.store.TouchEstimate(ctx, admin, id, s.now())
}
