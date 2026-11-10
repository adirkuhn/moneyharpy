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
)

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
	// Steps hold the payment term or validity first, then the reminders.
	Steps     []WorkflowStep
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WorkflowStep is one stage of a workflow. For the first step DueInterval is
// the payment term (invoices) or validity (estimates) in days; later steps
// are reminders sent ReminderDelay days after the previous stage.
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
	if c.Steps != nil {
		w.Steps = make([]WorkflowStep, len(*c.Steps))
		for i, in := range *c.Steps {
			w.Steps[i] = WorkflowStep{
				StepOrder: i + 1, DueInterval: in.DueInterval, ReminderDelay: in.ReminderDelay,
				ReminderText: strings.TrimSpace(in.ReminderText), ReminderAutoSend: in.ReminderAutoSend,
			}
		}
	}
	return validateWorkflow(*w)
}

func validateWorkflow(w Workflow) error {
	var verr resource.ValidationError
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
		if i > 0 && (st.ReminderDelay < 1 || st.ReminderDelay > maxDays) {
			verr.Add("steps", fmt.Sprintf("must send reminders between 1 and %d days apart", maxDays))
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

// defaultWorkflow is the starting point for a workflow of type t: a 14-day
// payment term with two reminders a week apart, or a 30-day estimate.
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
		w.Steps = []WorkflowStep{{StepOrder: 1, DueInterval: 30}}
		return w
	}
	first, second := "We have not received payment for this invoice yet. Please pay the outstanding amount as soon as possible.",
		"This is a second reminder: we still have not received payment for this invoice. Please pay the outstanding amount within 7 days."
	if dutch {
		first, second = "We hebben de betaling voor deze factuur nog niet ontvangen. Wil je het openstaande bedrag zo snel mogelijk betalen?",
			"Dit is een tweede herinnering: we hebben de betaling voor deze factuur nog steeds niet ontvangen. Wil je het openstaande bedrag binnen 7 dagen betalen?"
	}
	w.Steps = []WorkflowStep{
		{StepOrder: 1, DueInterval: 14},
		{StepOrder: 2, ReminderDelay: 7, ReminderText: first},
		{StepOrder: 3, ReminderDelay: 7, ReminderText: second},
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
