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
			// Payments that came in linked with their invoices settle it too.
			if err := r.s.targets.Banking.SettleImported(ctx, r.job.AdministrationID, r.job.UserID, m.ID.id()); err != nil {
				return err
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
	var linked bool
	if p.InvoiceType == vatDocument {
		// A VAT return cannot come in: what paid it is booked where the
		// return would have it.
		linked, err = r.bookVATPayment(ctx, mutation, price)
	} else {
		linked, err = r.s.targets.Banking.LinkImportedPayment(ctx, r.job.AdministrationID, r.job.UserID, mutation, p.InvoiceType, p.InvoiceID.id(), price)
	}
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

// revisitPayments gives an invoice that came in before without its payments
// the ones Moneybird has, such as when one was refused then.
func (r *run) revisitPayments(step, kind, entityType string) {
	if r.revisit == nil {
		r.revisit = map[string]func(context.Context, json.RawMessage) error{}
	}
	r.revisit[kind] = func(ctx context.Context, raw json.RawMessage) error {
		var inv struct {
			ID       mbID        `json:"id"`
			Payments []mbPayment `json:"payments"`
		}
		if err := json.Unmarshal(raw, &inv); err != nil || len(inv.Payments) == 0 {
			return nil
		}
		payments, err := r.payments(ctx, inv.Payments)
		if err != nil {
			return err
		}
		_, err = r.s.targets.Sales.ImportPayments(ctx, r.job.AdministrationID, r.job.UserID, entityType, inv.ID.id(), payments)
		return r.passed(ctx, step, kind, inv.ID.id(), err)
	}
}

// passed notes a record that came in before and could not be put as
// Moneybird has it now, passing it; any other failure stops the import.
func (r *run) passed(ctx context.Context, step, kind string, id resource.ID, err error) error {
	if err == nil || !isRecordProblem(err) {
		return err
	}
	return r.s.store.RecordProblem(ctx, r.job.AdministrationID, Problem{Step: step, Kind: kind, SourceID: id, Message: describe(err), CreatedAt: r.s.now()})
}

// vatDocument is Moneybird's invoice type of a VAT return.
const vatDocument = "VatDocument"

func (r *run) bookVATPayment(ctx context.Context, mutation resource.ID, price money.Amount) (bool, error) {
	ledger, err := r.s.targets.Books.SystemLedgerAccount(ctx, r.job.AdministrationID, r.job.UserID, "vat_paid_or_received")
	if err != nil {
		return false, err
	}
	if price < 0 {
		price = -price
	}
	return r.s.targets.Banking.BookImported(ctx, r.job.AdministrationID, r.job.UserID, mutation, ledger, price)
}
