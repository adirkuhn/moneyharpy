package vatreturns

import (
	"context"

	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Bookkeeping is the part of the bookkeeping service returns use.
type Bookkeeping interface {
	Settings(ctx context.Context, admin, user resource.ID) (bookkeeping.Settings, error)
	SystemLedgerAccount(ctx context.Context, admin, user resource.ID, role string) (resource.ID, error)
}

// BookkeepingBooks implements Books with the bookkeeping service.
type BookkeepingBooks struct{ Bookkeeping Bookkeeping }

// Settings implements Books.
func (b BookkeepingBooks) Settings(ctx context.Context, admin, user resource.ID) (Settings, error) {
	st, err := b.Bookkeeping.Settings(ctx, admin, user)
	if err != nil {
		return Settings{}, err
	}
	return Settings{VATPeriod: st.VATPeriod, ICPPeriod: st.ICPPeriod, LockedUntil: st.LockedUntil}, nil
}

// SystemLedgerAccount implements Books.
func (b BookkeepingBooks) SystemLedgerAccount(ctx context.Context, admin, user resource.ID, role string) (resource.ID, error) {
	return b.Bookkeeping.SystemLedgerAccount(ctx, admin, user, role)
}
