package sales

import (
	"context"
	"fmt"
	"io"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// DeliveryMethods are the ways an invoice can be sent.
var DeliveryMethods = []string{"Email", "Manual", "Post", "Peppol", "Simplerinvoicing"}

// SendInput is how to send an invoice; empty fields take the previous
// sending's, then the contact's and the workflow's defaults.
type SendInput struct {
	DeliveryMethod string
	// Scheduled sends on InvoiceDate, which must lie in the future.
	Scheduled    bool
	DeliverUBL   *bool
	Mergeable    *bool
	EmailAddress string
	EmailMessage string
	InvoiceDate  string
}

// Document is everything an invoice's PDF, packing slip and e-invoice show.
type Document struct {
	Administration administration.Administration
	Invoice        Invoice
	Totals         Totals
	Rates          map[resource.ID]bookkeeping.TaxRate
	Contact        contacts.Contact
	ContactPerson  *contacts.Person
	Identity       administration.Identity
	Style          invoicing.DocumentStyle
	Workflow       invoicing.Workflow
	// Logo is the document style's logo, if it has one.
	Logo, LogoType string
	LogoData       []byte
	// Stationery leaves out the sender's address and logo, for printing
	// on paper that has them.
	Stationery bool
	// Estimate is set when the document is an estimate; Invoice then holds
	// its lines, number and dates as an invoice would.
	Estimate *Estimate
}

// Send sends an invoice: a draft gets its number and is booked, a sent one
// goes out again. With Scheduled it waits until its invoice date.
func (s *Service) Send(ctx context.Context, admin, id, user resource.ID, in SendInput) (Invoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Invoice{}, err
	}
	inv, err := s.store.Invoice(ctx, admin, id)
	if err != nil {
		return Invoice{}, err
	}
	contact, err := s.contacts.Contact(ctx, admin, inv.ContactID, user)
	if err != nil {
		return Invoice{}, invalid("contact_id", "does not exist")
	}
	settings, err := s.invoicing.Settings(ctx, admin, user)
	if err != nil {
		return Invoice{}, wrap("load invoice settings", err)
	}
	delivery, err := resolveDelivery(in, inv.Delivery, contact, settings.Settings)
	if err != nil {
		return Invoice{}, err
	}
	today := s.today(a)
	if !inv.Sent() {
		if len(inv.Lines) == 0 {
			return Invoice{}, invalid(detailsField, "can't be blank; add a line first")
		}
		day := today
		switch {
		case in.InvoiceDate != "":
			d, err := time.Parse(time.DateOnly, in.InvoiceDate)
			if err != nil {
				return Invoice{}, invalid("invoice_date", "is not a date")
			}
			day = d
		case inv.InvoiceDate != nil && inv.State == Scheduled:
			day = *inv.InvoiceDate
		}
		inv.Delivery = &delivery
		inv.InvoiceDate = &day
		if in.Scheduled {
			if !day.After(today) {
				return Invoice{}, invalid("invoice_date", "must lie in the future to schedule sending")
			}
			was := inv.State
			inv.State, inv.UpdatedAt = Scheduled, s.now()
			if err := s.save(ctx, admin, user, &inv, nil, false); err != nil {
				return Invoice{}, err
			}
			if was != Scheduled {
				if err := s.record(ctx, user, stateChanged(inv, Scheduled)); err != nil {
					return Invoice{}, err
				}
			}
			return s.Invoice(ctx, admin, id, user)
		}
		if err := s.checkUnlocked(ctx, admin, user, &day); err != nil {
			return Invoice{}, err
		}
		if err := s.number(ctx, admin, user, &inv, contact, settings.Settings); err != nil {
			return Invoice{}, err
		}
		due := day.AddDate(0, 0, inv.FirstDueInterval)
		now := s.now()
		inv.DueDate, inv.PaymentReference, inv.State, inv.SentAt, inv.UpdatedAt = &due, inv.InvoiceID, Open, &now, now
		entries, err := s.post(ctx, admin, user, inv)
		if err != nil {
			return Invoice{}, err
		}
		if err := s.save(ctx, admin, user, &inv, entries, false); err != nil {
			return Invoice{}, err
		}
		if err := s.record(ctx, user, stateChanged(inv, Open)); err != nil {
			return Invoice{}, err
		}
	} else {
		inv.Delivery, inv.UpdatedAt = &delivery, s.now()
		if err := s.save(ctx, admin, user, &inv, nil, false); err != nil {
			return Invoice{}, err
		}
	}
	if err := s.deliver(ctx, a, user, inv, delivery, settings.Settings); err != nil {
		return Invoice{}, fmt.Errorf("%w: %v", ErrDelivery, err)
	}
	if err := s.record(ctx, user, sentEvent(inv)); err != nil {
		return Invoice{}, err
	}
	return s.Invoice(ctx, admin, id, user)
}

