package assets

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Roles of the system categories assets book on.
const (
	DepreciationRole = "depreciation"
	BookResultRole   = "book_result"
	ReserveRole      = "reinvestment_reserve"
)

// Administrations gives access to administrations.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Ledger is the part of bookkeeping assets rely on.
type Ledger interface {
	LedgerAccount(ctx context.Context, admin, id, user resource.ID) (bookkeeping.LedgerAccount, error)
	SystemLedgerAccount(ctx context.Context, admin, user resource.ID, role string) (resource.ID, error)
	// AccumulatedDepreciation returns the sub-category of a fixed asset
	// category that holds its accumulated depreciation.
	AccumulatedDepreciation(ctx context.Context, admin, user, parent resource.ID) (resource.ID, error)
	LockedUntil(ctx context.Context, admin, user resource.ID) (string, error)
}

// ErrBookingNotFound reports a purchase line or journal entry row a source
// names that the administration does not have.
var ErrBookingNotFound = errors.New("booking not found")

// Query selects assets.
type Query struct {
	LedgerAccountID *resource.ID
	// OnlyActive leaves out the assets with a disposal.
	OnlyActive    bool
	Page, PerPage int
}

// Workload is an administration with assets the background job may book
// depreciation for, and the member it acts as.
type Workload struct {
	AdministrationID resource.ID
	UserID           resource.ID
}

// Store persists assets with what they book.
type Store interface {
	Assets(ctx context.Context, admin resource.ID, q Query) ([]Asset, int, error)
	// Asset returns ErrNotFound when the administration has no such asset.
	Asset(ctx context.Context, admin, id resource.ID) (Asset, error)
	// Save stores the asset with its value changes and sources, and
	// replaces what it books with entries.
	Save(ctx context.Context, a Asset, created bool, entries []journal.Entry) error
	// Delete removes the asset with what it books.
	Delete(ctx context.Context, admin, id resource.ID) error
	// ReserveBalance is what reinvestment reserve sales added minus what
	// purchases took.
	ReserveBalance(ctx context.Context, admin resource.ID) (money.Amount, error)
	// DetailLedgerAccount returns the category of a purchase invoice or
	// receipt line, nil while it has none, or ErrBookingNotFound.
	DetailLedgerAccount(ctx context.Context, admin, id resource.ID) (*resource.ID, error)
	// EntryLedgerAccount returns the category of a journal entry row, or
	// ErrBookingNotFound.
	EntryLedgerAccount(ctx context.Context, admin, id resource.ID) (resource.ID, error)
	Workload(ctx context.Context) ([]Workload, error)
	// Lock keeps other instances off the administration's assets until
	// release; ok is false when another instance holds it.
	Lock(ctx context.Context, admin resource.ID) (release func(), ok bool, err error)
}

// Notifier tells about value changes MoneyHarpy books or removes by itself,
// such as the monthly depreciation.
type Notifier interface {
	Notify(ctx context.Context, admin resource.ID, action string, a Asset, c ValueChange)
}

// Service implements the asset use cases.
type Service struct {
	admins   Administrations
	ledger   Ledger
	store    Store
	notifier Notifier
	ids      *resource.Generator
	now      func() time.Time
}

// NewService returns a Service. notifier may be nil.
func NewService(admins Administrations, ledger Ledger, store Store, notifier Notifier, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, ledger: ledger, store: store, notifier: notifier, ids: ids, now: now}
}

// Limits of lists.
const (
	DefaultPerPage = 50
	MaxPerPage     = 100
	// MaxLifespanYears bounds a depreciation plan.
	MaxLifespanYears = 100
)

// List returns one page of the administration's assets, by purchase date.
func (s *Service) List(ctx context.Context, admin, user resource.ID, q Query) ([]Asset, int, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, 0, err
	}
	q.Page = max(q.Page, 1)
	if q.PerPage <= 0 {
		q.PerPage = DefaultPerPage
	}
	q.PerPage = min(q.PerPage, MaxPerPage)
	return s.store.Assets(ctx, admin, q)
}

