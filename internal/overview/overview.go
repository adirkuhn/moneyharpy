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
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
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

// Sales is the part of sales invoices the overview reads.
type Sales interface {
	Summary(ctx context.Context, admin, user resource.ID) (sales.Summary, error)
}

// Incoming is the part of incoming documents the overview reads.
type Incoming interface {
	Summary(ctx context.Context, admin, user resource.ID) (purchases.Summary, error)
}

// IncomingBlock sums up the purchase invoices and receipts still to pay
// and the uploads still to process.
type IncomingBlock struct {
	ToPay     purchases.Tally
	ToProcess int
	Currency  string
}

// SalesBlock sums up the sales invoices still to be paid.
type SalesBlock struct {
	ToReceive sales.Tally
	ToRemind  sales.Tally
	Currency  string
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
	Bank *BankBlock
	// Sales is nil for members without the sales invoices permission.
	Sales *SalesBlock
	// Incoming is nil for members without the documents permission.
	Incoming          *IncomingBlock
	RecentlyProcessed []Item
}

// Service implements the overview use case.
type Service struct {
	admins Administrations
	bank   Bank
	sales  Sales
	// incoming may be nil until incoming documents join.
	incoming Incoming
}

// NewService returns a Service.
func NewService(admins Administrations, bank Bank, sales Sales) *Service {
	return &Service{admins: admins, bank: bank, sales: sales}
}

// UseIncoming adds the incoming documents block.
func (s *Service) UseIncoming(i Incoming) { s.incoming = i }

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
	if may(administration.SalesInvoices) {
		sum, err := s.sales.Summary(ctx, admin, user)
		if err != nil {
			return Overview{}, fmt.Errorf("sales summary: %w", err)
		}
		out.Sales = &SalesBlock{ToReceive: sum.ToReceive, ToRemind: sum.ToRemind, Currency: sum.Currency}
	}
	if s.incoming != nil && may(administration.Documents) {
		sum, err := s.incoming.Summary(ctx, admin, user)
		if err != nil {
			return Overview{}, fmt.Errorf("incoming summary: %w", err)
		}
		out.Incoming = &IncomingBlock{ToPay: sum.ToPay, ToProcess: sum.ToProcess, Currency: sum.Currency}
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
