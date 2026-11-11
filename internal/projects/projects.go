// Package projects holds an administration's projects, after Moneybird's:
// named, with an optional budget in hours, active or archived. Invoice
// lines and journal entries can be booked on a project.
package projects

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ErrNotFound reports a project the administration does not have.
var ErrNotFound = errors.New("project not found")

// States, with Moneybird's names.
const (
	Active   = "active"
	Archived = "archived"
)

// Paging, as at Moneybird.
const (
	DefaultPerPage = 25
	MaxPerPage     = 100
)

// maxBudget bounds a budget, in hours.
const maxBudget = 1_000_000_000

// Project is something lines and journal entries are booked on.
type Project struct {
	ID               resource.ID
	AdministrationID resource.ID
	Name             string
	State            string
	// Budget is in hours.
	Budget    *int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Ref locates the project.
func (p Project) Ref() resource.Ref {
	return resource.Ref{ID: p.ID, Type: resource.TypeProject, AdministrationID: p.AdministrationID}
}

// Active reports whether lines can still be booked on the project.
func (p Project) Active() bool { return p.State == Active }

// Changes holds the fields to set; nil fields stay as they are. An empty
// Budget removes it.
type Changes struct {
	Name   *string
	Budget *string
	// State archives or reactivates the project; a MoneyHarpy extension.
	State *string
}

// Query selects projects.
type Query struct {
	// States keeps projects in one of these states; empty keeps all.
	States        []string
	Page, PerPage int
}

// Matches reports whether the query selects the project; the reference for
// stores without a query language of their own.
func (q Query) Matches(p Project) bool {
	return len(q.States) == 0 || slices.Contains(q.States, p.State)
}

// ParseFilter reads Moneybird's project filter: state, one of all, active
// or archived, several joined by |. Active projects unless given.
func ParseFilter(filter string) (Query, error) {
	q := Query{States: []string{Active}}
	var verr resource.ValidationError
	for term := range strings.SplitSeq(filter, ",") {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		key, value, ok := strings.Cut(term, ":")
		if !ok || key != "state" {
			verr.Add("filter", "has an unknown term: "+term)
			continue
		}
		states, all := []string{}, false
		for v := range strings.SplitSeq(value, "|") {
			switch v {
			case "all":
				all = true
			case Active, Archived:
				states = append(states, v)
			default:
				verr.Add("filter", "state "+v+" is not one of all, active, archived")
			}
		}
		q.States = states
		if all {
			q.States = nil
		}
	}
	return q, verr.OrNil()
}

// Administrations checks access to an administration.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Store persists projects.
type Store interface {
	// Projects lists the projects the query selects by name, with the
	// number selected in all.
	Projects(ctx context.Context, admin resource.ID, q Query) ([]Project, int, error)
	Project(ctx context.Context, admin, id resource.ID) (Project, error)
	// NameTaken reports whether another project than except has the name,
	// ignoring case.
	NameTaken(ctx context.Context, admin resource.ID, name string, except resource.ID) (bool, error)
	SaveProject(ctx context.Context, p Project, created bool) error
	DeleteProject(ctx context.Context, admin, id resource.ID) error
}

// References tells whether documents are booked on a project.
type References interface {
	ProjectInUse(ctx context.Context, admin, project resource.ID) (bool, error)
}

// AnyReferences reports a project in use when any of its members does.
type AnyReferences []References

// ProjectInUse implements References.
func (refs AnyReferences) ProjectInUse(ctx context.Context, admin, project resource.ID) (bool, error) {
	for _, r := range refs {
		used, err := r.ProjectInUse(ctx, admin, project)
		if err != nil || used {
			return used, err
		}
	}
	return false, nil
}

// Service implements the project use cases.
type Service struct {
	admins     Administrations
	store      Store
	references References
	ids        *resource.Generator
	now        func() time.Time
}

// NewService returns a Service. references may be nil.
func NewService(admins Administrations, store Store, references References, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, store: store, references: references, ids: ids, now: now}
}

// List returns the page of projects the query selects, by name.
func (s *Service) List(ctx context.Context, admin, user resource.ID, q Query) ([]Project, int, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, 0, err
	}
	q.Page = max(q.Page, 1)
	if q.PerPage <= 0 {
		q.PerPage = DefaultPerPage
	}
	q.PerPage = min(q.PerPage, MaxPerPage)
	list, total, err := s.store.Projects(ctx, admin, q)
	if err != nil {
		return nil, 0, fmt.Errorf("list projects: %w", err)
	}
	return list, total, nil
}

