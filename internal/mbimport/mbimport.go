// Package mbimport copies a whole Moneybird administration into a MoneyHarpy
// administration: its categories and VAT rates, contacts, products and
// projects, and later its documents and transactions. Records keep the ids
// they had in Moneybird, so their addresses stay the same.
//
// An import runs in the background as a job. A worker leases it, works
// through its steps and saves its progress after every batch, so an import
// that stops, such as at a restart, carries on where it was. What was
// imported is remembered per record, so running it again only adds what is
// new. Moneybird's rate limit is the source's to keep; the job only shows
// when it waits for it.
package mbimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/membership"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

var (
	// ErrNotFound reports that the administration has no import.
	ErrNotFound = errors.New("no moneybird import")
	// ErrTokenRejected reports that Moneybird refused the API token.
	ErrTokenRejected = errors.New("moneybird refused the api token")
	// ErrNotEmpty reports that the administration already has data, which
	// an import would mix with.
	ErrNotEmpty = errors.New("administration is not empty")
	// ErrUnavailable reports that Moneybird could not be reached or did not
	// answer as it should; trying again later may work.
	ErrUnavailable = errors.New("moneybird is unavailable")
	// ErrFileUnavailable reports a file Moneybird could not give, such as one
	// too large or gone.
	ErrFileUnavailable = errors.New("moneybird file unavailable")
	// ErrNotQueued reports an import that is not waiting to try again.
	ErrNotQueued = errors.New("moneybird import is not queued")
	// ErrRunning reports that an import is under way already.
	ErrRunning = errors.New("moneybird import is running")
)

// State is where a job is.
type State string

// The states of a job.
const (
	Running State = "running"
	Done    State = "done"
	Failed  State = "failed"
)

// The steps of an import, in the order they run: what comes later refers to
// what came before.
const (
	StepChart = "chart"
	// StepCustomFields, StepIdentities and StepWorkflows come before the
	// contacts and documents that use them.
	StepCustomFields = "custom_fields"
	StepIdentities   = "identities"
	StepWorkflows    = "workflows"
	StepContacts     = "contacts"
	StepProducts     = "products"
	StepProjects     = "projects"
	// StepFinancialAccounts and StepFinancialMutations bring the bank in
	// before the invoices, whose payments are made with its transactions.
	StepFinancialAccounts  = "financial_accounts"
	StepFinancialMutations = "financial_mutations"
	// StepEstimates and StepRecurring come before the invoices made from them.
	StepEstimates         = "estimates"
	StepRecurring         = "recurring_sales_invoices"
	StepSalesInvoices     = "sales_invoices"
	StepExternalInvoices  = "external_sales_invoices"
	StepPurchaseInvoices  = "purchase_invoices"
	StepReceipts          = "receipts"
	StepGeneralDocuments  = "general_documents"
	StepTypelessDocuments = "typeless_documents"
	StepJournal           = "general_journal_documents"
	// StepTimeEntries comes after the invoices the time was billed on.
	StepTimeEntries   = "time_entries"
	StepTaskTemplates = "task_list_templates"
	StepTaskLists     = "task_lists"
	// StepLockedPeriod locks the books as Moneybird had them, last, so the
	// lock stops nothing the import brings in.
	StepLockedPeriod = "locked_period"
)

// Steps lists the steps an import takes.
var Steps = []string{
	StepChart, StepCustomFields, StepIdentities, StepWorkflows, StepContacts, StepProducts, StepProjects, StepFinancialAccounts, StepFinancialMutations, StepEstimates, StepRecurring,
	StepSalesInvoices, StepExternalInvoices, StepPurchaseInvoices, StepReceipts, StepGeneralDocuments, StepTypelessDocuments, StepJournal,
	StepTimeEntries, StepTaskTemplates, StepTaskLists, StepLockedPeriod,
}

// maxProblems caps the problems a step keeps, so a step that fails on every
// record does not grow without bound.
const maxProblems = 50

// Step is the progress of one kind of data.
type Step struct {
	Kind string `json:"kind"`
	// Total is how many records Moneybird has, once known.
	Total    int  `json:"total"`
	Imported int  `json:"imported"`
	Skipped  int  `json:"skipped"`
	Failed   int  `json:"failed"`
	Done     bool `json:"done"`
	// Problems say why records failed, such as "contact 123: email is invalid".
	Problems []string `json:"problems"`
}

// note keeps what went wrong with part of a record, such as one of its
// files, which came in without it.
func (s *Step) note(format string, args ...any) {
	if len(s.Problems) < maxProblems {
		s.Problems = append(s.Problems, fmt.Sprintf(format, args...))
	}
}

func (s *Step) problem(format string, args ...any) {
	s.Failed++
	if len(s.Problems) < maxProblems {
		s.Problems = append(s.Problems, fmt.Sprintf(format, args...))
	}
}

