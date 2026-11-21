package purchases

import (
	"context"
	"fmt"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/journal"
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
	// New keeps the document as the source has it before processing it:
	// filled in, but not booked, as Moneybird keeps a new document.
	New bool
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
	if in.New {
		if err := s.keepNew(ctx, a, user, &d, true); err != nil {
			return Document{}, err
		}
		return s.store.Document(ctx, admin, in.Kind, d.ID)
	}
	if err := s.save(ctx, a, user, &d, true); err != nil {
		return Document{}, err
	}
	if err := s.importPayments(ctx, admin, user, in.Kind, d.ID, in.Payments); err != nil {
		return Document{}, err
	}
	return s.store.Document(ctx, admin, in.Kind, d.ID)
}

// ImportPayments brings in the payments the source has on a document that
// came in before that are not here, such as one refused then. It reports
// whether it added any; the payments here stay.
func (s *Service) ImportPayments(ctx context.Context, admin, user resource.ID, kind string, id resource.ID, payments []sales.ImportedPayment) (bool, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return false, err
	}
	d, err := s.store.Document(ctx, admin, kind, id)
	if err != nil {
		return false, err
	}
	missing := sales.MissingPayments(d.Payments, payments)
	if len(missing) == 0 || !d.Booked() {
		return false, nil
	}
	return true, s.importPayments(ctx, admin, user, kind, id, missing)
}

func (s *Service) importPayments(ctx context.Context, admin, user resource.ID, kind string, id resource.ID, payments []sales.ImportedPayment) error {
	for i, p := range payments {
		in := p.Input
		in.Imported = true
		if _, err := s.registerPayment(ctx, admin, user, kind, id, in); err != nil {
			return fmt.Errorf("payment %d: %w", i+1, err)
		}
	}
	return nil
}

// ImportedNew takes back the booking of an imported document the source
// keeps as new, not processed yet, as long as nothing paid it. It reports
// whether the document changed.
func (s *Service) ImportedNew(ctx context.Context, admin, user resource.ID, kind string, id resource.ID) (bool, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return false, err
	}
	d, err := s.store.Document(ctx, admin, kind, id)
	if err != nil {
		return false, err
	}
	if d.State == New || d.State == Paid || len(d.Payments) > 0 {
		return false, nil
	}
	d.UpdatedAt = s.now()
	return true, s.keepNew(ctx, a, user, &d, false)
}

// keepNew stores a document as new with its totals, booking nothing and
// taking back what it booked.
func (s *Service) keepNew(ctx context.Context, a administration.Listed, user resource.ID, d *Document, created bool) error {
	rates, err := s.rates(ctx, a.ID, user, *d)
	if err != nil {
		return err
	}
	d.Totals = computeTotals(*d, rates)
	allocate(d, rates)
	d.State = New
	return wrap("store document", s.store.SaveDocument(ctx, *d, []journal.Entry{}, created))
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
