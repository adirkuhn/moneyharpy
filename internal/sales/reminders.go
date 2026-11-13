package sales

import (
	"context"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ReminderInput is one reminder: invoices of a contact that share a
// workflow, identity and document style, as Moneybird groups them.
type ReminderInput struct {
	ContactID       resource.ID
	WorkflowID      resource.ID
	DocumentStyleID resource.ID
	IdentityID      resource.ID
	InvoiceIDs      []resource.ID
	// ReminderText defaults to the text of the workflow's next step.
	ReminderText string
	// DeliveryMethod is Email or Manual; it defaults to the contact's.
	DeliveryMethod string
	EmailAddress   string
}

// steps are a workflow's steps in order: sending with the payment term,
// then the reminders.
func steps(w invoicing.Workflow) []invoicing.WorkflowStep {
	out := slices.Clone(w.Steps)
	slices.SortStableFunc(out, func(a, b invoicing.WorkflowStep) int { return a.StepOrder - b.StepOrder })
	return out
}

// PausedNextReminder is the next reminder of a paused invoice, as Moneybird
// gives it (its API's example of pausing an invoice): a day far ahead
// rather than none, so lists sorted by it put paused invoices last.
var PausedNextReminder = time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)

// nextReminder is when the workflow's next reminder for the invoice is due,
// and that step. As on Moneybird each step expires after its due interval
// and the next reminder is due when it does: the first reminder on the
// invoice's due date, every later one the due interval of the reminder
// before it after the day that reminder went out.
func nextReminder(inv Invoice, w invoicing.Workflow) (time.Time, invoicing.WorkflowStep, bool) {
	if !payable(inv) || inv.Paused || inv.DueDate == nil || w.ID != inv.WorkflowID {
		return time.Time{}, invoicing.WorkflowStep{}, false
	}
	list := steps(w)
	next := inv.ReminderCount + 1
	if next >= len(list) {
		return time.Time{}, invoicing.WorkflowStep{}, false
	}
	if inv.ReminderCount == 0 {
		return *inv.DueDate, list[next], true
	}
	if inv.LastReminderOn != nil {
		return inv.LastReminderOn.AddDate(0, 0, list[inv.ReminderCount].DueInterval), list[next], true
	}
	// Without the day of the last reminder, the reminders are taken to have
	// gone out on the days the workflow plans them.
	day := *inv.DueDate
	for _, st := range list[1:next] {
		day = day.AddDate(0, 0, st.DueInterval)
	}
	return day, list[next], true
}

// DefaultReminderText is the reminder MoneyHarpy sends when the workflow
// has no text of its own. {invoices} lists the invoices.
func DefaultReminderText(language string) string {
	if strings.HasPrefix(language, "nl") {
		return "Beste {contact_name},\n\nWe hebben de betaling van onderstaande facturen nog niet ontvangen. " +
			"Wil je het openstaande bedrag zo snel mogelijk overmaken onder vermelding van het factuurnummer?\n\n{invoices}\n\nMet vriendelijke groet,\n{company_name}"
	}
	return "Dear {contact_name},\n\nWe have not yet received payment of the invoices below. " +
		"Would you please pay the open amount as soon as possible, quoting the invoice number?\n\n{invoices}\n\nKind regards,\n{company_name}"
}

