// Package assets keeps an administration's fixed assets (vaste activa),
// after Moneybird's assets: what was bought on a fixed asset category, the
// bookings it was bought with (sources), its depreciation plan and the value
// changes that follow it, its disposal, and the reinvestment reserve
// ("herinvesteringsreserve") it may draw from or add to.
//
// What an asset books follows from its state alone (see Entries): value
// changes on the category's accumulated depreciation against depreciation
// costs (or the book result for a divestment), a disposal clearing the
// accumulated depreciation against the asset's category, and reinvestment
// reserve bookings against the reserve.
package assets

import (
	"errors"
	"math"
	"slices"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Errors returned by Service.
var (
	ErrNotFound = errors.New("asset not found")
	// ErrSourceNotFound reports a source the asset does not have.
	ErrSourceNotFound = errors.New("asset source not found")
	// ErrReserveNotFound reports a reinvestment reserve purchase or sale the
	// asset does not have.
	ErrReserveNotFound = errors.New("reinvestment reserve booking not found")
)

// IsNotFound reports whether err means an asset resource is missing.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, ErrSourceNotFound) || errors.Is(err, ErrReserveNotFound)
}

// Types of value changes, with Moneybird's names.
const (
	Linear           = "linear"
	Manual           = "manual"
	Arbitrary        = "arbitrary"
	Divestment       = "divestment"
	FullDepreciation = "full_depreciation"
)

// Reasons of disposals, with Moneybird's names.
var Reasons = []string{"out_of_use", "sold", "private_withdrawal", "divested"}

// DocumentType names what assets book in the journal.
const DocumentType = "Asset"

// Plan is an asset's depreciation plan: linear, over its lifespan, down to
// its residual value.
type Plan struct {
	LifespanMonths int
	ResidualValue  money.Amount
}

// LifespanYears is the lifespan in years, as Moneybird shows it.
func (p Plan) LifespanYears() float64 { return float64(p.LifespanMonths) / 12 }

// ValueChange changes an asset's value on a day; negative amounts lower it.
type ValueChange struct {
	ID          resource.ID
	Type        string
	Date        time.Time
	Amount      money.Amount
	Description *string
	// ExternallyBooked changes are booked elsewhere; MoneyHarpy books none
	// of them.
	ExternallyBooked bool
	CreatedAt        time.Time
}

// ReserveBooking is a reinvestment reserve purchase or sale.
type ReserveBooking struct {
	ID     resource.ID
	Amount money.Amount
}

// Disposal takes an asset out of use.
type Disposal struct {
	ID               resource.ID
	Date             time.Time
	Reason           string
	ExternallyBooked bool
	// ReserveSale books (part of) the book result of the disposal to the
	// reinvestment reserve.
	ReserveSale *ReserveBooking
}

// Source is a booking the asset was bought with: a purchase invoice or
// receipt line, or a row of a journal entry.
type Source struct {
	ID        resource.ID
	DetailID  *resource.ID
	EntryID   *resource.ID
	CreatedAt time.Time
}

