package timeentries

import (
	"context"
	"fmt"

	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Import records time brought in from another system, such as Moneybird,
// keeping the entry's id and times. It checks the entry as Create does,
// but leaves the user's running timers alone. One that is here already
// stays as it is.
func (s *Service) Import(ctx context.Context, admin resource.ID, actor Actor, o resource.Origin, in Input) (TimeEntry, error) {
	if _, err := s.admins.Get(ctx, admin, actor.ID); err != nil {
		return TimeEntry{}, err
	}
	if e, err := s.store.Entry(ctx, admin, o.ID); err == nil {
		return e, nil
	}
	created, updated := o.Or(s.now())
	e := TimeEntry{ID: o.ID, AdministrationID: admin, Billable: true, CreatedAt: created, UpdatedAt: updated}
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
	if err := s.store.SaveEntry(ctx, e, true); err != nil {
		return TimeEntry{}, fmt.Errorf("store time entry: %w", err)
	}
	e.Notes = []notes.Note{}
	return e, nil
}