// ReminderEmail is the subject and body of a reminder about the documents'
// invoices.
func ReminderEmail(docs []Document, text string) (string, string) {
	first := docs[0]
	nl := strings.HasPrefix(first.Invoice.Language, "nl")
	var subject string
	switch {
	case nl && len(docs) == 1:
		subject = "Herinnering: factuur {invoice_id} van {company_name}"
	case nl:
		subject = fmt.Sprintf("Herinnering: %d facturen van {company_name}", len(docs))
	case len(docs) == 1:
		subject = "Reminder: invoice {invoice_id} from {company_name}"
	default:
		subject = fmt.Sprintf("Reminder: %d invoices from {company_name}", len(docs))
	}
	lines := make([]string, len(docs))
	for i, d := range docs {
		inv := d.Invoice
		date := ""
		if inv.InvoiceDate != nil {
			date = inv.InvoiceDate.Format("02-01-2006")
		}
		lines[i] = fmt.Sprintf("- %s (%s): %s", inv.InvoiceID, date, FormatMoney(inv.Unpaid(), inv.Currency, inv.Language))
	}
	list := strings.Join(lines, "\n")
	body := Expand(text, first)
	// {reminders_table} is Moneybird's tag for the list, {invoices} ours.
	if strings.Contains(body, "{invoices}") || strings.Contains(body, "{reminders_table}") {
		body = strings.NewReplacer("{invoices}", list, "{reminders_table}", list).Replace(body)
	} else {
		body += "\n\n" + list
	}
	return Expand(subject, first), body
}

type reminder struct {
	in       ReminderInput
	invoices []Invoice
	delivery Delivery
	text     string
}

// SendReminders sends reminders: one email per reminder listing its
// invoices, with their PDFs, or none when delivered by hand. Each invoice
// becomes reminded and counts the reminder.
func (s *Service) SendReminders(ctx context.Context, admin, user resource.ID, in []ReminderInput) ([]Invoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	if len(in) == 0 {
		return nil, invalid("sales_invoice_reminders", "can't be blank")
	}
	settings, err := s.invoicing.Settings(ctx, admin, user)
	if err != nil {
		return nil, wrap("load invoice settings", err)
	}
	var verr resource.ValidationError
	jobs := make([]reminder, 0, len(in))
	for i, r := range in {
		field := fmt.Sprintf("sales_invoice_reminders.%d.", i)
		j, ok := s.prepareReminder(ctx, admin, user, r, field, &verr)
		if ok {
			jobs = append(jobs, j)
		}
	}
	if err := verr.OrNil(); err != nil {
		return nil, err
	}
	var ids []resource.ID
	today := s.today(a)
	for _, j := range jobs {
		if err := s.deliverReminder(ctx, a, user, j, settings.Settings); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrDelivery, err)
		}
		action := "sales_invoice_send_reminder_manually"
		if j.delivery.Method == "Email" {
			action = "sales_invoice_send_reminder_email"
		}
		for _, inv := range j.invoices {
			was := inv.State
			inv.State, inv.ReminderCount, inv.LastReminderOn, inv.UpdatedAt = Reminded, inv.ReminderCount+1, &today, s.now()
			if err := s.save(ctx, admin, user, &inv, nil, false); err != nil {
				return nil, err
			}
			s.notify(ctx, admin, action, inv)
			events := []event{on(inv, action)}
			if was != Reminded {
				s.notify(ctx, admin, "sales_invoice_state_changed_to_reminded", inv)
				events = append(events, stateChanged(inv, Reminded))
			}
			if err := s.record(ctx, user, events...); err != nil {
				return nil, err
			}
			ids = append(ids, inv.ID)
		}
	}
	return s.InvoicesByID(ctx, admin, user, ids)
}