// Asset is a fixed asset.
type Asset struct {
	ID               resource.ID
	AdministrationID resource.ID
	LedgerAccountID  resource.ID
	Name             string
	PurchaseDate     time.Time
	PurchaseValue    money.Amount
	// Plan is nil for assets that do not depreciate, such as land.
	Plan         *Plan
	ValueChanges []ValueChange
	Disposal     *Disposal
	Sources      []Source
	// ReservePurchase funds part of the purchase value from the
	// reinvestment reserve; it is dated on the purchase date.
	ReservePurchase *ReserveBooking
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Ref locates the asset.
func (a Asset) Ref() resource.Ref {
	return resource.Ref{ID: a.ID, Type: resource.TypeAsset, AdministrationID: a.AdministrationID}
}

// CalculatedPurchaseValue is the purchase value less the reinvestment
// reserve purchase.
func (a Asset) CalculatedPurchaseValue() money.Amount {
	v := a.PurchaseValue
	if a.ReservePurchase != nil {
		v -= a.ReservePurchase.Amount
	}
	return v
}

// ValueAt is the asset's value at the end of day: the calculated purchase
// value with the value changes up to and including that day.
func (a Asset) ValueAt(day time.Time) money.Amount {
	v := a.CalculatedPurchaseValue()
	for _, c := range a.ValueChanges {
		if !c.Date.After(day) {
			v += c.Amount
		}
	}
	return v
}

// CurrentValue is the value with all value changes.
func (a Asset) CurrentValue() money.Amount {
	v := a.CalculatedPurchaseValue()
	for _, c := range a.ValueChanges {
		v += c.Amount
	}
	return v
}

// PlanActive reports whether the plan still depreciates: the asset is not
// disposed of and is worth more than its residual value.
func (a Asset) PlanActive() bool {
	return a.Plan != nil && a.Disposal == nil && a.CurrentValue() > a.Plan.ResidualValue
}

// lastArbitrary returns the latest arbitrary value change, which the plan
// restarts from.
func (a Asset) lastArbitrary() (ValueChange, bool) {
	var last ValueChange
	found := false
	for _, c := range a.ValueChanges {
		if c.Type == Arbitrary && (!found || !c.Date.Before(last.Date)) {
			last, found = c, true
		}
	}
	return last, found
}

// basis is where the plan depreciates from: the purchase, or the last
// arbitrary value change, with the value then and the months left.
func (a Asset) basis() (from time.Time, value money.Amount, months int, restarted bool) {
	if a.Plan == nil {
		return a.PurchaseDate, 0, 0, false
	}
	if c, ok := a.lastArbitrary(); ok {
		return c.Date, a.ValueAt(c.Date), a.Plan.LifespanMonths - monthsBetween(a.PurchaseDate, c.Date), true
	}
	return a.PurchaseDate, a.CalculatedPurchaseValue(), a.Plan.LifespanMonths, false
}

// MonthlyDepreciation is what the plan depreciates each month: what is left
// above the residual value spread over the months left.
func (a Asset) MonthlyDepreciation() money.Amount {
	_, value, months, _ := a.basis()
	if months <= 0 {
		return 0
	}
	left := value - a.Plan.ResidualValue
	if left <= 0 {
		return 0
	}
	return money.Amount(math.Round(float64(left) / float64(months)))
}

// Schedule returns the month ends the plan depreciates on, up to and
// including until: the end of each month from the purchase (or the last
// arbitrary value change, after its day) for the months left.
func (a Asset) Schedule(until time.Time) []time.Time {
	from, _, months, restarted := a.basis()
	var out []time.Time
	for i := range months {
		d := monthEnd(from, i)
		if d.After(until) {
			break
		}
		if restarted && !d.After(from) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// lastScheduled is the plan's final month end.
func (a Asset) lastScheduled() time.Time {
	from, _, months, _ := a.basis()
	return monthEnd(from, months-1)
}

// hasLinearOn reports whether a linear value change is booked on day.
func (a Asset) hasLinearOn(day time.Time) bool {
	return slices.ContainsFunc(a.ValueChanges, func(c ValueChange) bool { return c.Type == Linear && c.Date.Equal(day) })
}

// Missing returns the plan's month ends up to until without their linear
// value change.
func (a Asset) Missing(until time.Time) []time.Time {
	if !a.PlanActive() {
		return nil
	}
	var out []time.Time
	for _, d := range a.Schedule(until) {
		if !a.hasLinearOn(d) {
			out = append(out, d)
		}
	}
	return out
}

// linearOn is the linear value change of the month ending on day: the
// monthly depreciation, the rest down to the residual value in the plan's
// last month, and never below the residual value. ok is false when nothing
// is left to depreciate.
func (a Asset) linearOn(day time.Time) (money.Amount, bool) {
	left := a.ValueAt(day) - a.Plan.ResidualValue
	if left <= 0 {
		return 0, false
	}
	amount := min(a.MonthlyDepreciation(), left)
	if day.Equal(a.lastScheduled()) {
		amount = left
	}
	if amount <= 0 {
		return 0, false
	}
	return -amount, true
}

// addChange adds a value change, keeping them by date.
func (a *Asset) addChange(c ValueChange) {
	a.ValueChanges = append(a.ValueChanges, c)
	slices.SortStableFunc(a.ValueChanges, func(x, y ValueChange) int { return x.Date.Compare(y.Date) })
}

// removeLinearAfter removes the linear value changes after day and
// returns them.
func (a *Asset) removeLinearAfter(day time.Time) []ValueChange {
	var kept, removed []ValueChange
	for _, c := range a.ValueChanges {
		if c.Type == Linear && c.Date.After(day) {
			removed = append(removed, c)
			continue
		}
		kept = append(kept, c)
	}
	a.ValueChanges = kept
	return removed
}

// monthsBetween counts the calendar months from a's month to b's.
func monthsBetween(a, b time.Time) int {
	return (b.Year()-a.Year())*12 + int(b.Month()) - int(a.Month())
}

// monthEnd is the last day of the month add months after t's.
func monthEnd(t time.Time, add int) time.Time {
	return time.Date(t.Year(), t.Month()+time.Month(add)+1, 0, 0, 0, 0, 0, time.UTC)
}

// landOrBuilding reports whether a chart scheme code is land or buildings,
// which do not depreciate.
func landOrBuilding(code string) bool {
	return len(code) >= 7 && (code[:7] == "BMvaTer" || code[:7] == "BMvaBeg")
}