// Asset returns one asset.
func (s *Service) Asset(ctx context.Context, admin, id, user resource.ID) (Asset, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Asset{}, err
	}
	return s.store.Asset(ctx, admin, id)
}

// PlanInput is a depreciation plan as sent; nil fields are not sent.
type PlanInput struct {
	LifespanYears *float64
	ResidualValue *money.Amount
}

// Input holds an asset's fields as sent; nil fields are not sent.
type Input struct {
	Name            *string
	LedgerAccountID *resource.ID
	PurchaseDate    *time.Time
	PurchaseValue   *money.Amount
	Plan            *PlanInput
}

// Create adds an asset. It books nothing: the purchase is booked by the
// documents its sources link.
func (s *Service) Create(ctx context.Context, admin, user resource.ID, in Input) (Asset, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Asset{}, err
	}
	now := s.now()
	a := Asset{ID: s.ids.Next(), AdministrationID: admin, CreatedAt: now, UpdatedAt: now}
	if err := s.apply(ctx, admin, user, &a, in, true); err != nil {
		return Asset{}, err
	}
	if err := s.store.Save(ctx, a, true, nil); err != nil {
		return Asset{}, wrap("store asset", err)
	}
	return a, nil
}

// Update changes an asset. Once it has value changes, a disposal or a
// reinvestment reserve purchase, only its name can change.
func (s *Service) Update(ctx context.Context, admin, id, user resource.ID, in Input) (Asset, error) {
	before, err := s.Asset(ctx, admin, id, user)
	if err != nil {
		return Asset{}, err
	}
	a := before
	if len(a.ValueChanges) > 0 || a.Disposal != nil || a.ReservePurchase != nil {
		if in.LedgerAccountID != nil && *in.LedgerAccountID != a.LedgerAccountID || in.PurchaseDate != nil && !in.PurchaseDate.Equal(a.PurchaseDate) ||
			in.PurchaseValue != nil && *in.PurchaseValue != a.PurchaseValue || in.Plan != nil && planChanges(a.Plan, *in.Plan) {
			return Asset{}, invalid("base", "Cannot update asset when it has value changes")
		}
	}
	if err := s.apply(ctx, admin, user, &a, in, false); err != nil {
		return Asset{}, err
	}
	a.UpdatedAt = s.now()
	return a, s.save(ctx, user, &before, a, false)
}

func planChanges(p *Plan, in PlanInput) bool {
	if p == nil {
		return in.LifespanYears != nil || in.ResidualValue != nil
	}
	return in.LifespanYears != nil && lifespanMonths(*in.LifespanYears) != p.LifespanMonths || in.ResidualValue != nil && *in.ResidualValue != p.ResidualValue
}

func lifespanMonths(years float64) int { return int(math.Round(years * 12)) }