func (s *Service) prepareReminder(ctx context.Context, admin, user resource.ID, r ReminderInput, field string, verr *resource.ValidationError) (reminder, bool) {
	j := reminder{in: r}
	contact, err := s.contacts.Contact(ctx, admin, r.ContactID, user)
	if err != nil {
		verr.Add(field+"contact_id", "does not exist")
		return j, false
	}
	flow, err := s.invoicing.Workflow(ctx, admin, r.WorkflowID, user)
	if err != nil || flow.Type != invoicing.InvoiceWorkflow {
		verr.Add(field+"workflow_id", "does not exist")
		return j, false
	}
	if len(r.InvoiceIDs) == 0 || len(r.InvoiceIDs) > MaxPerPage {
		verr.Add(field+"sales_invoice_ids", fmt.Sprintf("must hold 1 to %d invoices", MaxPerPage))
		return j, false
	}
	list, err := s.store.InvoicesByID(ctx, admin, r.InvoiceIDs)
	if err != nil || len(list) != len(slices.Compact(slices.Sorted(slices.Values(r.InvoiceIDs)))) {
		verr.Add(field+"sales_invoice_ids", "are not all sales invoices of this administration")
		return j, false
	}
	for _, inv := range list {
		if inv.ContactID != r.ContactID || !payable(inv) {
			verr.Add(field+"sales_invoice_ids", "must all be unpaid sent invoices of the contact")
			return j, false
		}
	}
	j.invoices = list
	j.delivery = reminderDelivery(r, contact)
	switch {
	case j.delivery.Method != "Email" && j.delivery.Method != "Manual":
		verr.Add(field+"delivery_method", "is not included in the list")
	case j.delivery.Method == "Email" && j.delivery.EmailAddress == "":
		verr.Add(field+"email_address", "can't be blank; give one or add it to the contact")
	case j.delivery.Method == "Email":
		for addr := range strings.SplitSeq(j.delivery.EmailAddress, ",") {
			if _, err := mail.ParseAddress(strings.TrimSpace(addr)); err != nil {
				verr.Add(field+"email_address", "is not a valid email address")
				break
			}
		}
	}
	j.text = strings.TrimSpace(r.ReminderText)
	if j.text == "" {
		if _, step, ok := nextReminder(list[0], flow); ok {
			j.text = strings.TrimSpace(step.ReminderText)
		}
	}
	if j.text == "" {
		j.text = DefaultReminderText(list[0].Language)
	}
	if len(j.text) > 10000 {
		verr.Add(field+"reminder_text", "is too long (maximum is 10000 characters)")
	}
	return j, true
}

// reminderDelivery is how a reminder goes out: as asked, or by email when
// the contact gets its invoices by email and by hand otherwise.
func reminderDelivery(r ReminderInput, contact contacts.Contact) Delivery {
	d := Delivery{Method: r.DeliveryMethod, EmailAddress: strings.TrimSpace(r.EmailAddress)}
	if d.Method == "" {
		d.Method = "Manual"
		if contact.DeliveryMethod == "Email" || contact.DeliveryMethod == "" {
			d.Method = "Email"
		}
	}
	if d.EmailAddress == "" {
		d.EmailAddress = contact.SendInvoicesToEmail
	}
	return d
}

func (s *Service) deliverReminder(ctx context.Context, a administration.Listed, user resource.ID, j reminder, settings invoicing.Settings) error {
	if j.delivery.Method != "Email" {
		return nil
	}
	docs := make([]Document, len(j.invoices))
	files := make([]File, len(j.invoices))
	for i, inv := range j.invoices {
		inv.IdentityID, inv.DocumentStyleID = cmpID(j.in.IdentityID, inv.IdentityID), cmpID(j.in.DocumentStyleID, inv.DocumentStyleID)
		doc, err := s.document(ctx, a, user, inv)
		if err != nil {
			return err
		}
		pdf, err := s.renderer.PDF(doc)
		if err != nil {
			return wrap("render invoice", err)
		}
		docs[i], files[i] = doc, File{Name: fileName(inv, "pdf"), ContentType: "application/pdf", Data: pdf}
	}
	subject, body := ReminderEmail(docs, j.text)
	m := Mail{To: splitAddresses(j.delivery.EmailAddress), BCC: splitAddresses(settings.InvoiceBCC), ReplyTo: docs[0].Identity.Email,
		FromName: docs[0].Identity.CompanyName, Subject: subject, Body: body, Files: files}
	if len(docs) == 1 {
		m.Look = s.emailLook(ctx, a.ID, user, docs[0], InvoiceDetails(docs[0]))
	} else {
		m.Look = s.emailLook(ctx, a.ID, user, docs[0], nil)
	}
	return s.mailer.SendDocument(ctx, m)
}

func cmpID(a, b resource.ID) resource.ID {
	if a != 0 {
		return a
	}
	return b
}