// Problem is a record an import could not bring in, or brought in without
// part of it, with why.
type Problem struct {
	Step string
	// Kind and SourceID name the Moneybird record; SourceID is 0 when it had
	// none that could be read.
	Kind     string
	SourceID resource.ID
	Message  string
	// Partial is a record that came in without part of it, such as a file.
	Partial   bool
	CreatedAt time.Time
}

// Job is the import of one Moneybird administration into one here.
type Job struct {
	AdministrationID resource.ID
	// UserID is the owner who started it; the import acts with their access.
	UserID resource.ID
	// SourceID and SourceName are the Moneybird administration's.
	SourceID, SourceName string
	State                State
	Steps                []Step
	// WaitingUntil is when Moneybird's rate limit lets the import go on,
	// while it waits.
	WaitingUntil *time.Time
	Error        string
	// Token is the sealed API token, kept only while the job may run.
	Token    string
	Attempts int
	// NextAttemptAt is when the job runs next; the worker moves it ahead
	// while it runs, as a lease.
	NextAttemptAt *time.Time
	StartedAt     time.Time
	FinishedAt    *time.Time
	UpdatedAt     time.Time
}

// Step returns the job's step of a kind.
func (j *Job) Step(kind string) *Step {
	for i := range j.Steps {
		if j.Steps[i].Kind == kind {
			return &j.Steps[i]
		}
	}
	j.Steps = append(j.Steps, Step{Kind: kind})
	return &j.Steps[len(j.Steps)-1]
}

// SourceAdministration is a Moneybird administration the token reaches.
type SourceAdministration struct {
	ID, Name, Country, Currency string
	// PeriodLockedUntil is the last day of its locked period, yyyy-mm-dd, if any.
	PeriodLockedUntil string
}

// Version is a record's id and version in Moneybird, as its
// synchronization lists them.
type Version struct {
	ID      string
	Version int64
}

// Reader reads one Moneybird account's data. It keeps Moneybird's rate
// limit and calls back while it waits for it.
type Reader interface {
	List(ctx context.Context, adminID, path string, query url.Values) ([]json.RawMessage, error)
	SyncIDs(ctx context.Context, adminID, kind string, query url.Values) ([]Version, error)
	FetchByIDs(ctx context.Context, adminID, kind string, ids []string) ([]json.RawMessage, error)
	Get(ctx context.Context, adminID, path string, query url.Values) (json.RawMessage, error)
	// Download reads a file, such as an attachment, at a path below the
	// administration; it returns its content type too.
	Download(ctx context.Context, adminID, path string) ([]byte, string, error)
}

// Source opens Moneybird with an API token.
type Source interface {
	Administrations(ctx context.Context, token string) ([]SourceAdministration, error)
	Open(token string, onWait func(until time.Time)) Reader
}

// Store keeps jobs and what they imported.
type Store interface {
	Job(ctx context.Context, admin resource.ID) (Job, error)
	SaveJob(ctx context.Context, j Job) error
	// LeaseDueJobs takes running jobs due at now, moving them to lease.
	LeaseDueJobs(ctx context.Context, now, lease time.Time, max int) ([]Job, error)
	// Imported returns the id here of a record imported before.
	Imported(ctx context.Context, admin resource.ID, kind string, source resource.ID) (resource.ID, bool, error)
	RecordImported(ctx context.Context, admin resource.ID, kind string, source, target resource.ID) error
	// Empty reports whether the administration has none of the data an
	// import brings: no contacts, documents, transactions and the like.
	Empty(ctx context.Context, admin resource.ID) (bool, error)
	// RecordProblem keeps what went wrong with a record; ResolveProblems
	// forgets that a record failed once it came in.
	RecordProblem(ctx context.Context, admin resource.ID, p Problem) error
	ResolveProblems(ctx context.Context, admin resource.ID, kind string, source resource.ID) error
	// Problems returns one page of them, by step and record, with how many
	// there are in all.
	Problems(ctx context.Context, admin resource.ID, page, perPage int) ([]Problem, int, error)
}

// Secrets seals the API token while a job keeps it.
type Secrets interface {
	Seal(plain string) (string, error)
	Open(sealed string) (string, error)
}

// Administrations checks a user's access to an administration.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Service runs imports from Moneybird.
type Service struct {
	admins  Administrations
	source  Source
	store   Store
	secrets Secrets
	targets Targets
	now     func() time.Time
}

// NewService returns a Service.
func NewService(admins Administrations, source Source, store Store, secrets Secrets, targets Targets, now func() time.Time) *Service {
	return &Service{admins: admins, source: source, store: store, secrets: secrets, targets: targets, now: now}
}

// owner checks the user owns the administration: an import replaces its
// categories and VAT rates.
func (s *Service) owner(ctx context.Context, admin, user resource.ID) error {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return err
	}
	if !a.Can(administration.Ownership) {
		return membership.ErrForbidden
	}
	return nil
}