// Project returns one project.
func (s *Service) Project(ctx context.Context, admin, id, user resource.ID) (Project, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Project{}, err
	}
	return s.store.Project(ctx, admin, id)
}

// CheckProject returns a validation error on project_id unless the project
// exists and is active, for records booked on it.
func (s *Service) CheckProject(ctx context.Context, admin, user, id resource.ID) error {
	p, err := s.Project(ctx, admin, id, user)
	var verr resource.ValidationError
	switch {
	case errors.Is(err, ErrNotFound):
		verr.Add("project_id", "does not exist")
	case err != nil:
		return err
	case !p.Active():
		verr.Add("project_id", "is archived")
	}
	return verr.OrNil()
}

// Create adds an active project; its name must be new.
func (s *Service) Create(ctx context.Context, admin, user resource.ID, c Changes) (Project, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Project{}, err
	}
	now := s.now()
	p := Project{ID: s.ids.Next(), AdministrationID: admin, State: Active, CreatedAt: now, UpdatedAt: now}
	if err := s.apply(ctx, &p, c); err != nil {
		return Project{}, err
	}
	if err := s.store.SaveProject(ctx, p, true); err != nil {
		return Project{}, fmt.Errorf("store project: %w", err)
	}
	return p, nil
}

// Update changes a project.
func (s *Service) Update(ctx context.Context, admin, id, user resource.ID, c Changes) (Project, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Project{}, err
	}
	p, err := s.store.Project(ctx, admin, id)
	if err != nil {
		return Project{}, err
	}
	if err := s.apply(ctx, &p, c); err != nil {
		return Project{}, err
	}
	p.UpdatedAt = s.now()
	if err := s.store.SaveProject(ctx, p, false); err != nil {
		return Project{}, fmt.Errorf("store project: %w", err)
	}
	return p, nil
}

func (s *Service) apply(ctx context.Context, p *Project, c Changes) error {
	var verr resource.ValidationError
	if c.Name != nil {
		p.Name = strings.TrimSpace(*c.Name)
	}
	switch {
	case p.Name == "":
		verr.Add("name", "can't be blank")
	case len(p.Name) > 255:
		verr.Add("name", "is too long (maximum is 255 characters)")
	default:
		taken, err := s.store.NameTaken(ctx, p.AdministrationID, p.Name, p.ID)
		if err != nil {
			return fmt.Errorf("check project name: %w", err)
		}
		if taken {
			verr.Add("name", "has already been taken")
		}
	}
	if c.Budget != nil {
		budget, err := ParseBudget(*c.Budget)
		if err != nil {
			verr.Add("budget", err.Error())
		} else {
			p.Budget = budget
		}
	}
	if c.State != nil {
		switch *c.State {
		case Active, Archived:
			p.State = *c.State
		default:
			verr.Add("state", "is not one of active, archived")
		}
	}
	return verr.OrNil()
}

// ParseBudget reads a budget in hours, such as 10, "10,5" or "12.0",
// rounded to whole hours; empty means none.
func ParseBudget(raw string) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(strings.Replace(raw, ",", ".", 1), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, errors.New("is not a number")
	}
	if f < 0 || f > maxBudget {
		return nil, fmt.Errorf("must be from 0 to %d hours", maxBudget)
	}
	n := int(math.Round(f))
	return &n, nil
}

// Delete removes a project nothing is booked on. One that documents are
// booked on is archived instead, as Moneybird does, and kept reports so.
func (s *Service) Delete(ctx context.Context, admin, id, user resource.ID) (kept bool, err error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return false, err
	}
	p, err := s.store.Project(ctx, admin, id)
	if err != nil {
		return false, err
	}
	if s.references != nil {
		used, err := s.references.ProjectInUse(ctx, admin, id)
		if err != nil {
			return false, fmt.Errorf("check project use: %w", err)
		}
		if used {
			p.State, p.UpdatedAt = Archived, s.now()
			if err := s.store.SaveProject(ctx, p, false); err != nil {
				return false, fmt.Errorf("archive project: %w", err)
			}
			return true, nil
		}
	}
	return false, s.store.DeleteProject(ctx, admin, id)
}
