package assets

import (
	"context"
	"errors"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ImportedAsset is an asset brought in from another system, such as
// Moneybird, as it was there: its value changes as that system booked
// them, and the purchase lines or journal rows it came from.
type ImportedAsset struct {
	Origin resource.Origin
	Input  Input
	// Sources are ids of purchase invoice or receipt lines, or of journal
	// entry rows; those that are not here are left out.
	Sources      []resource.ID
	ValueChanges []ValueChange
}

// Import adds an asset brought in from another system, keeping its id and
// booking its value changes as they were, the locked period too. The
// months after go on as for any asset. Bringing in an asset that is here
// already changes nothing.
func (s *Service) Import(ctx context.Context, admin, user resource.ID, in ImportedAsset) (Asset, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Asset{}, err
	}
	if a, err := s.store.Asset(ctx, admin, in.Origin.ID); err == nil {
		return a, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Asset{}, err
	}
	created, updated := in.Origin.Or(s.now())
	a := Asset{ID: in.Origin.ID, AdministrationID: admin, CreatedAt: created, UpdatedAt: updated}
	if err := s.apply(ctx, admin, user, &a, in.Input, true); err != nil {
		return Asset{}, err
	}
	if len(in.Sources) > 0 {
		found, err := s.store.Bookings(ctx, admin, in.Sources, in.Sources)
		if err != nil {
			return Asset{}, wrap("find sources", err)
		}
		for _, b := range found {
			a.Sources = append(a.Sources, Source{ID: s.ids.Next(), DetailID: b.DetailID, EntryID: b.EntryID, CreatedAt: created})
		}
	}
	for _, c := range in.ValueChanges {
		c.ID, c.CreatedAt = s.ids.Next(), created
		a.ValueChanges = append(a.ValueChanges, c)
	}
	if err := s.save(ctx, user, nil, a, true); err != nil {
		return Asset{}, err
	}
	return a, s.record(ctx, admin, a.ID, user, "asset_created", nil)
}
