package audits

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

type admins struct{}

func (admins) Get(_ context.Context, id, _ resource.ID) (administration.Listed, error) {
	return administration.Listed{Administration: administration.Administration{ID: id, TimeZone: "Europe/Amsterdam"}}, nil
}

type store struct {
	sums   []Sums
	audits map[resource.ID]Audit
}

func (s *store) Sums(context.Context, resource.ID, money.Period) ([]Sums, error) { return s.sums, nil }
func (s *store) Audits(_ context.Context, _ resource.ID, p money.Period) ([]Audit, error) {
	var out []Audit
	for _, a := range s.audits {
		if a.Period == p {
			out = append(out, a)
		}
	}
	return out, nil
}
func (s *store) Save(_ context.Context, _ resource.ID, a Audit) error {
	s.audits[a.LedgerAccountID] = a
	return nil
}
func (s *store) HasAccount(_ context.Context, _, ledger resource.ID) (bool, error) {
	return ledger == 1010 || ledger == 8000, nil
}
func (s *store) Delete(_ context.Context, _, ledger resource.ID, _ money.Period) error {
	delete(s.audits, ledger)
	return nil
}

func TestAudit(t *testing.T) {
	st := &store{sums: []Sums{{LedgerAccountID: 1010, Debit: 100_00, TaxRateIDs: []resource.ID{}}, {LedgerAccountID: 8000, Credit: 100_00, TaxRateIDs: []resource.ID{21}}}, audits: map[resource.ID]Audit{}}
	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	svc := NewService(admins{}, st, resource.NewGenerator(1), func() time.Time { return now })
	ctx := context.Background()

	line, err := svc.Mark(ctx, 11, 7, 8000, "202610..202612")
	if err != nil || line.Audit == nil || line.Audit.UserID != 7 || line.Audit.Credit != 100_00 {
		t.Fatalf("Mark() = %+v, %v", line, err)
	}
	r, err := svc.Report(ctx, 11, 7, "202610..202612")
	if err != nil || len(r.Lines) != 2 || r.Lines[0].Audit != nil || r.Lines[1].Audit == nil || r.Lines[1].Changed {
		t.Fatalf("Report() = %+v, %v", r, err)
	}
	// A booking after the audit shows as a change.
	st.sums[1].Credit = 150_00
	r, _ = svc.Report(ctx, 11, 7, "202610..202612")
	if !r.Lines[1].Changed {
		t.Errorf("a change after the audit is not seen: %+v", r.Lines[1])
	}
	// Another period has its own audits.
	if other, _ := svc.Report(ctx, 11, 7, "202607..202609"); other.Lines[1].Audit != nil {
		t.Errorf("the audit counts for another period: %+v", other.Lines[1])
	}
	if err := svc.Unmark(ctx, 11, 7, 8000, "202610..202612"); err != nil {
		t.Fatal(err)
	}
	if r, _ := svc.Report(ctx, 11, 7, "202610..202612"); r.Lines[1].Audit != nil {
		t.Errorf("the audit stays after unmarking: %+v", r.Lines[1])
	}
	if _, err := svc.Mark(ctx, 11, 7, 999, "202610..202612"); !errors.Is(err, ErrNotFound) {
		t.Errorf("marking another administration's category: %v", err)
	}
	if _, err := svc.Report(ctx, 11, 7, "someday"); !errors.Is(err, ErrInvalid) {
		t.Errorf("an unknown period: %v", err)
	}
}
