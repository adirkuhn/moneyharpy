package purchases

import (
	"context"
	"fmt"

	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// ImportedDocument is a purchase invoice or receipt brought in from another
// system, such as Moneybird, as it was there.
type ImportedDocument struct {
	Origin      resource.Origin
	Kind        string
	Changes     Changes
	Details     []sales.ImportedDetail
	EntryNumber int
	Payments    []sales.ImportedPayment
}

// Import adds a purchase invoice or receipt brought in from another
// system, keeping its id and entry number, and books it with its payments
// as the books here would have. It learns nothing about the supplier and
// reads none of its files. Bringing in a document that is here already
// changes nothing.
func (s *Service) Import(ctx context.Context, admin, user resource.ID, in ImportedDocument) (Document, error) {
	if err := checkKind(in.Kind); err != nil {
		return Document{}, err
	}
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Document{}, err
	}
	if d, err := s.store.Document(ctx, admin, in.Kind, in.Origin.ID); err == nil {
		return d, nil
	} else if !IsNotFound(err) {
		return Document{}, err
	}
	d, err := s.newDocument(ctx, a, in.Kind, OriginAPI)
	if err != nil {
		return Document{}, err
	}
	created, updated := in.Origin.Or(s.now())
	d.ID, d.CreatedAt, d.UpdatedAt = in.Origin.ID, created, updated
	if in.EntryNumber > 0 {
		d.EntryNumber = in.EntryNumber
	}
	c := in.Changes
	c.PaymentAccountID = nil
	c.Details = make([]sales.DetailInput, len(in.Details))
	for i, det := range in.Details {
		c.Details[i] = det.Input
	}
	if err := s.apply(ctx, a, user, &d, c); err != nil {
		return Document{}, err
	}
	for i := range d.Lines {
		if i < len(in.Details) && in.Details[i].Origin.ID != 0 {
			d.Lines[i].ID = in.Details[i].Origin.ID
			d.Lines[i].CreatedAt, d.Lines[i].UpdatedAt = in.Details[i].Origin.Or(created)
		}
	}
	if err := s.save(ctx, a, user, &d, true); err != nil {
		return Document{}, err
	}
	for i, p := range in.Payments {
		if _, err := s.RegisterPayment(ctx, admin, user, in.Kind, d.ID, p.Input); err != nil {
			return Document{}, fmt.Errorf("payment %d: %w", i+1, err)
		}
	}
	return s.store.Document(ctx, admin, in.Kind, d.ID)
}

// ImportTypeless adds a document without a type brought in from another
// system, keeping its id and dates.
func (s *Service) ImportTypeless(ctx context.Context, admin, user resource.ID, o resource.Origin, c TypelessChanges) (Typeless, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Typeless{}, err
	}
	if t, err := s.store.TypelessDocument(ctx, admin, o.ID); err == nil {
		return t, nil
	}
	created, updated := o.Or(s.now())
	t := Typeless{ID: o.ID, AdministrationID: admin, Date: s.today(a), Origin: OriginAPI, State: New, CreatedAt: created, UpdatedAt: updated}
	if err := s.applyTypeless(ctx, admin, user, &t, c); err != nil {
		return Typeless{}, err
	}
	return t, wrap("store typeless document", s.store.SaveTypelessDocument(ctx, t, true))
}

// ImportGeneral adds a general document brought in from another system,
// keeping its id, entry number and dates.
func (s *Service) ImportGeneral(ctx context.Context, admin, user resource.ID, o resource.Origin, entryNumber int, c GeneralChanges) (General, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return General{}, err
	}
	if g, err := s.store.GeneralDocument(ctx, admin, o.ID); err == nil {
		return g, nil
	}
	if entryNumber == 0 {
		if entryNumber, err = s.store.NextEntryNumber(ctx, admin); err != nil {
			return General{}, wrap("number document", err)
		}
	}
	created, updated := o.Or(s.now())
	g := General{ID: o.ID, AdministrationID: admin, EntryNumber: entryNumber, Date: s.today(a), State: Saved, CreatedAt: created, UpdatedAt: updated}
	if err := s.applyGeneral(ctx, a, user, &g, c); err != nil {
		return General{}, err
	}
	return g, wrap("store general document", s.store.SaveGeneralDocument(ctx, g, true))
}
