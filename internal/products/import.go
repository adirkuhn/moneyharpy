package products

import (
	"context"
	"fmt"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Import adds a product brought in from another system, such as Moneybird,
// keeping the id and times it had there. It checks the fields as Create
// does.
func (s *Service) Import(ctx context.Context, admin, user resource.ID, o resource.Origin, c Changes) (Product, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Product{}, err
	}
	var verr resource.ValidationError
	if c.Price == nil {
		verr.Add("price", "can't be blank")
	}
	if c.LedgerAccountID == nil {
		verr.Add("ledger_account_id", "can't be blank")
	}
	created, updated := o.Or(s.now())
	p := Product{ID: o.ID, AdministrationID: admin, Currency: a.Currency, Active: true, CreatedAt: created, UpdatedAt: updated}
	if err := s.apply(ctx, user, &p, c, &verr); err != nil {
		return Product{}, err
	}
	if err := s.store.SaveProduct(ctx, p, true); err != nil {
		return Product{}, fmt.Errorf("store product: %w", err)
	}
	return p, nil
}
