package invoicing

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// WorkflowType tells what a workflow is for, with Moneybird's names.
type WorkflowType string

// The workflow types.
const (
	InvoiceWorkflow  WorkflowType = "InvoiceWorkflow"
	EstimateWorkflow WorkflowType = "EstimateWorkflow"
)

const (
	maxDays       = 365
	maxReminders  = 10
	maxReminderTx = 5000
	maxConditions = 1000
)

// DefaultPaymentConditions is the payment conditions text a new invoice
// workflow starts with, in the workflow's language: Moneybird's default text
// in Dutch, and its translation otherwise. Its tags are filled in where an
// invoice is shown.
func DefaultPaymentConditions(language string) string {
	if strings.HasPrefix(language, "nl") {
		return "We verzoeken u vriendelijk het bovenstaande bedrag van {document.total_price} voor {document.due_date} te voldoen op onze bankrekening " +
			"onder vermelding van de omschrijving {document.payment_reference}. Voor vragen kunt u contact opnemen per e-mail."
	}
	return "We kindly request you to pay the above amount of {document.total_price} before {document.due_date} to our bank account, " +
		"stating the description {document.payment_reference}. If you have any questions, please contact us by email."
}

// Workflow is how invoices or estimates of one kind are handled: currency,
// language, payment term and reminders.
type Workflow struct {
	ID               resource.ID
	AdministrationID resource.ID
	Type             WorkflowType
	Name             string
	Default          bool
	Currency         string
	Language         string
	Active           bool
	PricesAreInclTax bool
	ShowQRCode       bool
	// InvoiceEmailSubject and InvoiceEmailBody are the email invoices go
	// out with; empty means MoneyHarpy's default in the invoice's language.
	InvoiceEmailSubject string
	InvoiceEmailBody    string
	// PaymentConditions is the text new invoices of an invoice workflow
	// start with, as Moneybird's "Payment conditions on invoice"; it may
	// hold Moneybird's tags and textile.
	PaymentConditions string
	// Steps hold the payment term or validity first, then the reminders.
	Steps     []WorkflowStep
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WorkflowStep is one stage of a workflow, as Moneybird's workflow steps.
// StepOrder counts from 0: step 0 sends the invoice (or estimate), steps 1
// and on are the reminders. Each step expires DueInterval days after it was
// taken ("Reminder expires after" on Moneybird): for step 0 that is the
// payment term or validity, and when a step expires unpaid the next reminder
// is due. So on the workflow's page reminder n is on Day 1 plus the due
// intervals of the steps before it. ReminderDelay is kept for Moneybird's
// API, whose workflows always hold 0; it does not move a reminder.
type WorkflowStep struct {
	ID               resource.ID
	StepOrder        int
	DueInterval      int
	ReminderDelay    int
	ReminderText     string
	ReminderAutoSend bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Ref locates the workflow.
func (w Workflow) Ref() resource.Ref {
	return resource.Ref{ID: w.ID, Type: resource.TypeWorkflow, AdministrationID: w.AdministrationID}
}

// StepInput describes a step to set.
type StepInput struct {
	DueInterval      int
	ReminderDelay    int
	ReminderText     string
	ReminderAutoSend bool
}

// WorkflowChanges holds the workflow fields to set. Nil fields stay as they
// are; Steps replaces all steps.
type WorkflowChanges struct {
	Name             *string
	Currency         *string
	Language         *string
	Active           *bool
	Default          *bool
	PricesAreInclTax *bool
	ShowQRCode       *bool
	Steps            *[]StepInput
	// InvoiceEmailSubject and InvoiceEmailBody set the invoice email; empty
	// returns to MoneyHarpy's default.
	InvoiceEmailSubject *string
	InvoiceEmailBody    *string
	// PaymentConditions sets the text new invoices start with; estimate
	// workflows have none.
	PaymentConditions *string
}

// Workflows lists the administration's workflows, creating the default ones
// on first use.
func (s *Service) Workflows(ctx context.Context, admin, user resource.ID) ([]Workflow, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	if err := s.ensureWorkflows(ctx, a.Administration); err != nil {
		return nil, err
	}
	list, err := s.store.Workflows(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}
	slices.SortStableFunc(list, func(x, y Workflow) int {
		return cmp.Or(
			cmp.Compare(x.Type, y.Type),
			-boolCompare(x.Default, y.Default),
			cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name)),
			cmp.Compare(x.ID, y.ID),
		)
	})
	return list, nil
}

