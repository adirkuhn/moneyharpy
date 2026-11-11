// Package timeentries holds the time members spend, after Moneybird's time
// entries: a description, a start and an end, for a contact and a project,
// billable or not. An entry without an end is a running timer. Billable
// time ends up on a sales invoice, after which the entry is fixed.
package timeentries

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// EntityType names time entries in notes and events, as Moneybird does.
const EntityType = "TimeEntry"

// Errors of the use cases.
var (
	// ErrNotFound reports a time entry the administration does not have.
	ErrNotFound = errors.New("time entry not found")
	// ErrForbidden reports another member's entry to someone who may not
	// work on invoices, as Moneybird asks the sales_invoices scope for it.
	ErrForbidden = errors.New("time entry of another user")
)

// IsNotFound reports errors that mean the entry or its note is missing.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, notes.ErrNotFound)
}

// States, with Moneybird's filter names: open entries are billable and not
// on an invoice yet.
const (
	All         = "all"
	Open        = "open"
	NonBillable = "non_billable"
)

// Paging, as at Moneybird.
const (
	DefaultPerPage = 25
	MaxPerPage     = 100
)

// MaxDescription caps a description.
const MaxDescription = 10000

// TimeEntry is time a member spent.
type TimeEntry struct {
	ID               resource.ID
	AdministrationID resource.ID
	UserID           resource.ID
	ContactID        *resource.ID
	ProjectID        *resource.ID
	// SalesInvoiceID is the invoice the time is billed on.
	SalesInvoiceID *resource.ID
	StartedAt      time.Time
	// EndedAt is nil while the timer runs.
	EndedAt     *time.Time
	Description string
	// PausedDuration is in seconds, whole minutes.
	PausedDuration int
	Billable       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Notes          []notes.Note
}

// Ref locates the entry.
func (e TimeEntry) Ref() resource.Ref {
	return resource.Ref{ID: e.ID, Type: resource.TypeTimeEntry, AdministrationID: e.AdministrationID}
}

// Running reports a timer that has not stopped.
func (e TimeEntry) Running() bool { return e.EndedAt == nil }

// Invoiced reports time billed on a sales invoice.
func (e TimeEntry) Invoiced() bool { return e.SalesInvoiceID != nil }

// Duration is the time worked: from start to end, or to now while
// running, less the pauses.
func (e TimeEntry) Duration(now time.Time) time.Duration {
	end := now
	if e.EndedAt != nil {
		end = *e.EndedAt
	}
	return max(end.Sub(e.StartedAt)-time.Duration(e.PausedDuration)*time.Second, 0)
}