// apply validates the input and sets it on the asset.
func (s *Service) apply(ctx context.Context, admin, user resource.ID, a *Asset, in Input, creating bool) error {
	var verr resource.ValidationError
	if in.Name != nil {
		a.Name = strings.TrimSpace(*in.Name)
	}
	if a.Name == "" {
		verr.Add("name", "can't be blank")
	}
	if in.PurchaseDate != nil {
		a.PurchaseDate = *in.PurchaseDate
	}
	if in.PurchaseValue != nil {
		if *in.PurchaseValue < 0 {
			verr.Add("purchase_value", "must be greater than or equal to 0")
		}
		a.PurchaseValue = *in.PurchaseValue
	}
	if in.LedgerAccountID != nil {
		a.LedgerAccountID = *in.LedgerAccountID
	}
	if in.Plan != nil {
		p := Plan{}
		if a.Plan != nil {
			p = *a.Plan
		}
		if in.Plan.LifespanYears != nil {
			if y := *in.Plan.LifespanYears; y <= 0 || y > MaxLifespanYears || lifespanMonths(y) < 1 {
				verr.Add("value_change_plan.lifespan_in_years", fmt.Sprintf("must be greater than 0 and at most %d", MaxLifespanYears))
			} else {
				p.LifespanMonths = lifespanMonths(y)
			}
		}
		if in.Plan.ResidualValue != nil {
			p.ResidualValue = *in.Plan.ResidualValue
		}
		if p.ResidualValue < 0 {
			verr.Add("value_change_plan.residual_value", "must be greater than or equal to 0")
		}
		if p.LifespanMonths == 0 && creating && in.Plan.LifespanYears == nil {
			verr.Add("value_change_plan.lifespan_in_years", "can't be blank")
		}
		a.Plan = &p
	}
	if a.Plan != nil && a.Plan.ResidualValue > a.PurchaseValue {
		verr.Add("value_change_plan.residual_value", "cannot be more than the purchase value")
	}
	l, err := s.ledger.LedgerAccount(ctx, admin, a.LedgerAccountID, user)
	switch {
	case bookkeeping.IsNotFound(err):
		verr.Add("ledger_account", "is invalid")
	case err != nil:
		return err
	case l.Type != "non_current_assets" || !l.Active || l.System != "":
		verr.Add("ledger_account", "is invalid")
	case l.ParentID != nil:
		verr.Add("ledger_account", "cannot be a sub-category. Depreciation then lands in a different place on the balance sheet than the purchase value. Add a new category via 'Fixed assets'.")
	case landOrBuilding(l.Taxonomy) && a.Plan != nil:
		verr.Add("value_change_plan.base", "Depreciations are not possible with this ledger account")
	case !landOrBuilding(l.Taxonomy) && a.Plan == nil:
		verr.Add("value_change_plan", "can't be blank")
	}
	return verr.OrNil()
}

// Delete removes an asset with what it booked, unless that is in the
// locked period.
func (s *Service) Delete(ctx context.Context, admin, id, user resource.ID) error {
	a, err := s.Asset(ctx, admin, id, user)
	if err != nil {
		return err
	}
	acc, err := s.accounts(ctx, admin, user, a)
	if err != nil {
		return err
	}
	if err := s.checkLocked(ctx, admin, user, a.Entries(acc), nil); err != nil {
		return invalid("base", "Asset cannot be deleted because there are journal entries in the locked period of the administration.")
	}
	return wrap("delete asset", s.store.Delete(ctx, admin, id))
}

// SourceInput names the booking a source links: a purchase invoice or
// receipt line, or a journal entry row.
type SourceInput struct {
	DetailID *resource.ID
	EntryID  *resource.ID
}

// AddSource links a booking on the asset's category to the asset.
func (s *Service) AddSource(ctx context.Context, admin, id, user resource.ID, in SourceInput) (Source, error) {
	a, err := s.Asset(ctx, admin, id, user)
	if err != nil {
		return Source{}, err
	}
	var ledger *resource.ID
	switch {
	case in.DetailID != nil:
		ledger, err = s.store.DetailLedgerAccount(ctx, admin, *in.DetailID)
	case in.EntryID != nil:
		var l resource.ID
		l, err = s.store.EntryLedgerAccount(ctx, admin, *in.EntryID)
		ledger = &l
	default:
		return Source{}, invalid("booking", "can't be blank")
	}
	if errors.Is(err, ErrBookingNotFound) {
		return Source{}, invalid("booking", "does not exist")
	}
	if err != nil {
		return Source{}, err
	}
	if ledger == nil || *ledger != a.LedgerAccountID {
		return Source{}, invalid("booking", "The ledger account of the source must be equal to the ledger account of the asset")
	}
	for _, o := range a.Sources {
		if in.DetailID != nil && o.DetailID != nil && *o.DetailID == *in.DetailID || in.EntryID != nil && o.EntryID != nil && *o.EntryID == *in.EntryID {
			return Source{}, invalid("booking", "has already been taken")
		}
	}
	src := Source{ID: s.ids.Next(), DetailID: in.DetailID, EntryID: in.EntryID, CreatedAt: s.now()}
	before := a
	a.Sources = append(append([]Source(nil), a.Sources...), src)
	a.UpdatedAt = s.now()
	return src, s.save(ctx, user, &before, a, false)
}