// Workflow returns one workflow.
func (s *Service) Workflow(ctx context.Context, admin, id, user resource.ID) (Workflow, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Workflow{}, err
	}
	return s.store.Workflow(ctx, admin, id)
}

// WorkflowKind returns a workflow's type and whether it is active, for
// records that refer to one.
func (s *Service) WorkflowKind(ctx context.Context, admin, id, user resource.ID) (string, bool, error) {
	w, err := s.Workflow(ctx, admin, id, user)
	if err != nil {
		return "", false, err
	}
	return string(w.Type), w.Active, nil
}

// CreateWorkflow adds a workflow of type t, filled from the administration's
// defaults where c leaves fields out.
func (s *Service) CreateWorkflow(ctx context.Context, admin, user resource.ID, t WorkflowType, c WorkflowChanges) (Workflow, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Workflow{}, err
	}
	if t != InvoiceWorkflow && t != EstimateWorkflow {
		var verr resource.ValidationError
		verr.Add("type", "is not included in the list")
		return Workflow{}, &verr
	}
	if err := s.ensureWorkflows(ctx, a.Administration); err != nil {
		return Workflow{}, err
	}
	w := s.defaultWorkflow(a.Administration, t)
	w.Default = false
	w.Name = ""
	// A workflow in another language starts with that language's conditions.
	if t == InvoiceWorkflow && c.Language != nil && c.PaymentConditions == nil {
		w.PaymentConditions = DefaultPaymentConditions(*c.Language)
	}
	if err := s.apply(&w, c); err != nil {
		return Workflow{}, err
	}
	s.stamp(&w)
	// The store keeps one default per type, so a new default is stored as a
	// regular workflow first and then takes over.
	makeDefault := w.Default
	w.Default = false
	if _, err := s.store.CreateWorkflow(ctx, w); err != nil {
		return Workflow{}, fmt.Errorf("store workflow: %w", err)
	}
	if makeDefault {
		w.Default = true
		if err := s.store.UpdateWorkflow(ctx, w); err != nil {
			return Workflow{}, fmt.Errorf("make workflow default: %w", err)
		}
	}
	return w, nil
}

// UpdateWorkflow changes a workflow.
func (s *Service) UpdateWorkflow(ctx context.Context, admin, id, user resource.ID, c WorkflowChanges) (Workflow, error) {
	w, err := s.Workflow(ctx, admin, id, user)
	if err != nil {
		return Workflow{}, err
	}
	wasDefault := w.Default
	if err := s.apply(&w, c); err != nil {
		return Workflow{}, err
	}
	if wasDefault && (!w.Default || !w.Active) {
		return Workflow{}, ErrDefaultInUse
	}
	s.stamp(&w)
	if err := s.store.UpdateWorkflow(ctx, w); err != nil {
		return Workflow{}, fmt.Errorf("store workflow: %w", err)
	}
	return w, nil
}

// DeleteWorkflow removes a workflow that is not a default.
func (s *Service) DeleteWorkflow(ctx context.Context, admin, id, user resource.ID) error {
	w, err := s.Workflow(ctx, admin, id, user)
	if err != nil {
		return err
	}
	if w.Default {
		return ErrDefaultInUse
	}
	return s.store.DeleteWorkflow(ctx, admin, id)
}

