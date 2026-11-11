package assets

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Run books the monthly depreciation every interval until ctx ends.
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

// Work does one round: each administration, held by one instance at a
// time, gets the linear value changes of its assets that are due. A month
// is due at its end, from the month the asset was added in and at most a
// month back, so a missed round catches up while months before the asset
// existed wait for AddRetroactiveLinear, as at Moneybird. Months already
// booked are skipped, so rounds can repeat.
func (s *Service) Work(ctx context.Context) error {
	list, err := s.store.Workload(ctx)
	if err != nil {
		return fmt.Errorf("list asset workload: %w", err)
	}
	var errs []error
	for _, w := range list {
		if ctx.Err() != nil {
			break
		}
		release, ok, err := s.store.Lock(ctx, w.AdministrationID)
		if err != nil {
			errs = append(errs, fmt.Errorf("lock administration %s: %w", w.AdministrationID, err))
			continue
		}
		if !ok {
			continue
		}
		if err := s.workOn(ctx, w.AdministrationID, w.UserID); err != nil {
			errs = append(errs, fmt.Errorf("administration %s: %w", w.AdministrationID, err))
		}
		release()
	}
	return errors.Join(errs...)
}

func (s *Service) workOn(ctx context.Context, admin, user resource.ID) error {
	adm, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return err
	}
	today := s.today(adm)
	locked, err := s.lockedUntil(ctx, admin, user)
	if err != nil {
		return err
	}
	list, _, err := s.store.Assets(ctx, admin, Query{OnlyActive: true})
	if err != nil {
		return err
	}
	earliest := time.Date(today.Year(), today.Month()-1, 1, 0, 0, 0, 0, time.UTC)
	var errs []error
	for _, a := range list {
		added := s.dayIn(adm, a.CreatedAt)
		var due []time.Time
		for _, d := range a.Missing(today) {
			if d.Before(added) || d.Before(earliest) || locked != nil && !d.After(*locked) {
				continue
			}
			due = append(due, d)
		}
		if len(due) == 0 {
			continue
		}
		if err := s.bookLinear(ctx, admin, user, a, due); err != nil {
			errs = append(errs, fmt.Errorf("asset %s: %w", a.ID, err))
		}
	}
	return errors.Join(errs...)
}

// dayIn is the date of t in the administration's time zone.
func (s *Service) dayIn(a administration.Listed, t time.Time) time.Time {
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		t = t.In(loc)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
