package mbimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// paymentLinks links the invoice payments Moneybird's transactions made to
// those transactions here. An invoice that came in before its transaction
// came in paid, without the link; Moneybird has it on the transaction, so
// that is where it is read. A payment that is not here is noted on the
// transaction, until it is.
func (r *run) paymentLinks(ctx context.Context) error {
	st := r.job.Step(StepPaymentLinks)
	versions, err := r.read.SyncIDs(ctx, r.job.SourceID, "financial_mutations", everything)
	if err != nil {
		return err
	}
	var todo []string
	for _, v := range versions {
		id, err := resource.ParseID(v.ID)
		if err != nil {
			continue
		}
		if _, done, err := r.s.store.ImportedAt(ctx, r.job.AdministrationID, kindFinancialMutation, id); err != nil {
			return err
		} else if done {
			todo = append(todo, v.ID)
		}
	}
	for start := 0; start < len(todo); start += batch {
		raws, err := r.read.FetchByIDs(ctx, r.job.SourceID, "financial_mutations", todo[start:min(start+batch, len(todo))])
		if err != nil {
			return err
		}
		for _, raw := range raws {
			var m mbMutation
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			for _, p := range m.Payments {
				if err := r.linkPayment(ctx, st, m, p); err != nil {
					return err
				}
			}
		}
		if err := r.save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *run) linkPayment(ctx context.Context, st *Step, m mbMutation, p mbMutationPayment) error {
	st.Total++
	mutation := m.ID.id()
	missing := fmt.Sprintf("its payment of %s %d is not here", p.InvoiceType, p.InvoiceID.id())
	price, err := money.ParseAmount(string(p.Price))
	if err != nil {
		return r.fail(ctx, st, kindFinancialMutation, mutation, fmt.Errorf("payment of %s %d: price: is not an amount: %q", p.InvoiceType, p.InvoiceID.id(), string(p.Price)))
	}
	linked, err := r.s.targets.Banking.LinkImportedPayment(ctx, r.job.AdministrationID, r.job.UserID, mutation, p.InvoiceType, p.InvoiceID.id(), price)
	switch {
	case errors.Is(err, banking.ErrBookingNotFound):
		return r.fail(ctx, st, kindFinancialMutation, mutation, errors.New(missing))
	case err != nil && isRecordProblem(err):
		return r.fail(ctx, st, kindFinancialMutation, mutation, err)
	case err != nil:
		return err
	case linked:
		st.Imported++
	default:
		st.Skipped++
	}
	return r.s.store.ForgetProblem(ctx, r.job.AdministrationID, kindFinancialMutation, mutation, missing)
}
