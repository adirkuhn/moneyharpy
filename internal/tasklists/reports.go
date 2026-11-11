package tasklists

import (
	"context"
	"slices"

	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// LedgerAccounts lists an administration's categories.
type LedgerAccounts interface {
	LedgerAccounts(ctx context.Context, admin, user resource.ID) ([]bookkeeping.LedgerAccount, error)
}

// LedgerReports generates tasks from the chart of accounts: a task per
// main category of the balance sheet or of profit and loss, and a subtask
// per sub-category. Debtor, creditor and asset reports have no lines in
// MoneyHarpy yet, so groups that follow them start empty.
type LedgerReports struct{ Accounts LedgerAccounts }

// ReportLines implements Reports.
func (r LedgerReports) ReportLines(ctx context.Context, admin, user resource.ID, reportType string) ([]Line, error) {
	var types []string
	switch reportType {
	case "balance_sheet":
		types = bookkeeping.BalanceSheetTypes
	case "profit_loss":
		types = bookkeeping.ProfitAndLossTypes
	default:
		return []Line{}, nil
	}
	accounts, err := r.Accounts.LedgerAccounts(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	out := []Line{}
	for _, a := range accounts {
		if a.Active && a.ParentID == nil && slices.Contains(types, a.Type) {
			out = append(out, line(a))
		}
	}
	return out, nil
}

// Children implements Reports.
func (r LedgerReports) Children(ctx context.Context, admin, user, category resource.ID) ([]Line, bool, error) {
	accounts, err := r.Accounts.LedgerAccounts(ctx, admin, user)
	if err != nil {
		return nil, false, err
	}
	found := false
	out := []Line{}
	for _, a := range accounts {
		found = found || a.ID == category
		if a.Active && a.ParentID != nil && *a.ParentID == category {
			out = append(out, line(a))
		}
	}
	return out, found, nil
}

func line(a bookkeeping.LedgerAccount) Line {
	id := a.ID
	name := a.Name
	if a.Code != "" {
		name = a.Code + " " + a.Name
	}
	return Line{Name: name, EntityID: &id}
}