// DeleteSource unlinks a source; the booking stays.
func (s *Service) DeleteSource(ctx context.Context, admin, id, source, user resource.ID) error {
	a, err := s.Asset(ctx, admin, id, user)
	if err != nil {
		return err
	}
	before := a
	a.Sources = nil
	found := false
	for _, o := range before.Sources {
		if o.ID == source {
			found = true
			continue
		}
		a.Sources = append(a.Sources, o)
	}
	if !found {
		return ErrSourceNotFound
	}
	a.UpdatedAt = s.now()
	return s.save(ctx, user, &before, a, false)
}

// ChangeInput is a value change as sent.
type ChangeInput struct {
	Date             time.Time
	Amount           money.Amount
	Description      string
	ExternallyBooked bool
}

// AddValueChange adds a manual or arbitrary value change. An arbitrary one
// restarts the plan from its day: the linear value changes after it go.
func (s *Service) AddValueChange(ctx context.Context, admin, id, user resource.ID, kind string, in ChangeInput) (ValueChange, error) {
	a, today, err := s.forChange(ctx, admin, id, user)
	if err != nil {
		return ValueChange{}, err
	}
	var verr resource.ValidationError
	s.checkChangeDay(ctx, admin, user, a, in.Date, today, &verr)
	if strings.TrimSpace(in.Description) == "" {
		verr.Add("description", "can't be blank")
	}
	if err := verr.OrNil(); err != nil {
		return ValueChange{}, err
	}
	before := a
	a.ValueChanges = append([]ValueChange(nil), a.ValueChanges...)
	var removed []ValueChange
	if kind == Arbitrary {
		removed = a.removeLinearAfter(in.Date)
	}
	if in.Amount < 0 && -in.Amount > a.ValueAt(in.Date) {
		return ValueChange{}, invalid("amount", "cannot be larger than the value of the asset on this date")
	}
	description := in.Description
	c := ValueChange{ID: s.ids.Next(), Type: kind, Date: in.Date, Amount: in.Amount, Description: &description, ExternallyBooked: in.ExternallyBooked, CreatedAt: s.now()}
	a.addChange(c)
	a.UpdatedAt = s.now()
	if err := s.save(ctx, user, &before, a, false); err != nil {
		return ValueChange{}, err
	}
	s.notifyAll(ctx, admin, "company_assets_value_changes_linear_destroyed", a, removed)
	return c, nil
}

// Dispose writes the asset off on a day with a divestment (the rest of its
// value to the book result) or a full depreciation (to depreciation
// costs), and disposes of it. Linear value changes after the day go.
func (s *Service) Dispose(ctx context.Context, admin, id, user resource.ID, kind string, day time.Time, description *string) (ValueChange, error) {
	a, today, err := s.forChange(ctx, admin, id, user)
	if err != nil {
		return ValueChange{}, err
	}
	var verr resource.ValidationError
	s.checkChangeDay(ctx, admin, user, a, day, today, &verr)
	if last, ok := a.lastArbitrary(); ok && day.Before(last.Date) {
		verr.Add("date", "must be after the last arbitrary depreciation")
	}
	if kind == FullDepreciation && (description == nil || strings.TrimSpace(*description) == "") {
		verr.Add("description", "can't be blank")
	}
	if err := verr.OrNil(); err != nil {
		return ValueChange{}, err
	}
	before := a
	a.ValueChanges = append([]ValueChange(nil), a.ValueChanges...)
	removed := a.removeLinearAfter(day)
	c := ValueChange{ID: s.ids.Next(), Type: kind, Date: day, Amount: -a.ValueAt(day), Description: description, CreatedAt: s.now()}
	a.addChange(c)
	reason := "out_of_use"
	if kind == Divestment {
		reason = "divested"
	}
	a.Disposal = &Disposal{ID: s.ids.Next(), Date: day, Reason: reason}
	a.UpdatedAt = s.now()
	if err := s.save(ctx, user, &before, a, false); err != nil {
		return ValueChange{}, err
	}
	s.notifyAll(ctx, admin, "company_assets_value_changes_linear_destroyed", a, removed)
	return c, nil
}

