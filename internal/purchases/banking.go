package purchases

import (
	"context"

	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/iban"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Banking is the banking service as documents use it.
type Banking struct{ *banking.Service }

// FinancialAccount implements Bank.
func (b Banking) FinancialAccount(ctx context.Context, admin, account, user resource.ID) (BankAccount, error) {
	a, err := b.Account(ctx, admin, account, user)
	if err != nil {
		return BankAccount{}, err
	}
	return toBankAccount(a), nil
}

// FinancialAccounts implements Bank.
func (b Banking) FinancialAccounts(ctx context.Context, admin, user resource.ID) ([]BankAccount, error) {
	list, err := b.Accounts(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	out := make([]BankAccount, 0, len(list))
	for _, a := range list {
		if a.Active {
			out = append(out, toBankAccount(a))
		}
	}
	return out, nil
}

func toBankAccount(a banking.Account) BankAccount {
	out := BankAccount{ID: a.ID, Name: a.Name, Type: a.Type, LedgerAccountID: a.LedgerAccountID}
	if a.Type == banking.BankAccount {
		out.IBAN = iban.Normalize(a.Identifier)
	}
	return out
}