// User is a member as entries show them.
type User struct {
	ID        resource.ID
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Input holds the fields to set; nil fields stay as they are. An id of 0
// clears a reference.
type Input struct {
	UserID         *resource.ID
	ContactID      *resource.ID
	ProjectID      *resource.ID
	SalesInvoiceID *resource.ID
	StartedAt      *time.Time
	EndedAt        *time.Time
	Description    *string
	PausedDuration *int
	Billable       *bool
}

// Actor is who works on entries: anyone may work on their own, only those
// who may work on invoices on other members' entries.
type Actor struct {
	ID       resource.ID
	AllUsers bool
}

func (a Actor) may(e TimeEntry) bool { return a.AllUsers || e.UserID == a.ID }

// Query selects time entries.
type Query struct {
	// States keeps entries in one of these states; empty keeps all.
	States []string
	// Period keeps entries started on these days, in Location.
	From, To *time.Time
	// Day keeps entries started on this day, in Location.
	Day      *time.Time
	Location *time.Location
	// ContactID keeps the contact's entries, and those without a contact
	// when IncludeNilContacts.
	ContactID          *resource.ID
	IncludeNilContacts bool
	// IncludeActive keeps running timers too.
	IncludeActive bool
	ProjectID     *resource.ID
	UserID        *resource.ID
	// Search matches the description.
	Search        string
	Page, PerPage int
}

func (q Query) loc() *time.Location {
	if q.Location == nil {
		return time.UTC
	}
	return q.Location
}

// Matches reports whether the query selects the entry; the reference for
// stores without a query language of their own.
func (q Query) Matches(e TimeEntry) bool {
	day := dayOf(e.StartedAt, q.loc())
	switch {
	case !q.IncludeActive && e.Running(),
		q.From != nil && day.Before(*q.From), q.To != nil && day.After(*q.To),
		q.Day != nil && !day.Equal(*q.Day),
		q.ProjectID != nil && (e.ProjectID == nil || *e.ProjectID != *q.ProjectID),
		q.UserID != nil && e.UserID != *q.UserID,
		q.Search != "" && !strings.Contains(strings.ToLower(e.Description), strings.ToLower(q.Search)):
		return false
	}
	if q.ContactID != nil && (e.ContactID == nil && !q.IncludeNilContacts || e.ContactID != nil && *e.ContactID != *q.ContactID) {
		return false
	}
	if len(q.States) == 0 {
		return true
	}
	return slices.Contains(q.States, Open) && e.Billable && !e.Invoiced() || slices.Contains(q.States, NonBillable) && !e.Billable
}

func dayOf(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Administrations checks access to an administration.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Store persists time entries.
type Store interface {
	// Entries lists the entries the query selects, newest start first,
	// with the number selected in all.
	Entries(ctx context.Context, admin resource.ID, q Query) ([]TimeEntry, int, error)
	Entry(ctx context.Context, admin, id resource.ID) (TimeEntry, error)
	// RunningOf returns the user's running timers.
	RunningOf(ctx context.Context, admin, user resource.ID) ([]TimeEntry, error)
	// OfInvoices returns the entries billed on the invoices.
	OfInvoices(ctx context.Context, admin resource.ID, invoices []resource.ID) ([]TimeEntry, error)
	SaveEntry(ctx context.Context, e TimeEntry, created bool) error
	DeleteEntry(ctx context.Context, admin, id resource.ID) error
	// Users returns the administration's members among ids.
	Users(ctx context.Context, admin resource.ID, ids []resource.ID) (map[resource.ID]User, error)
	ContactExists(ctx context.Context, admin, id resource.ID) (bool, error)
	ProjectExists(ctx context.Context, admin, id resource.ID) (bool, error)
	InvoiceExists(ctx context.Context, admin, id resource.ID) (bool, error)
}

// Notes keeps notes on entries.
type Notes interface {
	Add(ctx context.Context, admin resource.ID, entityType string, entity, user resource.ID, in notes.Input) (notes.Note, error)
	Delete(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) error
	Of(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]notes.Note, error)
}

// Service implements the time entry use cases.
type Service struct {
	admins Administrations
	store  Store
	notes  Notes
	ids    *resource.Generator
	now    func() time.Time
}

// NewService returns a Service.
func NewService(admins Administrations, store Store, notes Notes, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, store: store, notes: notes, ids: ids, now: now}
}

func (s *Service) location(ctx context.Context, admin resource.ID, user resource.ID) (*time.Location, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(a.TimeZone)
	if err != nil {
		return time.UTC, nil //nolint:nilerr // An unknown zone counts as UTC.
	}
	return loc, nil
}