// forChange loads an asset that can still change value, with today.
func (s *Service) forChange(ctx context.Context, admin, id, user resource.ID) (Asset, time.Time, error) {
	adm, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Asset{}, time.Time{}, err
	}
	a, err := s.store.Asset(ctx, admin, id)
	if err != nil {
		return Asset{}, time.Time{}, err
	}
	if a.Disposal != nil {
		return Asset{}, time.Time{}, invalid("base", "Asset has a disposal and cannot change value any more")
	}
	return a, s.today(adm), nil
}

// checkChangeDay checks the day of a value change or disposal.
func (s *Service) checkChangeDay(ctx context.Context, admin, user resource.ID, a Asset, day, today time.Time, verr *resource.ValidationError) {
	switch {
	case day.Before(a.PurchaseDate):
		verr.Add("date", "can not be before the commissioning date of the asset")
	case day.After(today):
		verr.Add("date", "cannot be in the future")
	}
	if locked, err := s.lockedUntil(ctx, admin, user); err == nil && locked != nil && !day.After(*locked) {
		verr.Add("date", "is in the locked period, which ends on "+locked.Format(time.DateOnly))
	}
}

// AddDisposal disposes of an asset whose value is zero on the day.
func (s *Service) AddDisposal(ctx context.Context, admin, id, user resource.ID, day time.Time, reason string) (Disposal, error) {
	adm, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Disposal{}, err
	}
	a, err := s.store.Asset(ctx, admin, id)
	if err != nil {
		return Disposal{}, err
	}
	if a.Disposal != nil {
		return Disposal{}, invalid("base", "Asset already has a disposal")
	}
	var verr resource.ValidationError
	s.checkChangeDay(ctx, admin, user, a, day, s.today(adm), &verr)
	if a.ValueAt(day) != 0 {
		verr.Add("date", "is invalid, asset is not fully depreciated on this date")
	}
	if err := verr.OrNil(); err != nil {
		return Disposal{}, err
	}
	before := a
	d := Disposal{ID: s.ids.Next(), Date: day, Reason: reason}
	a.Disposal = &d
	a.UpdatedAt = s.now()
	return d, s.save(ctx, user, &before, a, false)
}

// AddReservePurchase funds part of the purchase value from the
// reinvestment reserve, at most the depreciable value and what the reserve
// holds.
func (s *Service) AddReservePurchase(ctx context.Context, admin, id, user resource.ID, amount money.Amount) (ReserveBooking, error) {
	a, err := s.Asset(ctx, admin, id, user)
	if err != nil {
		return ReserveBooking{}, err
	}
	if a.ReservePurchase != nil {
		return ReserveBooking{}, invalid("base", "Asset already has a reinvestment reserve purchase")
	}
	var verr resource.ValidationError
	residual := money.Amount(0)
	if a.Plan != nil {
		residual = a.Plan.ResidualValue
	}
	if amount > a.PurchaseValue-residual {
		verr.Add("amount", "cannot exceed the purchase value minus the residual value")
	}
	available, err := s.store.ReserveBalance(ctx, admin)
	if err != nil {
		return ReserveBooking{}, wrap("sum up reinvestment reserve", err)
	}
	if amount > available {
		verr.Add("amount", "cannot exceed the available reinvestment reserve")
	}
	if locked, err := s.lockedUntil(ctx, admin, user); err != nil {
		return ReserveBooking{}, err
	} else if locked != nil && !a.PurchaseDate.After(*locked) {
		verr.Add("date", "is in the locked period, which ends on "+locked.Format(time.DateOnly))
	}
	if err := verr.OrNil(); err != nil {
		return ReserveBooking{}, err
	}
	before := a
	r := ReserveBooking{ID: s.ids.Next(), Amount: amount}
	a.ReservePurchase = &r
	a.UpdatedAt = s.now()
	return r, s.save(ctx, user, &before, a, false)
}