// Administrations lists the Moneybird administrations an API token reaches,
// to pick the one to import.
func (s *Service) Administrations(ctx context.Context, admin, user resource.ID, token string) ([]SourceAdministration, error) {
	if err := s.owner(ctx, admin, user); err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, tokenBlank()
	}
	return s.sourceAdministrations(ctx, token)
}

// sourceAdministrations asks Moneybird, telling a refused token from
// Moneybird being out of reach.
func (s *Service) sourceAdministrations(ctx context.Context, token string) ([]SourceAdministration, error) {
	list, err := s.source.Administrations(ctx, token)
	if err != nil && !errors.Is(err, ErrTokenRejected) && !errors.Is(err, context.Canceled) {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return list, err
}

func tokenBlank() error {
	var verr resource.ValidationError
	verr.Add("token", "can't be blank")
	return verr.OrNil()
}

// Start begins importing a Moneybird administration. The administration
// must be empty, unless it imported this Moneybird administration before:
// then the import adds what is new since.
func (s *Service) Start(ctx context.Context, admin, user resource.ID, token, sourceID string) (Job, error) {
	if err := s.owner(ctx, admin, user); err != nil {
		return Job{}, err
	}
	token, sourceID = strings.TrimSpace(token), strings.TrimSpace(sourceID)
	var verr resource.ValidationError
	if token == "" {
		verr.Add("token", "can't be blank")
	}
	if sourceID == "" {
		verr.Add("administration_id", "can't be blank")
	}
	if err := verr.OrNil(); err != nil {
		return Job{}, err
	}
	previous, err := s.store.Job(ctx, admin)
	switch {
	case errors.Is(err, ErrNotFound):
	case err != nil:
		return Job{}, err
	case previous.State == Running:
		return Job{}, ErrRunning
	}
	if previous.SourceID != sourceID {
		empty, err := s.store.Empty(ctx, admin)
		if err != nil {
			return Job{}, err
		}
		if !empty {
			return Job{}, ErrNotEmpty
		}
	}
	// The token must reach the administration; this also checks it works.
	sources, err := s.sourceAdministrations(ctx, token)
	if err != nil {
		return Job{}, err
	}
	var name string
	for _, a := range sources {
		if a.ID == sourceID {
			name = a.Name
		}
	}
	if name == "" {
		verr.Add("administration_id", "is not an administration this token reaches")
		return Job{}, verr.OrNil()
	}
	sealed, err := s.secrets.Seal(token)
	if err != nil {
		return Job{}, fmt.Errorf("seal token: %w", err)
	}
	now := s.now()
	j := Job{
		AdministrationID: admin, UserID: user, SourceID: sourceID, SourceName: name, State: Running,
		Token: sealed, NextAttemptAt: &now, StartedAt: now, UpdatedAt: now,
	}
	for _, kind := range Steps {
		j.Steps = append(j.Steps, Step{Kind: kind, Problems: []string{}})
	}
	if err := s.store.SaveJob(ctx, j); err != nil {
		return Job{}, fmt.Errorf("save import: %w", err)
	}
	return j, nil
}

// Status returns the administration's import.
func (s *Service) Status(ctx context.Context, admin, user resource.ID) (Job, error) {
	if err := s.owner(ctx, admin, user); err != nil {
		return Job{}, err
	}
	return s.store.Job(ctx, admin)
}

// TryNow brings an import queued after a failure forward, so it tries again
// at once rather than after its pause, such as once the cause was fixed.
func (s *Service) TryNow(ctx context.Context, admin, user resource.ID) (Job, error) {
	if err := s.owner(ctx, admin, user); err != nil {
		return Job{}, err
	}
	j, err := s.store.Job(ctx, admin)
	if err != nil {
		return Job{}, err
	}
	now := s.now()
	if j.State != Running || j.Error == "" || j.NextAttemptAt == nil || !j.NextAttemptAt.After(now) {
		return Job{}, ErrNotQueued
	}
	j.NextAttemptAt, j.UpdatedAt = &now, now
	if err := s.store.SaveJob(ctx, j); err != nil {
		return Job{}, fmt.Errorf("save import: %w", err)
	}
	return j, nil
}

// Problems returns one page of what the administration's import could not
// bring in, with how many in all.
func (s *Service) Problems(ctx context.Context, admin, user resource.ID, page, perPage int) ([]Problem, int, error) {
	if err := s.owner(ctx, admin, user); err != nil {
		return nil, 0, err
	}
	return s.store.Problems(ctx, admin, max(page, 1), min(max(perPage, 1), 100))
}

// describe says why a record could not come in: for a validation error,
// what is wrong with each field.
func describe(err error) string {
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		return err.Error()
	}
	fields := slices.Sorted(maps.Keys(verr.Fields))
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, f+": "+strings.Join(verr.Fields[f], ", "))
	}
	// What wrapped it says where, such as which payment.
	where, _ := strings.CutSuffix(err.Error(), verr.Error())
	if where == err.Error() {
		where = ""
	}
	return where + strings.Join(parts, "; ")
}
