package sales

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func (m *memStore) Estimates(_ context.Context, _ resource.ID, q EstimateQuery) ([]Estimate, int, error) {
	var out []Estimate
	for _, e := range m.estimates {
		if q.Matches(e) {
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b Estimate) int { return int(b.ID - a.ID) })
	total := len(out)
	if q.PerPage > 0 {
		start := min((max(q.Page, 1)-1)*q.PerPage, len(out))
		out = out[start:min(start+q.PerPage, len(out))]
	}
	return out, total, nil
}

func (m *memStore) Estimate(_ context.Context, _, id resource.ID) (Estimate, error) {
	e, ok := m.estimates[id]
	if !ok {
		return Estimate{}, ErrEstimateNotFound
	}
	return e, nil
}

func (m *memStore) EstimatesByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]Estimate, error) {
	var out []Estimate
	for _, id := range ids {
		if e, err := m.Estimate(ctx, admin, id); err == nil {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *memStore) EstimateByNumber(_ context.Context, _ resource.ID, number string) (Estimate, error) {
	for _, e := range m.estimates {
		if e.EstimateID == number {
			return e, nil
		}
	}
	return Estimate{}, ErrEstimateNotFound
}

func (m *memStore) NextEstimateDraftID(context.Context, resource.ID) (int, error) {
	highest := 0
	for _, e := range m.estimates {
		highest = max(highest, e.DraftID)
	}
	return highest + 1, nil
}

func (m *memStore) ReserveEstimateNumber(context.Context, resource.ID) (string, error) {
	m.estimateSequence++
	return fmt.Sprintf("%04d", m.estimateSequence), nil
}

func (m *memStore) EstimateNumberTaken(_ context.Context, _ resource.ID, number string, except resource.ID) (bool, error) {
	for _, e := range m.estimates {
		if e.EstimateID == number && e.ID != except {
			return true, nil
		}
	}
	return false, nil
}

func (m *memStore) SaveEstimate(_ context.Context, e Estimate, _ bool) error {
	m.estimates[e.ID] = e
	return nil
}

func (m *memStore) DeleteEstimate(_ context.Context, _, id resource.ID) error {
	delete(m.estimates, id)
	return nil
}

func (m *memStore) TouchEstimate(_ context.Context, _, id resource.ID, at time.Time) error {
	e := m.estimates[id]
	e.UpdatedAt = at
	m.estimates[id] = e
	return nil
}

func (m *memStore) SaveBilled(_ context.Context, inv Invoice, e Estimate) error {
	m.invoices[inv.ID] = inv
	m.estimates[e.ID] = e
	return nil
}