// DeleteReservePurchase removes the reinvestment reserve purchase.
func (s *Service) DeleteReservePurchase(ctx context.Context, admin, id, user resource.ID) error {
	a, err := s.Asset(ctx, admin, id, user)
	if err != nil {
		return err
	}
	if a.ReservePurchase == nil {
		return ErrReserveNotFound
	}
	before := a
	a.ReservePurchase = nil
	a.UpdatedAt = s.now()
	return s.save(ctx, user, &before, a, false)
}

// AddReserveSale books (part of) the book result of the asset's disposal
// to the reinvestment reserve.
func (s *Service) AddReserveSale(ctx context.Context, admin, id, user resource.ID, amount money.Amount) (ReserveBooking, error) {
	a, err := s.Asset(ctx, admin, id, user)
	if err != nil {
		return ReserveBooking{}, err
	}
	if a.Disposal == nil {
		return ReserveBooking{}, invalid("base", "Asset has no disposal")
	}
	if a.Disposal.ReserveSale != nil {
		return ReserveBooking{}, invalid("base", "The disposal already has a reinvestment reserve sale")
	}
	if locked, err := s.lockedUntil(ctx, admin, user); err != nil {
		return ReserveBooking{}, err
	} else if locked != nil && !a.Disposal.Date.After(*locked) {
		return ReserveBooking{}, invalid("date", "is in the locked period, which ends on "+locked.Format(time.DateOnly))
	}
	before := a
	r := ReserveBooking{ID: s.ids.Next(), Amount: amount}
	d := *a.Disposal
	d.ReserveSale = &r
	a.Disposal = &d
	a.UpdatedAt = s.now()
	return r, s.save(ctx, user, &before, a, false)
}

// DeleteReserveSale removes the reinvestment reserve sale of the disposal.
func (s *Service) DeleteReserveSale(ctx context.Context, admin, id, user resource.ID) error {
	a, err := s.Asset(ctx, admin, id, user)
	if err != nil {
		return err
	}
	if a.Disposal == nil || a.Disposal.ReserveSale == nil {
		return ErrReserveNotFound
	}
	before := a
	d := *a.Disposal
	d.ReserveSale = nil
	a.Disposal = &d
	a.UpdatedAt = s.now()
	return s.save(ctx, user, &before, a, false)
}

// AddRetroactiveLinear books the plan's linear value changes that are
// missing up to today: from the purchase or the last arbitrary value
// change. None of them may be in the locked period.
func (s *Service) AddRetroactiveLinear(ctx context.Context, admin, id, user resource.ID) error {
	adm, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return err
	}
	a, err := s.store.Asset(ctx, admin, id)
	if err != nil {
		return err
	}
	if a.Plan == nil {
		return invalid("base", "Asset has no value change plan")
	}
	if !a.PlanActive() {
		return invalid("base", "The value change plan of the asset is not active")
	}
	missing := a.Missing(s.today(adm))
	if len(missing) == 0 {
		return invalid("base", "The asset has no missing value changes")
	}
	locked, err := s.lockedUntil(ctx, admin, user)
	if err != nil {
		return err
	}
	if locked != nil && !missing[0].After(*locked) {
		return invalid("base", "Missing value changes are in the locked period, which ends on "+locked.Format(time.DateOnly))
	}
	return s.bookLinear(ctx, admin, user, a, missing)
}

// bookLinear adds the linear value changes of the month ends, in order.
func (s *Service) bookLinear(ctx context.Context, admin, user resource.ID, a Asset, days []time.Time) error {
	before := a
	a.ValueChanges = append([]ValueChange(nil), a.ValueChanges...)
	var added []ValueChange
	for _, d := range days {
		amount, ok := a.linearOn(d)
		if !ok {
			break
		}
		description := "Linear depreciation"
		c := ValueChange{ID: s.ids.Next(), Type: Linear, Date: d, Amount: amount, Description: &description, CreatedAt: s.now()}
		a.addChange(c)
		added = append(added, c)
	}
	if len(added) == 0 {
		return nil
	}
	a.UpdatedAt = s.now()
	if err := s.save(ctx, user, &before, a, false); err != nil {
		return err
	}
	s.notifyAll(ctx, admin, "company_assets_value_changes_linear_created", a, added)
	return nil
}

