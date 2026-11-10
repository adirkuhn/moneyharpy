// Package overview gathers what an administration's home page shows: the
// balances and work waiting in each part of the books, and what was
// processed last. Each part is shown only to members allowed to see it.
package overview

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// RecentLimit is how many recently processed records the overview lists.
const RecentLimit = 25

// FinancialMutation is the type of a processed bank transaction.
const FinancialMutation = "financial_mutation"

// Administrations gives access to administrations; Get returns
// administration.ErrNotFound when the user has no access.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Bank is the part of banking the overview reads.
type Bank interface {
	Summary(ctx context.Context, admin, user resource.ID, recent int) (banking.Summary, error)
}

// BankBlock sums up the financial accounts.
type BankBlock struct {
	Balance     banking.Amount
	Currency    string
	Unprocessed int
}

// Item is one recently processed record.
type Item struct {
	Type        string
	ID          resource.ID
	Ref         resource.Ref
	Title       string
	Description string
	Amount      string
	Currency    string
	Date        time.Time
	State       string
	ProcessedAt time.Time
}

// Overview is an administration's home page.
type Overview struct {
	// Bank is nil for members without the bank permission.
	Bank              *BankBlock
	RecentlyProcessed []Item
}

// Service implements the overview use case.
type Service struct {
	admins Administrations
	bank   Bank
}

// NewService returns a Service.
func NewService(admins Administrations, bank Bank) *Service {
	return &Service{admins: admins, bank: bank}
}

// Get returns the overview as user may see it. A non-nil scopes further
// limits it to what a connection token was granted.
func (s *Service) Get(ctx context.Context, admin, user resource.ID, scopes []administration.Permission) (Overview, error) {
	l, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Overview{}, err
	}
	may := func(p administration.Permission) bool {
		return l.Can(p) && (scopes == nil || slices.Contains(scopes, p))
	}
	out := Overview{RecentlyProcessed: []Item{}}
	if may(administration.Bank) {
		sum, err := s.bank.Summary(ctx, admin, user, RecentLimit)
		if err != nil {
			return Overview{}, fmt.Errorf("bank summary: %w", err)
		}
		out.Bank = &BankBlock{Balance: sum.Balance, Currency: sum.Currency, Unprocessed: sum.Unprocessed}
		for _, m := range sum.RecentlyProcessed {
			out.RecentlyProcessed = append(out.RecentlyProcessed, mutationItem(m))
		}
	}
	// Each source lists newest first; merging keeps that order on ties.
	slices.SortStableFunc(out.RecentlyProcessed, func(a, b Item) int { return b.ProcessedAt.Compare(a.ProcessedAt) })
	return out, nil
}

func mutationItem(m banking.Mutation) Item {
	message := strings.Join(strings.Fields(m.Message), " ")
	title, description := m.ContraName, message
	if title == "" {
		title, _, _ = strings.Cut(strings.TrimSpace(m.Message), "\n")
		title = strings.TrimSpace(title)
	}
	it := Item{
		Type: FinancialMutation, ID: m.ID, Ref: m.Ref(), Title: title, Description: description,
		Amount: m.Amount.String(), Currency: m.Currency, Date: m.Date, State: m.State,
	}
	if m.ProcessedAt != nil {
		it.ProcessedAt = *m.ProcessedAt
	}
	return it
}