func (s *Service) apply(w *Workflow, c WorkflowChanges) error {
	if c.Name != nil {
		w.Name = strings.TrimSpace(*c.Name)
	}
	for _, f := range []struct {
		dst *string
		src *string
	}{{&w.Currency, c.Currency}, {&w.Language, c.Language}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	for _, f := range []struct {
		dst *bool
		src *bool
	}{{&w.Active, c.Active}, {&w.Default, c.Default}, {&w.PricesAreInclTax, c.PricesAreInclTax}, {&w.ShowQRCode, c.ShowQRCode}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if c.InvoiceEmailSubject != nil {
		w.InvoiceEmailSubject = strings.TrimSpace(*c.InvoiceEmailSubject)
	}
	if c.InvoiceEmailBody != nil {
		w.InvoiceEmailBody = strings.TrimSpace(*c.InvoiceEmailBody)
	}
	if c.PaymentConditions != nil {
		w.PaymentConditions = strings.TrimSpace(*c.PaymentConditions)
	}
	if w.Type != InvoiceWorkflow {
		w.PaymentConditions = ""
	}
	if c.Steps != nil {
		w.Steps = make([]WorkflowStep, len(*c.Steps))
		for i, in := range *c.Steps {
			w.Steps[i] = WorkflowStep{
				StepOrder: i, DueInterval: in.DueInterval, ReminderDelay: in.ReminderDelay,
				ReminderText: strings.TrimSpace(in.ReminderText), ReminderAutoSend: in.ReminderAutoSend,
			}
		}
	}
	return validateWorkflow(*w)
}

func validateWorkflow(w Workflow) error {
	var verr resource.ValidationError
	if len(w.InvoiceEmailSubject) > 255 {
		verr.Add("invoice_email_subject", "is too long (maximum is 255 characters)")
	}
	if len(w.InvoiceEmailBody) > 10000 {
		verr.Add("invoice_email_body", "is too long (maximum is 10000 characters)")
	}
	if len([]rune(w.PaymentConditions)) > maxConditions {
		verr.Add("payment_conditions", fmt.Sprintf("is too long (maximum is %d characters)", maxConditions))
	}
	if w.Name == "" || len(w.Name) > 255 {
		verr.Add("name", "can't be blank")
	}
	if !slices.Contains(administration.Currencies(), w.Currency) {
		verr.Add("currency", "must be an ISO 4217 code")
	}
	if !slices.Contains([]string{"nl", "nl-be", "en"}, w.Language) {
		verr.Add("language", "is not included in the list")
	}
	if w.Default && !w.Active {
		verr.Add("active", "can't be off for the default workflow")
	}
	switch {
	case len(w.Steps) == 0:
		verr.Add("steps", "must include the payment term")
	case w.Type == EstimateWorkflow && len(w.Steps) > 1:
		verr.Add("steps", "can only hold the validity of an estimate")
	case len(w.Steps) > maxReminders+1:
		verr.Add("steps", fmt.Sprintf("can hold at most %d reminders", maxReminders))
	}
	for i, st := range w.Steps {
		if i == 0 && (st.DueInterval < 0 || st.DueInterval > maxDays) {
			verr.Add("steps", fmt.Sprintf("must have a term between 0 and %d days", maxDays))
		}
		if i > 0 && (st.DueInterval < 1 || st.DueInterval > maxDays) {
			verr.Add("steps", fmt.Sprintf("must have reminders that expire after between 1 and %d days", maxDays))
		}
		if st.ReminderDelay < 0 || st.ReminderDelay > maxDays {
			verr.Add("steps", fmt.Sprintf("must have a reminder delay between 0 and %d days", maxDays))
		}
		if len(st.ReminderText) > maxReminderTx {
			verr.Add("steps", fmt.Sprintf("must have reminder texts of at most %d characters", maxReminderTx))
		}
	}
	return verr.OrNil()
}

// stamp gives w and its steps IDs and times.
func (s *Service) stamp(w *Workflow) {
	now := s.now()
	if w.ID == 0 {
		w.ID, w.CreatedAt = s.ids.Next(), now
	}
	w.UpdatedAt = now
	for i := range w.Steps {
		if w.Steps[i].ID == 0 {
			w.Steps[i].ID, w.Steps[i].CreatedAt = s.ids.Next(), now
		}
		w.Steps[i].UpdatedAt = now
	}
}

// ensureWorkflows creates the default invoice and estimate workflows for an
// administration that has none yet.
func (s *Service) ensureWorkflows(ctx context.Context, a administration.Administration) error {
	for _, t := range []WorkflowType{InvoiceWorkflow, EstimateWorkflow} {
		w := s.defaultWorkflow(a, t)
		s.stamp(&w)
		if _, err := s.store.CreateWorkflow(ctx, w); err != nil {
			return fmt.Errorf("create default workflow: %w", err)
		}
	}
	return nil
}

// DefaultReminderTexts are the emails of the three reminders of Moneybird's
// default invoice workflow: Moneybird's text in Dutch, its translation
// otherwise. {reminders_table} is where the unpaid invoices are listed.
// Moneybird's text also links to the contact's open invoices online
// ({recipient.show_open_invoices_online_url}); MoneyHarpy has no such page,
// so that sentence is left out.
func DefaultReminderTexts(language string) [3]string {
	if strings.HasPrefix(language, "nl") {
		const crossed = "Mochten de betalingen en deze herinnering elkaar kruisen, dan kunt u deze herinnering als niet verzonden beschouwen."
		return [3]string{
			reminderText("Geachte {recipient.name},",
				"Uit onze administratie blijkt dat u de onderstaande facturen nog niet voldaan heeft. We willen u erop wijzen dat de betaaltermijn van deze facturen verstreken is en verzoeken u vriendelijk doch dringend om de betaling zo spoedig mogelijk te voldoen.",
				crossed, "Met vriendelijke groet,"),
			reminderText("Geachte {recipient.name},",
				"Ondanks een eerdere herinnering blijkt uit onze administratie dat u de onderstaande facturen nog niet voldaan heeft. We willen u er op wijzen dat de betaaltermijn van deze facturen verstreken is en verzoeken u vriendelijk doch dringend om de betaling zo spoedig mogelijk te voldoen.",
				crossed+" Wanneer betaling uitblijft, zullen wij de vordering uit handen moeten geven aan een incassobureau en de wettelijke incassokosten in rekening brengen.",
				"Met vriendelijke groet,"),
			reminderText("Geachte {recipient.name},",
				"Ondanks eerdere herinneringen blijkt uit onze administratie dat u de onderstaande facturen nog niet voldaan heeft. Dit is de derde herinnering voor het voldoen van de vordering, voordat de vordering uit handen gegeven zal worden aan een incassobureau.",
				"Wij verzoeken u om de betalingen binnen 5 dagen te voldoen. Wanneer betaling uitblijft, zullen wij de vordering uit handen geven aan een incassobureau en brengen we de wettelijke incassokosten in rekening.",
				"Met vriendelijke groet,"),
		}
	}
	const crossed = "If this reminder and your payment have crossed, please disregard this reminder."
	return [3]string{
		reminderText("Dear {recipient.name},",
			"According to our records, the invoices below have not been paid yet. We would like to point out that the payment term of these invoices has expired and kindly but urgently request you to pay as soon as possible.",
			crossed, "Kind regards,"),
		reminderText("Dear {recipient.name},",
			"Despite an earlier reminder, our records show that the invoices below have not been paid yet. We would like to point out that the payment term of these invoices has expired and kindly but urgently request you to pay as soon as possible.",
			crossed+" If payment is not received, we will have to hand the claim over to a debt collection agency and charge the statutory collection costs.",
			"Kind regards,"),
		reminderText("Dear {recipient.name},",
			"Despite earlier reminders, our records show that the invoices below have not been paid yet. This is the third reminder to settle the claim before it is handed over to a debt collection agency.",
			"We request you to pay within 5 days. If payment is not received, we will hand the claim over to a debt collection agency and charge the statutory collection costs.",
			"Kind regards,"),
	}
}

// reminderText is a reminder email as Moneybird lays it out: a greeting, the
// text, the table of unpaid invoices, a closing text and the sender.
func reminderText(greeting, text, closing, regards string) string {
	return strings.Join([]string{greeting, text, "{reminders_table}", closing, regards, "{company.company_name}"}, "\n\n")
}

// defaultWorkflow is the starting point for a workflow of type t: as
// Moneybird's default invoice workflow a 14-day payment term, then three
// reminders that expire after 10, 5 and 5 days (Day 15, 25 and 30); or a
// 30-day estimate.
func (s *Service) defaultWorkflow(a administration.Administration, t WorkflowType) Workflow {
	dutch := strings.HasPrefix(a.Language, "nl")
	name := "Default (EN)"
	if dutch {
		name = "Standaard (NL)"
	}
	w := Workflow{
		AdministrationID: a.ID, Type: t, Name: name, Default: true, Currency: a.Currency, Language: a.Language,
		Active: true, ShowQRCode: t == InvoiceWorkflow,
	}
	if t == EstimateWorkflow {
		w.Steps = []WorkflowStep{{StepOrder: 0, DueInterval: 30}}
		return w
	}
	w.PaymentConditions = DefaultPaymentConditions(a.Language)
	texts := DefaultReminderTexts(a.Language)
	w.Steps = []WorkflowStep{
		{StepOrder: 0, DueInterval: 14},
		{StepOrder: 1, DueInterval: 10, ReminderText: texts[0]},
		{StepOrder: 2, DueInterval: 5, ReminderText: texts[1]},
		{StepOrder: 3, DueInterval: 5, ReminderText: texts[2]},
	}
	return w
}

func boolCompare(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	default:
		return -1
	}
}