func (s *Service) notifyAll(ctx context.Context, admin resource.ID, action string, a Asset, list []ValueChange) {
	if s.notifier == nil {
		return
	}
	for _, c := range list {
		s.notifier.Notify(ctx, admin, action, a, c)
	}
}

// save stores the asset and what it books, refusing changes to bookings in
// the locked period.
func (s *Service) save(ctx context.Context, user resource.ID, before *Asset, a Asset, created bool) error {
	both := []Asset{a}
	if before != nil {
		both = append(both, *before)
	}
	acc, err := s.accounts(ctx, a.AdministrationID, user, both...)
	if err != nil {
		return err
	}
	entries := a.Entries(acc)
	if before != nil {
		if err := s.checkLocked(ctx, a.AdministrationID, user, before.Entries(acc), entries); err != nil {
			return err
		}
	}
	return wrap("store asset", s.store.Save(ctx, a, created, entries))
}

// accounts finds the categories the assets (an asset before and after a
// change) book on.
func (s *Service) accounts(ctx context.Context, admin, user resource.ID, list ...Asset) (Accounts, error) {
	var acc Accounts
	var n Needs
	for _, a := range list {
		m := a.needs()
		n.Depreciation, n.BookResult, n.Reserve = n.Depreciation || m.Depreciation, n.BookResult || m.BookResult, n.Reserve || m.Reserve
	}
	a := list[0]
	var err error
	if n.Depreciation {
		if acc.Depreciation, err = s.ledger.SystemLedgerAccount(ctx, admin, user, DepreciationRole); err != nil {
			return acc, wrap("find depreciation category", err)
		}
		if acc.Accumulated, err = s.ledger.AccumulatedDepreciation(ctx, admin, user, a.LedgerAccountID); err != nil {
			return acc, wrap("find accumulated depreciation category", err)
		}
	}
	if n.BookResult {
		if acc.BookResult, err = s.ledger.SystemLedgerAccount(ctx, admin, user, BookResultRole); err != nil {
			return acc, wrap("find book result category", err)
		}
	}
	if n.Reserve {
		if acc.Reserve, err = s.ledger.SystemLedgerAccount(ctx, admin, user, ReserveRole); err != nil {
			return acc, wrap("find reinvestment reserve category", err)
		}
	}
	return acc, nil
}

// checkLocked refuses a change to what is booked in the locked period.
func (s *Service) checkLocked(ctx context.Context, admin, user resource.ID, before, after []journal.Entry) error {
	locked, err := s.lockedUntil(ctx, admin, user)
	if err != nil || locked == nil {
		return err
	}
	in := func(list []journal.Entry) map[resource.ID]journal.Entry {
		m := map[resource.ID]journal.Entry{}
		for _, e := range list {
			if !e.Date.After(*locked) {
				e.RowOrder, e.Description = 0, ""
				m[e.ID] = e
			}
		}
		return m
	}
	b, af := in(before), in(after)
	changed := len(b) != len(af)
	for id, e := range b {
		if o, ok := af[id]; !ok || o != e {
			changed = true
		}
	}
	if changed {
		return invalid("base", "This changes bookings in the locked period, which ends on "+locked.Format(time.DateOnly))
	}
	return nil
}

func (s *Service) lockedUntil(ctx context.Context, admin, user resource.ID) (*time.Time, error) {
	until, err := s.ledger.LockedUntil(ctx, admin, user)
	if err != nil || until == "" {
		return nil, err
	}
	t, err := time.Parse(time.DateOnly, until)
	if err != nil {
		return nil, wrap("read locked period", err)
	}
	return &t, nil
}

// today is the date in the administration's time zone.
func (s *Service) today(a administration.Listed) time.Time {
	now := s.now()
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		now = now.In(loc)
	}
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func invalid(field, problem string) error {
	var verr resource.ValidationError
	verr.Add(field, problem)
	return &verr
}

func wrap(what string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", what, err)
}