func resolveDelivery(in SendInput, previous *Delivery, contact contacts.Contact, settings invoicing.Settings) (Delivery, error) {
	d := Delivery{Method: contact.DeliveryMethod, EmailAddress: contact.SendInvoicesToEmail, DeliverUBL: contact.EmailUBL || settings.DefaultEmailUBL}
	if previous != nil {
		d = *previous
	}
	if in.DeliveryMethod != "" {
		d.Method = in.DeliveryMethod
	}
	if in.EmailAddress != "" {
		d.EmailAddress = strings.TrimSpace(in.EmailAddress)
	}
	d.EmailMessage = in.EmailMessage
	if in.DeliverUBL != nil {
		d.DeliverUBL = *in.DeliverUBL
	}
	if in.Mergeable != nil {
		d.Mergeable = *in.Mergeable
	}
	if d.Method == "" {
		d.Method = "Email"
	}
	switch {
	case !slices.Contains(DeliveryMethods, d.Method):
		return d, invalid("delivery_method", "is not included in the list")
	case d.Method == "Peppol" || d.Method == "Simplerinvoicing":
		return d, ErrDeliveryUnavailable
	case d.Method == "Email" && d.EmailAddress == "":
		return d, invalid("email_address", "can't be blank; give one or add it to the contact")
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

// number gives a draft its invoice number.
func (s *Service) number(ctx context.Context, admin, user resource.ID, inv *Invoice, contact contacts.Contact, settings invoicing.Settings) error {
	render := func(seq string) string {
		return invoicing.RenderNumber(settings.InvoiceIDTemplate, seq, *inv.InvoiceDate, inv.Language, contact.CustomerID)
	}
	if inv.SequenceID != "" {
		number := render(inv.SequenceID)
		taken, err := s.store.NumberTaken(ctx, admin, number, inv.ID)
		if err != nil {
			return wrap("check invoice number", err)
		}
		if taken {
			return invalid("invoice_sequence_id", "gives the number "+number+", which another invoice has")
		}
		inv.InvoiceID = number
		return nil
	}
	for range 20 {
		seq, err := s.store.ReserveNumber(ctx, admin)
		if err != nil {
			return wrap("reserve invoice number", err)
		}
		number := render(seq)
		taken, err := s.store.NumberTaken(ctx, admin, number, inv.ID)
		if err != nil {
			return wrap("check invoice number", err)
		}
		if !taken {
			inv.InvoiceID = number
			return nil
		}
	}
	return invalid("invoice_sequence_id", "could not find a free invoice number; change the sequence in the invoice settings")
}

// Document gathers what the invoice's documents show.
func (s *Service) Document(ctx context.Context, admin, id, user resource.ID) (Document, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Document{}, err
	}
	inv, err := s.Invoice(ctx, admin, id, user)
	if err != nil {
		return Document{}, err
	}
	return s.document(ctx, a, user, inv)
}

func (s *Service) document(ctx context.Context, a administration.Listed, user resource.ID, inv Invoice) (Document, error) {
	admin := a.ID
	doc := Document{Administration: a.Administration, Invoice: inv}
	var err error
	if doc.Rates, err = s.rates(ctx, admin, user, inv); err != nil {
		return Document{}, err
	}
	doc.Totals = ComputeInvoice(inv, doc.Rates)
	if doc.Contact, err = s.contacts.Contact(ctx, admin, inv.ContactID, user); err != nil {
		return Document{}, wrap("load contact", err)
	}
	if inv.ContactPersonID != nil {
		if i := slices.IndexFunc(doc.Contact.People, func(p contacts.Person) bool { return p.ID == *inv.ContactPersonID }); i >= 0 {
			doc.ContactPerson = &doc.Contact.People[i]
		}
	}
	if doc.Identity, err = s.admins.Identity(ctx, admin, inv.IdentityID, user); err != nil {
		if doc.Identity, err = s.admins.DefaultIdentity(ctx, admin, user); err != nil {
			return Document{}, wrap("load identity", err)
		}
	}
	if inv.DocumentStyleID != 0 {
		if doc.Style, err = s.invoicing.DocumentStyle(ctx, admin, inv.DocumentStyleID, user); err == nil && doc.Style.Logo != nil {
			if body, contentType, err := s.invoicing.OpenFile(ctx, admin, doc.Style.ID, user, invoicing.Logo); err == nil {
				doc.LogoData, _ = io.ReadAll(io.LimitReader(body, 5<<20))
				doc.LogoType = contentType
				_ = body.Close()
			}
		}
	}
	if inv.WorkflowID != 0 {
		doc.Workflow, _ = s.invoicing.Workflow(ctx, admin, inv.WorkflowID, user)
	}
	return doc, nil
}

// PDF renders the invoice, with the file name to save it under.
func (s *Service) PDF(ctx context.Context, admin, id, user resource.ID, stationery bool) ([]byte, string, error) {
	doc, err := s.Document(ctx, admin, id, user)
	if err != nil {
		return nil, "", err
	}
	doc.Stationery = stationery
	data, err := s.renderer.PDF(doc)
	return data, fileName(doc.Invoice, "pdf"), wrap("render invoice", err)
}

// PackingSlip renders the invoice without prices.
func (s *Service) PackingSlip(ctx context.Context, admin, id, user resource.ID, stationery bool) ([]byte, string, error) {
	doc, err := s.Document(ctx, admin, id, user)
	if err != nil {
		return nil, "", err
	}
	doc.Stationery = stationery
	data, err := s.renderer.PackingSlip(doc)
	return data, fileName(doc.Invoice, "packing-slip.pdf"), wrap("render packing slip", err)
}

// UBL renders the sent invoice as a UBL 2.1 e-invoice.
func (s *Service) UBL(ctx context.Context, admin, id, user resource.ID) ([]byte, string, error) {
	doc, err := s.Document(ctx, admin, id, user)
	if err != nil {
		return nil, "", err
	}
	if !doc.Invoice.Sent() {
		return nil, "", invalid("base", "Send the invoice first; a draft has no invoice number for its e-invoice")
	}
	data, err := s.renderer.UBL(doc)
	return data, fileName(doc.Invoice, "xml"), wrap("render UBL", err)
}

func fileName(inv Invoice, ext string) string {
	name := inv.InvoiceID
	if name == "" {
		name = fmt.Sprintf("draft-%d", inv.DraftID)
	}
	return "invoice-" + strings.NewReplacer("/", "-", `\`, "-", " ", "-").Replace(name) + "." + ext
}

// deliver sends the invoice the chosen way. Manual and Post leave sending
// to the user; MoneyHarpy has no postal service.
func (s *Service) deliver(ctx context.Context, a administration.Listed, user resource.ID, inv Invoice, d Delivery, settings invoicing.Settings) error {
	if d.Method != "Email" {
		return nil
	}
	doc, err := s.document(ctx, a, user, inv)
	if err != nil {
		return err
	}
	pdf, err := s.renderer.PDF(doc)
	if err != nil {
		return wrap("render invoice", err)
	}
	files := []File{{Name: fileName(inv, "pdf"), ContentType: "application/pdf", Data: pdf}}
	if d.DeliverUBL {
		ubl, err := s.renderer.UBL(doc)
		if err != nil {
			return wrap("render UBL", err)
		}
		files = append(files, File{Name: fileName(inv, "xml"), ContentType: "application/xml", Data: ubl})
	}
	subject, body := EmailText(doc)
	if strings.TrimSpace(d.EmailMessage) != "" {
		body = Expand(d.EmailMessage, doc)
	}
	m := Mail{To: splitAddresses(d.EmailAddress), BCC: splitAddresses(settings.InvoiceBCC), ReplyTo: doc.Identity.Email, FromName: doc.Identity.CompanyName, Subject: subject, Body: body, Files: files,
		Look: s.emailLook(ctx, a.ID, user, doc, InvoiceDetails(doc))}
	return s.mailer.SendDocument(ctx, m)
}

func splitAddresses(list string) []string {
	var out []string
	for addr := range strings.SplitSeq(list, ",") {
		if addr = strings.TrimSpace(addr); addr != "" {
			out = append(out, addr)
		}
	}
	return out
}

// Credit makes a draft credit invoice for a sent one: its lines, or the
// chosen ones, with the quantities turned negative.
func (s *Service) Credit(ctx context.Context, admin, id, user resource.ID, lineIDs []resource.ID) (Invoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Invoice{}, err
	}
	orig, err := s.store.Invoice(ctx, admin, id)
	if err != nil {
		return Invoice{}, err
	}
	if !orig.Sent() {
		return Invoice{}, invalid("base", "Only a sent invoice can be credited; delete a draft instead")
	}
	now := s.now()
	credit := orig
	credit.ID, credit.InvoiceID, credit.SequenceID, credit.State = s.ids.Next(), "", "", Draft
	credit.InvoiceDate, credit.DueDate, credit.SentAt, credit.PaidAt, credit.PaymentReference = nil, nil, nil, nil, ""
	credit.MarkedDubiousOn, credit.MarkedUncollectibleOn, credit.Paused, credit.ReminderCount = nil, nil, false, 0
	credit.Delivery, credit.Payments, credit.Notes, credit.Attachments = nil, nil, nil, nil
	credit.OriginalID, credit.RecurringSalesInvoiceID, credit.OriginalEstimateID = &orig.ID, nil, nil
	credit.CreatedAt, credit.UpdatedAt = now, now
	credit.Lines = nil
	for _, d := range orig.Lines {
		if len(lineIDs) > 0 && !slices.Contains(lineIDs, d.ID) {
			continue
		}
		d.ID, d.CreatedAt, d.UpdatedAt = s.ids.Next(), now, now
		d.Quantity = -d.Quantity
		d.Amount = negate(d.Amount)
		credit.Lines = append(credit.Lines, d)
	}
	if len(credit.Lines) == 0 {
		return Invoice{}, invalid("detail_ids", "are not lines of this invoice")
	}
	if credit.DraftID, err = s.store.NextDraftID(ctx, admin); err != nil {
		return Invoice{}, wrap("number draft", err)
	}
	if err := s.save(ctx, admin, user, &credit, nil, true); err != nil {
		return Invoice{}, err
	}
	if err := s.record(ctx, user,
		on(credit, "credit_invoice_created_from_original").with(map[string]any{"original_invoice": orig.InvoiceID}),
		on(orig, "credit_invoice_created").linked(EntityType, credit.ID),
	); err != nil {
		return Invoice{}, err
	}
	return s.one(ctx, a.ID, user, func() (Invoice, error) { return s.store.Invoice(ctx, admin, credit.ID) })
}

// negate turns a quantity text such as "2 hours" into "-2 hours".
func negate(amount string) string {
	amount = strings.TrimSpace(amount)
	if strings.HasPrefix(amount, "-") {
		return strings.TrimPrefix(amount, "-")
	}
	return "-" + amount
}

// Pause stops a sent invoice's reminders; Resume starts them again.
func (s *Service) Pause(ctx context.Context, admin, id, user resource.ID, paused bool) (Invoice, error) {
	inv, err := s.Invoice(ctx, admin, id, user)
	if err != nil {
		return Invoice{}, err
	}
	if !inv.Sent() || inv.State == Paid || inv.State == Uncollectible {
		return Invoice{}, ErrNotEditable
	}
	raw, err := s.store.Invoice(ctx, admin, id)
	if err != nil {
		return Invoice{}, err
	}
	was := raw.Paused
	raw.Paused, raw.UpdatedAt = paused, s.now()
	if err := s.save(ctx, admin, user, &raw, nil, false); err != nil {
		return Invoice{}, err
	}
	if was != paused {
		action := "sales_invoice_unpaused"
		if paused {
			action = "sales_invoice_paused"
		}
		if err := s.record(ctx, user, on(raw, action)); err != nil {
			return Invoice{}, err
		}
	}
	return s.Invoice(ctx, admin, id, user)
}

// AddNote adds a note to an invoice.
func (s *Service) AddNote(ctx context.Context, admin, id, user resource.ID, in notes.Input) (notes.Note, error) {
	if _, err := s.Invoice(ctx, admin, id, user); err != nil {
		return notes.Note{}, err
	}
	n, err := s.notes.Add(ctx, admin, EntityType, id, user, in)
	if err != nil {
		return notes.Note{}, err
	}
	return n, s.store.TouchInvoice(ctx, admin, id, n.CreatedAt)
}

// DeleteNote removes a note from an invoice.
func (s *Service) DeleteNote(ctx context.Context, admin, id, noteID, user resource.ID) error {
	if _, err := s.Invoice(ctx, admin, id, user); err != nil {
		return err
	}
	if err := s.notes.Delete(ctx, admin, EntityType, id, noteID); err != nil {
		return err
	}
	return s.store.TouchInvoice(ctx, admin, id, s.now())
}

// AddAttachment attaches a file to an invoice.
func (s *Service) AddAttachment(ctx context.Context, admin, id, user resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error) {
	if _, err := s.Invoice(ctx, admin, id, user); err != nil {
		return attachments.Attachment{}, err
	}
	a, err := s.attachments.Add(ctx, admin, EntityType, id, filename, contentType, data)
	if err != nil {
		return attachments.Attachment{}, err
	}
	return a, s.store.TouchInvoice(ctx, admin, id, a.CreatedAt)
}

// OpenAttachment returns an attachment of an invoice; the caller closes it.
func (s *Service) OpenAttachment(ctx context.Context, admin, id, attachment, user resource.ID) (attachments.Attachment, io.ReadCloser, error) {
	if _, err := s.Invoice(ctx, admin, id, user); err != nil {
		return attachments.Attachment{}, nil, err
	}
	return s.attachments.Open(ctx, admin, EntityType, id, attachment)
}

// DeleteAttachment removes an attachment from an invoice.
func (s *Service) DeleteAttachment(ctx context.Context, admin, id, attachment, user resource.ID) error {
	if _, err := s.Invoice(ctx, admin, id, user); err != nil {
		return err
	}
	if err := s.attachments.Delete(ctx, admin, EntityType, id, attachment); err != nil {
		return err
	}
	return s.store.TouchInvoice(ctx, admin, id, s.now())
}

// ContactInUse reports whether invoices refer to a contact.
func (s *Service) ContactInUse(ctx context.Context, admin, contact resource.ID) (bool, error) {
	return s.store.ContactInUse(ctx, admin, contact)
}
