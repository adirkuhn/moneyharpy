package reports

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// SubscriptionFigures are the subscription figures of a month.
type SubscriptionFigures struct {
	// MonthlyRecurringRevenue is what the subscriptions running at the end
	// of the month bill per month, excluding VAT.
	MonthlyRecurringRevenue money.Amount
	// AnnualRunRate is twelve times that.
	AnnualRunRate money.Amount
	// ChurnRate is the percentage of the subscriptions running at the start
	// of the month that ended in it, as "12.5"; "0" for none.
	ChurnRate string
	// LifetimeValue is the monthly revenue per customer divided by the
	// churn rate; "0" without churn.
	LifetimeValue string
}

var monthsPer = map[string]float64{"month": 1, "quarter": 3, "year": 12}
var daysPer = map[string]float64{"day": 1, "week": 7}

// monthly is what an item bills per month.
func (it SubscriptionItem) monthly() float64 {
	price := float64(it.Price) * float64(it.Quantity) / 1000 * float64(10000-it.Discount) / 10000
	every := float64(max(it.Every, 1))
	if m, ok := monthsPer[it.Unit]; ok {
		return price / (m * every)
	}
	if d, ok := daysPer[it.Unit]; ok {
		return price * 365.25 / 12 / (d * every)
	}
	return 0
}

// runs reports whether the subscription runs on day.
func (it SubscriptionItem) runs(day time.Time) bool {
	end := it.End
	if end == nil && it.CancelledAt != nil {
		end = ptr(time.Date(it.CancelledAt.Year(), it.CancelledAt.Month(), it.CancelledAt.Day(), 0, 0, 0, 0, time.UTC))
	}
	return !it.Start.After(day) && (end == nil || !end.Before(day))
}

// itemRuns reports whether the product runs on day.
func (it SubscriptionItem) itemRuns(day time.Time) bool {
	return !it.ItemStart.After(day) && (it.ItemEnd == nil || !it.ItemEnd.Before(day))
}

// Subscriptions reports the subscription figures of the last month of the
// period.
func (s *Service) Subscriptions(ctx context.Context, admin, user resource.ID, period string) (SubscriptionFigures, error) {
	w, err := s.open(ctx, admin, user, period, "")
	if err != nil {
		return SubscriptionFigures{}, err
	}
	items, err := s.store.Subscriptions(ctx, admin)
	if err != nil {
		return SubscriptionFigures{}, fmt.Errorf("list subscriptions: %w", err)
	}
	end := w.period.To
	start := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
	before := dayBefore(start)
	var mrr float64
	running, ended := map[resource.ID]bool{}, map[resource.ID]bool{}
	customers := map[resource.ID]bool{}
	for _, it := range items {
		if it.runs(before) {
			running[it.SubscriptionID] = true
			if !it.runs(end) {
				ended[it.SubscriptionID] = true
			}
		}
		if it.runs(end) && it.itemRuns(end) {
			mrr += it.monthly()
			customers[it.ContactID] = true
		}
	}
	out := SubscriptionFigures{MonthlyRecurringRevenue: money.Amount(math.Round(mrr)), ChurnRate: "0", LifetimeValue: "0"}
	out.AnnualRunRate = out.MonthlyRecurringRevenue * 12
	if len(running) > 0 && len(ended) > 0 {
		churn := float64(len(ended)) / float64(len(running))
		out.ChurnRate = strconv.FormatFloat(math.Round(churn*10000)/100, 'f', -1, 64)
		if len(customers) > 0 {
			perCustomer := float64(out.MonthlyRecurringRevenue) / float64(len(customers))
			out.LifetimeValue = money.Amount(math.Round(perCustomer / churn)).String()
		}
	}
	return out, nil
}