// List returns the page of entries the query selects. Those who may not
// see other members' entries see their own only.
func (s *Service) List(ctx context.Context, admin resource.ID, actor Actor, q Query) ([]TimeEntry, int, error) {
	loc, err := s.location(ctx, admin, actor.ID)
	if err != nil {
		return nil, 0, err
	}
	if q.Location == nil {
		q.Location = loc
	}
	if !actor.AllUsers {
		if q.UserID != nil && *q.UserID != actor.ID {
			return []TimeEntry{}, 0, nil
		}
		q.UserID = &actor.ID
	}
	q.Page = max(q.Page, 1)
	if q.PerPage <= 0 {
		q.PerPage = DefaultPerPage
	}
	q.PerPage = min(q.PerPage, MaxPerPage)
	list, total, err := s.store.Entries(ctx, admin, q)
	if err != nil {
		return nil, 0, fmt.Errorf("list time entries: %w", err)
	}
	if err := s.withNotes(ctx, admin, list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (s *Service) withNotes(ctx context.Context, admin resource.ID, list []TimeEntry) error {
	ids := make([]resource.ID, len(list))
	for i, e := range list {
		ids[i] = e.ID
	}
	byID, err := s.notes.Of(ctx, admin, EntityType, ids)
	if err != nil {
		return err
	}
	for i := range list {
		list[i].Notes = byID[list[i].ID]
	}
	return nil
}

// Entry returns one entry with its notes.
func (s *Service) Entry(ctx context.Context, admin, id resource.ID, actor Actor) (TimeEntry, error) {
	if _, err := s.admins.Get(ctx, admin, actor.ID); err != nil {
		return TimeEntry{}, err
	}
	return s.load(ctx, admin, id, actor)
}

func (s *Service) load(ctx context.Context, admin, id resource.ID, actor Actor) (TimeEntry, error) {
	e, err := s.store.Entry(ctx, admin, id)
	if err != nil {
		return TimeEntry{}, err
	}
	if !actor.may(e) {
		return TimeEntry{}, ErrForbidden
	}
	list := []TimeEntry{e}
	if err := s.withNotes(ctx, admin, list); err != nil {
		return TimeEntry{}, err
	}
	return list[0], nil
}

// Users returns the members among ids, as entries show them.
func (s *Service) Users(ctx context.Context, admin resource.ID, ids []resource.ID) (map[resource.ID]User, error) {
	return s.store.Users(ctx, admin, ids)
}

// Create records time. Without an end it starts a timer, which stops the
// user's running one.
func (s *Service) Create(ctx context.Context, admin resource.ID, actor Actor, in Input) (TimeEntry, error) {
	if _, err := s.admins.Get(ctx, admin, actor.ID); err != nil {
		return TimeEntry{}, err
	}
	now := s.now()
	e := TimeEntry{ID: s.ids.Next(), AdministrationID: admin, Billable: true, CreatedAt: now, UpdatedAt: now}
	var verr resource.ValidationError
	if in.UserID == nil || *in.UserID == 0 {
		verr.Add("user_id", "can't be blank")
	}
	if in.StartedAt == nil {
		verr.Add("started_at", "can't be blank")
	}
	if in.Description == nil {
		verr.Add("description", "can't be blank")
	}
	if err := verr.OrNil(); err != nil {
		return TimeEntry{}, err
	}
	if *in.UserID != actor.ID && !actor.AllUsers {
		return TimeEntry{}, ErrForbidden
	}
	if err := s.apply(ctx, &e, in); err != nil {
		return TimeEntry{}, err
	}
	if e.Running() {
		if err := s.stopOthers(ctx, admin, e.UserID, e.ID, now); err != nil {
			return TimeEntry{}, err
		}
	}
	if err := s.store.SaveEntry(ctx, e, true); err != nil {
		return TimeEntry{}, fmt.Errorf("store time entry: %w", err)
	}
	e.Notes = []notes.Note{}
	return e, nil
}

// Update changes an entry that is not on an invoice yet.
func (s *Service) Update(ctx context.Context, admin, id resource.ID, actor Actor, in Input) (TimeEntry, error) {
	e, err := s.Entry(ctx, admin, id, actor)
	if err != nil {
		return TimeEntry{}, err
	}
	if e.Invoiced() {
		return TimeEntry{}, invoiced()
	}
	if in.UserID != nil && *in.UserID != actor.ID && !actor.AllUsers {
		return TimeEntry{}, ErrForbidden
	}
	if err := s.apply(ctx, &e, in); err != nil {
		return TimeEntry{}, err
	}
	e.UpdatedAt = s.now()
	if err := s.store.SaveEntry(ctx, e, false); err != nil {
		return TimeEntry{}, fmt.Errorf("store time entry: %w", err)
	}
	return e, nil
}

func invoiced() error {
	var verr resource.ValidationError
	verr.Add("id", "is invalid: the time entry is on a sales invoice")
	return &verr
}

func (s *Service) apply(ctx context.Context, e *TimeEntry, in Input) error {
	var verr resource.ValidationError
	admin := e.AdministrationID
	if in.UserID != nil {
		users, err := s.store.Users(ctx, admin, []resource.ID{*in.UserID})
		if err != nil {
			return fmt.Errorf("check user: %w", err)
		}
		if _, ok := users[*in.UserID]; ok {
			e.UserID = *in.UserID
		} else {
			verr.Add("user_id", "is not a user of this administration")
		}
	}
	for _, ref := range []struct {
		field  string
		in     *resource.ID
		dst    **resource.ID
		exists func(context.Context, resource.ID, resource.ID) (bool, error)
	}{
		{"contact_id", in.ContactID, &e.ContactID, s.store.ContactExists},
		{"project_id", in.ProjectID, &e.ProjectID, s.store.ProjectExists},
		{"sales_invoice_id", in.SalesInvoiceID, &e.SalesInvoiceID, s.store.InvoiceExists},
	} {
		if ref.in == nil {
			continue
		}
		if *ref.in == 0 {
			*ref.dst = nil
			continue
		}
		ok, err := ref.exists(ctx, admin, *ref.in)
		if err != nil {
			return fmt.Errorf("check %s: %w", ref.field, err)
		}
		if !ok {
			verr.Add(ref.field, "is not found")
			continue
		}
		id := *ref.in
		*ref.dst = &id
	}
	if in.Description != nil {
		e.Description = strings.TrimSpace(*in.Description)
	}
	switch {
	case e.Description == "":
		verr.Add("description", "can't be blank")
	case len(e.Description) > MaxDescription:
		verr.Add("description", fmt.Sprintf("is too long (maximum is %d characters)", MaxDescription))
	}
	if in.Billable != nil {
		e.Billable = *in.Billable
	}
	if in.StartedAt != nil {
		e.StartedAt = in.StartedAt.UTC().Truncate(time.Minute)
	}
	if in.EndedAt != nil {
		end := in.EndedAt.UTC().Truncate(time.Minute)
		e.EndedAt = &end
	}
	if in.PausedDuration != nil {
		if *in.PausedDuration < 0 {
			verr.Add("paused_duration", "must be greater than or equal to 0")
		} else {
			e.PausedDuration = *in.PausedDuration / 60 * 60
		}
	}
	if e.EndedAt != nil {
		switch {
		case e.EndedAt.Before(e.StartedAt):
			verr.Add("ended_at", "must be after started_at")
		case e.EndedAt.Sub(e.StartedAt) < time.Minute:
			// Moneybird keeps at least a minute between start and end.
			end := e.StartedAt.Add(time.Minute)
			e.EndedAt = &end
		}
		if time.Duration(e.PausedDuration)*time.Second >= e.EndedAt.Sub(e.StartedAt) && e.PausedDuration > 0 {
			verr.Add("paused_duration", "must be shorter than the time between started_at and ended_at")
		}
	}
	return verr.OrNil()
}

// stopOthers stops the user's running timers other than except.
func (s *Service) stopOthers(ctx context.Context, admin, user, except resource.ID, now time.Time) error {
	running, err := s.store.RunningOf(ctx, admin, user)
	if err != nil {
		return fmt.Errorf("find running timers: %w", err)
	}
	for _, r := range running {
		if r.ID == except {
			continue
		}
		stop(&r, now)
		if err := s.store.SaveEntry(ctx, r, false); err != nil {
			return fmt.Errorf("stop timer: %w", err)
		}
	}
	return nil
}

func stop(e *TimeEntry, now time.Time) {
	end := now.UTC().Truncate(time.Minute)
	if least := e.StartedAt.Add(time.Minute); end.Before(least) {
		end = least
	}
	e.EndedAt, e.UpdatedAt = &end, now
}

// Stop ends a running timer; a stopped entry comes back unchanged.
func (s *Service) Stop(ctx context.Context, admin, id resource.ID, actor Actor) (TimeEntry, error) {
	e, err := s.Entry(ctx, admin, id, actor)
	if err != nil || !e.Running() {
		return e, err
	}
	stop(&e, s.now())
	if err := s.store.SaveEntry(ctx, e, false); err != nil {
		return TimeEntry{}, fmt.Errorf("store time entry: %w", err)
	}
	return e, nil
}

// Resume restarts a timer that started today, in the administration's
// time zone; the time it was stopped counts as a pause. The user's other
// running timer stops first. A running entry comes back unchanged.
func (s *Service) Resume(ctx context.Context, admin, id resource.ID, actor Actor) (TimeEntry, error) {
	loc, err := s.location(ctx, admin, actor.ID)
	if err != nil {
		return TimeEntry{}, err
	}
	e, err := s.load(ctx, admin, id, actor)
	if err != nil || e.Running() {
		return e, err
	}
	now := s.now()
	if e.Invoiced() || !dayOf(e.StartedAt, loc).Equal(dayOf(now, loc)) {
		var verr resource.ValidationError
		verr.Add("base", "Cannot resume timer")
		return TimeEntry{}, &verr
	}
	if err := s.stopOthers(ctx, admin, e.UserID, e.ID, now); err != nil {
		return TimeEntry{}, err
	}
	pause := now.UTC().Truncate(time.Minute).Sub(*e.EndedAt)
	e.PausedDuration += int(max(pause, 0) / time.Second)
	e.EndedAt, e.UpdatedAt = nil, now
	if err := s.store.SaveEntry(ctx, e, false); err != nil {
		return TimeEntry{}, fmt.Errorf("store time entry: %w", err)
	}
	return e, nil
}

// Delete removes an entry that is not on an invoice.
func (s *Service) Delete(ctx context.Context, admin, id resource.ID, actor Actor) error {
	e, err := s.Entry(ctx, admin, id, actor)
	if err != nil {
		return err
	}
	if e.Invoiced() {
		return invoiced()
	}
	return s.store.DeleteEntry(ctx, admin, id)
}

// AddNote adds a note to an entry.
func (s *Service) AddNote(ctx context.Context, admin, id resource.ID, actor Actor, in notes.Input) (notes.Note, error) {
	if _, err := s.Entry(ctx, admin, id, actor); err != nil {
		return notes.Note{}, err
	}
	return s.notes.Add(ctx, admin, EntityType, id, actor.ID, in)
}

// DeleteNote removes a note from an entry.
func (s *Service) DeleteNote(ctx context.Context, admin, id, note resource.ID, actor Actor) error {
	if _, err := s.Entry(ctx, admin, id, actor); err != nil {
		return err
	}
	return s.notes.Delete(ctx, admin, EntityType, id, note)
}

// CheckBillable verifies entries can be billed on an invoice: they exist,
// have stopped and are on no other invoice. invoice is 0 for a new one.
func (s *Service) CheckBillable(ctx context.Context, admin resource.ID, actor Actor, ids []resource.ID, invoice resource.ID) error {
	var verr resource.ValidationError
	for _, id := range ids {
		e, err := s.store.Entry(ctx, admin, id)
		switch {
		case errors.Is(err, ErrNotFound):
			verr.Add("time_entry_ids", "contains an unknown time entry: "+id.String())
		case err != nil:
			return err
		case !actor.may(e):
			return ErrForbidden
		case e.Running():
			verr.Add("time_entry_ids", "contains a running timer: "+id.String())
		case e.Invoiced() && *e.SalesInvoiceID != invoice:
			verr.Add("time_entry_ids", "contains time already on another invoice: "+id.String())
		}
	}
	return verr.OrNil()
}

// Bill puts the entries on the invoice, after CheckBillable, and returns
// them.
func (s *Service) Bill(ctx context.Context, admin, invoice resource.ID, actor Actor, ids []resource.ID) ([]TimeEntry, error) {
	if err := s.CheckBillable(ctx, admin, actor, ids, invoice); err != nil {
		return nil, err
	}
	now := s.now()
	out := make([]TimeEntry, 0, len(ids))
	for _, id := range ids {
		e, err := s.store.Entry(ctx, admin, id)
		if err != nil {
			return nil, err
		}
		if e.SalesInvoiceID == nil {
			inv := invoice
			e.SalesInvoiceID, e.UpdatedAt = &inv, now
			if err := s.store.SaveEntry(ctx, e, false); err != nil {
				return nil, fmt.Errorf("bill time entry: %w", err)
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// OfInvoices returns the entries billed on each invoice.
func (s *Service) OfInvoices(ctx context.Context, admin resource.ID, invoices []resource.ID) (map[resource.ID][]TimeEntry, error) {
	out := map[resource.ID][]TimeEntry{}
	if len(invoices) == 0 {
		return out, nil
	}
	list, err := s.store.OfInvoices(ctx, admin, invoices)
	if err != nil {
		return nil, fmt.Errorf("load billed time: %w", err)
	}
	for _, e := range list {
		out[*e.SalesInvoiceID] = append(out[*e.SalesInvoiceID], e)
	}
	return out, nil
}

// ParseFilter reads Moneybird's time entry filter, as key:value terms
// separated by commas: state (all, open, non_billable, several joined by
// |), period, contact_id, include_nil_contacts, include_active, project_id,
// user_id and day. Without a filter, period:this_year applies; any filter
// replaces that default. today is in the administration's time zone.
func ParseFilter(filter string, today time.Time) (Query, error) {
	if strings.TrimSpace(filter) == "" {
		filter = "period:this_year"
	}
	var q Query
	var verr resource.ValidationError
	id := func(key, v string) *resource.ID {
		n, err := resource.ParseID(v)
		if err != nil {
			verr.Add("filter", key+" "+v+" is not an id")
			return nil
		}
		return &n
	}
	flag := func(key, v string) bool {
		switch v {
		case "true":
			return true
		case "false":
			return false
		}
		verr.Add("filter", key+" "+v+" is not true or false")
		return false
	}
	for term := range strings.SplitSeq(filter, ",") {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		key, value, ok := strings.Cut(term, ":")
		if !ok {
			verr.Add("filter", "has an unknown term: "+term)
			continue
		}
		switch key {
		case "state":
			q.States = nil
			all := false
			for v := range strings.SplitSeq(value, "|") {
				switch v {
				case All:
					all = true
				case Open, NonBillable:
					q.States = append(q.States, v)
				default:
					verr.Add("filter", "state "+v+" is not one of all, open, non_billable")
				}
			}
			if all {
				q.States = nil
			}
		case "period":
			p, err := money.ParsePeriod(value, today)
			if err != nil {
				verr.Add("filter", "period "+value+" "+err.Error())
				continue
			}
			q.From, q.To = &p.From, &p.To
		case "day":
			d, err := time.Parse(time.DateOnly, value)
			if err != nil {
				verr.Add("filter", "day "+value+" is not a date (YYYY-MM-DD)")
				continue
			}
			q.Day = &d
		case "contact_id":
			q.ContactID = id(key, value)
		case "project_id":
			q.ProjectID = id(key, value)
		case "user_id":
			q.UserID = id(key, value)
		case "include_nil_contacts":
			q.IncludeNilContacts = flag(key, value)
		case "include_active":
			q.IncludeActive = flag(key, value)
		default:
			verr.Add("filter", "has an unknown term: "+term)
		}
	}
	return q, verr.OrNil()
}
