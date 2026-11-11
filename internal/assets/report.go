package assets

import (
	"context"
	"errors"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Errors about the report period.
var (
	ErrPeriod     = errors.New("period is invalid")
	ErrLongPeriod = errors.New("period cannot exceed 12 months")
)

// secondsPerYear is the average Gregorian year Moneybird counts lifespans
// in.
const secondsPerYear = 31556952

// ReportLine is one asset in the assets report ("staat van vaste activa")
// over a period.
type ReportLine struct {
	Asset Asset
	// LifespanSeconds is nil for an asset without a plan.
	LifespanSeconds *int64
	ResidualValue   money.Amount
	// TotalSincePurchase adds up the value changes up to the period's end.
	TotalSincePurchase money.Amount
	ValueAtBegin       money.Amount
	// Investment is the calculated purchase value of an asset bought in the
	// period.
	Investment money.Amount
	// Depreciation and Divestment split ValueChanges, the value changes in
	// the period.
	Depreciation money.Amount
	Divestment   money.Amount
	ValueChanges money.Amount
	ValueAtEnd   money.Amount
	// NotInUse is the value of an asset bought after the period.
	NotInUse money.Amount
}

// Report returns the assets report over a period of whole months, at most a
// year; this year when none is given. Assets disposed of before the period
// are left out.
func (s *Service) Report(ctx context.Context, admin, user resource.ID, period string) ([]ReportLine, error) {
	adm, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	if period == "" {
		period = "this_year"
	}
	p, err := money.ParseReportPeriod(period, s.today(adm))
	if err != nil {
		return nil, ErrPeriod
	}
	if p.Months() > 12 {
		return nil, ErrLongPeriod
	}
	list, _, err := s.store.Assets(ctx, admin, Query{})
	if err != nil {
		return nil, wrap("list assets", err)
	}
	out := []ReportLine{}
	for _, a := range list {
		if a.Disposal != nil && a.Disposal.Date.Before(p.From) {
			continue
		}
		out = append(out, reportLine(a, p))
	}
	return out, nil
}

func reportLine(a Asset, p money.Period) ReportLine {
	l := ReportLine{Asset: a}
	if a.Plan != nil {
		seconds := int64(a.Plan.LifespanMonths) * secondsPerYear / 12
		l.LifespanSeconds, l.ResidualValue = &seconds, a.Plan.ResidualValue
	}
	if a.PurchaseDate.After(p.To) {
		l.NotInUse = a.CalculatedPurchaseValue()
		return l
	}
	if a.PurchaseDate.Before(p.From) {
		l.ValueAtBegin = a.ValueAt(p.From.Add(-24 * time.Hour))
	} else {
		l.Investment = a.CalculatedPurchaseValue()
	}
	for _, c := range a.ValueChanges {
		if c.Date.After(p.To) {
			continue
		}
		l.TotalSincePurchase += c.Amount
		if c.Date.Before(p.From) {
			continue
		}
		if c.Type == Divestment {
			l.Divestment += c.Amount
		} else {
			l.Depreciation += c.Amount
		}
		l.ValueChanges += c.Amount
	}
	l.ValueAtEnd = l.ValueAtBegin + l.Investment + l.ValueChanges
	return l
}
