package mbimport

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// everything lifts Moneybird's default filter of this year: any filter
// replaces it, and without a period the dates are not limited.
var everything = url.Values{"filter": {"state:all"}}

func (r *run) financialAccounts(ctx context.Context) error {
	st := r.job.Step(StepFinancialAccounts)
	raws, err := r.read.List(ctx, r.job.SourceID, "financial_accounts", nil)
	if err != nil {
		return err
	}
	st.Total = len(raws)
	// The administration's own cash account makes way for Moneybird's
	// first one, rather than sit beside it under the same name.
	var firstCash mbID
	for _, raw := range raws {
		var a mbFinancialAccount
		if json.Unmarshal(raw, &a) == nil && a.Type == banking.CashAccount && (firstCash == 0 || a.ID < firstCash) {
			firstCash = a.ID
		}
	}
	return r.take(ctx, st, kindFinancialAccount, raws, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var a mbFinancialAccount
		if err := json.Unmarshal(raw, &a); err != nil {
			return 0, err
		}
		ledger, err := r.mapped(ctx, kindAccountLedger, ptrTo(a.ID.id()))
		if err != nil {
			return 0, err
		}
		in := banking.ImportedAccount{Origin: origin(a.ID, a.stamps), Changes: a.changes(), LedgerAccountID: ledger, TakeOverCash: a.ID == firstCash}
		if strings.TrimSpace(deref(in.Changes.Name)) == "" {
			// Moneybird's own accounts may have no name; one here needs one.
			name := strings.TrimSpace(*in.Changes.Type + " " + deref(a.Identifier))
			in.Changes.Name = &name
		}
		acc, err := r.s.targets.Banking.ImportAccount(ctx, r.job.AdministrationID, r.job.UserID, in)
		for n := 2; err != nil && optionalOnly(err, []string{"name"}) != nil && n < 10; n++ {
			// A name taken here takes the account's number, else a number of its own.
			base := deref(a.changes().Name)
			if base == "" {
				base = *in.Changes.Type
			}
			name := fmt.Sprintf("%s (%d)", base, n)
			if n == 2 && deref(a.Identifier) != "" {
				name = fmt.Sprintf("%s (%s)", base, *a.Identifier)
			}
			in.Changes.Name = &name
			acc, err = r.s.targets.Banking.ImportAccount(ctx, r.job.AdministrationID, r.job.UserID, in)
		}
		if err != nil {
			return 0, err
		}
		// A cash account that took the place of ours keeps our id.
		if acc.ID != a.ID.id() {
			if err := r.s.store.RecordImported(ctx, r.job.AdministrationID, kindFinancialAccount, a.ID.id(), acc.ID); err != nil {
				return 0, err
			}
		}
		return a.ID.id(), nil
	})
}

// financialMutations brings in the bank transactions. Moneybird's API has no
// list of statements, so the transactions are grouped by the statement they
// name, and each statement comes in with them.
func (r *run) financialMutations(ctx context.Context) error {
	st := r.job.Step(StepFinancialMutations)
	versions, err := r.read.SyncIDs(ctx, r.job.SourceID, "financial_mutations", everything)
	if err != nil {
		return err
	}
	st.Total = len(versions)
	var todo []string
	for _, v := range versions {
		id, err := resource.ParseID(v.ID)
		if err != nil {
			if err := r.fail(ctx, st, kindFinancialMutation, 0, fmt.Errorf("id %s: %w", v.ID, err)); err != nil {
				return err
			}
			continue
		}
		if _, done, err := r.s.store.Imported(ctx, r.job.AdministrationID, kindFinancialMutation, id); err != nil {
			return err
		} else if done {
			st.Skipped++
			continue
		}
		todo = append(todo, v.ID)
	}
	type statement struct {
		account   resource.ID
		mutations []mbMutation
	}
	statements := map[resource.ID]*statement{}
	var order []resource.ID
	for start := 0; start < len(todo); start += batch {
		raws, err := r.read.FetchByIDs(ctx, r.job.SourceID, "financial_mutations", todo[start:min(start+batch, len(todo))])
		if err != nil {
			return err
		}
		for _, raw := range raws {
			var m mbMutation
			if err := json.Unmarshal(raw, &m); err != nil || m.ID == 0 {
				if err := r.fail(ctx, st, kindFinancialMutation, 0, errors.New(errOr(err, "no id"))); err != nil {
					return err
				}
				continue
			}
			key := m.FinancialStatement.id()
			if key == 0 {
				// A transaction without a statement gets one of its own.
				key = m.ID.id()
			}
			if statements[key] == nil {
				account, err := r.mappedOrSame(ctx, kindFinancialAccount, m.FinancialAccountID.id())
				if err != nil {
					return err
				}
				statements[key] = &statement{account: account}
				order = append(order, key)
			}
			statements[key].mutations = append(statements[key].mutations, m)
		}
	}
	for _, key := range order {
		s := statements[key]
		slices.SortStableFunc(s.mutations, func(a, b mbMutation) int { return cmp.Or(cmp.Compare(a.Date, b.Date), cmp.Compare(a.ID, b.ID)) })
		in := banking.ImportedStatement{Origin: resource.Origin{ID: key, CreatedAt: s.mutations[0].CreatedAt}, AccountID: s.account, Reference: "Moneybird"}
		for _, m := range s.mutations {
			in.Mutations = append(in.Mutations, banking.ImportedMutation{Origin: origin(m.ID, m.stamps), Input: m.input()})
		}
		if _, err := r.s.targets.Banking.ImportStatement(ctx, r.job.AdministrationID, r.job.UserID, in); err != nil {
			if isRecordProblem(err) {
				// Each of its transactions failed with it.
				for _, m := range s.mutations {
					if err := r.fail(ctx, st, kindFinancialMutation, m.ID.id(), fmt.Errorf("statement %d: %s", key, describe(err))); err != nil {
						return err
					}
				}
				continue
			}
			return err
		}
		for _, m := range s.mutations {
			if err := r.s.store.ResolveProblems(ctx, r.job.AdministrationID, kindFinancialMutation, m.ID.id()); err != nil {
				return err
			}
			if err := r.bookOnCategories(ctx, st, m); err != nil {
				return err
			}
			if err := r.s.store.RecordImported(ctx, r.job.AdministrationID, kindFinancialMutation, m.ID.id(), m.ID.id()); err != nil {
				return err
			}
			st.Imported++
		}
		if err := r.save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *run) salesInvoices(ctx context.Context) error {
	return r.synced(ctx, StepSalesInvoices, "sales_invoices", kindSalesInvoice, everything, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var inv mbSalesInvoice
		if err := json.Unmarshal(raw, &inv); err != nil {
			return 0, err
		}
		details, err := r.details(ctx, inv.Details)
		if err != nil {
			return 0, err
		}
		payments, err := r.payments(ctx, inv.Payments)
		if err != nil {
			return 0, err
		}
		discount := string(inv.Discount)
		c := sales.Changes{
			ContactID: ref(inv.ContactID), ContactPersonID: ref(inv.ContactPersonID), Reference: opt(inv.Reference), InvoiceDate: opt(inv.InvoiceDate),
			Currency: opt(inv.Currency), PricesAreInclTax: inv.PricesAreInclTax, PaymentConditions: inv.PaymentConditions,
		}
		if discount != "" {
			c.Discount = &discount
		}
		if c.WorkflowID, c.IdentityID, err = r.invoiceRefs(ctx, inv.WorkflowID, inv.IdentityID); err != nil {
			return 0, err
		}
		fields, err := r.fieldValues(ctx, inv.CustomFields)
		if err != nil {
			return 0, err
		}
		for _, f := range fields {
			c.CustomFields = append(c.CustomFields, sales.FieldValue{FieldID: f.id, Value: f.value})
		}
		in := sales.ImportedInvoice{
			Origin: origin(inv.ID, inv.stamps), Changes: c, Details: details, InvoiceID: deref(inv.InvoiceID), Sent: inv.sent(),
			DueDate: day(inv.DueDate), SentAt: inv.SentAt.time(), PaymentReference: deref(inv.PaymentReference), OriginalID: ref(inv.OriginalSalesInvoiceID),
			Payments: payments, Writeoff: sales.Writeoff{DubiousOn: deref(inv.MarkedDubiousOn), UncollectibleOn: deref(inv.MarkedUncollectibleOn)},
		}
		if inv.DraftID != nil {
			in.DraftID = *inv.DraftID
		}
		in.OriginalEstimateID, in.RecurringID = ref(inv.OriginalEstimateID), ref(inv.RecurringID)
		got, err := r.s.targets.Sales.ImportInvoice(ctx, r.job.AdministrationID, r.job.UserID, in)
		if err != nil {
			return 0, err
		}
		st := r.job.Step(StepSalesInvoices)
		add := func(ctx context.Context, name, contentType string, data []byte) error {
			_, err := r.s.targets.Sales.AddAttachment(ctx, r.job.AdministrationID, got.ID, r.job.UserID, name, contentType, data)
			return err
		}
		if err := r.attach(ctx, st, "sales_invoices", got.ID, inv.Attachments, add); err != nil {
			return 0, err
		}
		if err := r.notes(ctx, st, sales.EntityType, got.ID, inv.Notes); err != nil {
			return 0, err
		}
		if inv.sent() && deref(inv.InvoiceID) != "" {
			if err := r.keepPDF(ctx, st, fmt.Sprintf("sales_invoices/%d/download_pdf", got.ID), fileName(deref(inv.InvoiceID))+" (Moneybird).pdf", add); err != nil {
				return 0, err
			}
		}
		return inv.ID.id(), nil
	})
}

func (r *run) externalInvoices(ctx context.Context) error {
	return r.synced(ctx, StepExternalInvoices, "external_sales_invoices", kindExternalInvoice, everything, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var e mbExternalInvoice
		if err := json.Unmarshal(raw, &e); err != nil {
			return 0, err
		}
		details, err := r.details(ctx, e.Details)
		if err != nil {
			return 0, err
		}
		payments, err := r.payments(ctx, e.Payments)
		if err != nil {
			return 0, err
		}
		in := sales.ImportedExternal{
			Origin: origin(e.ID, e.stamps), Details: details, Payments: payments,
			Changes: sales.ExternalChanges{
				ContactID: ref(e.ContactID), Reference: opt(e.Reference), Date: opt(e.Date), DueDate: opt(e.DueDate), Currency: opt(e.Currency),
				PricesAreInclTax: e.PricesAreInclTax, Source: opt(e.Source), SourceURL: opt(e.SourceURL),
			},
			Writeoff: sales.Writeoff{DubiousOn: deref(e.MarkedDubiousOn), UncollectibleOn: deref(e.MarkedUncollectibleOn)},
		}
		if e.EntryNumber != nil {
			in.EntryNumber = *e.EntryNumber
		}
		got, err := r.s.targets.Sales.ImportExternal(ctx, r.job.AdministrationID, r.job.UserID, in)
		if err != nil {
			return 0, err
		}
		st := r.job.Step(StepExternalInvoices)
		err = r.attach(ctx, st, "external_sales_invoices", got.ID, e.Attachments, func(ctx context.Context, name, contentType string, data []byte) error {
			_, err := r.s.targets.Sales.AddExternalAttachment(ctx, r.job.AdministrationID, got.ID, r.job.UserID, name, contentType, data)
			return err
		})
		if err != nil {
			return 0, err
		}
		return e.ID.id(), r.notes(ctx, st, sales.ExternalEntityType, got.ID, e.Notes)
	})
}

// details turns Moneybird's invoice lines into lines here, pointing to the
// VAT rates and categories that took Moneybird's place.
func (r *run) details(ctx context.Context, in []mbDetail) ([]sales.ImportedDetail, error) {
	out := make([]sales.ImportedDetail, 0, len(in))
	for _, d := range in {
		rate, err := r.mapped(ctx, kindTaxRate, ref(d.TaxRateID))
		if err != nil {
			return nil, err
		}
		ledger, err := r.mapped(ctx, kindLedgerAccount, ref(d.LedgerAccountID))
		if err != nil {
			return nil, err
		}
		price := string(d.Price)
		input := sales.DetailInput{
			Description: d.Description, Period: opt(d.Period), Amount: opt(d.amount()), TaxRateID: rate, LedgerAccountID: ledger,
			ProjectID: ref(d.ProjectID), ProductID: ref(d.ProductID), RowOrder: d.RowOrder, Optional: d.IsOptional, Selected: d.IsSelected,
		}
		if price != "" {
			input.Price = &price
		}
		out = append(out, sales.ImportedDetail{Origin: origin(d.ID, d.stamps), Input: input})
	}
	return out, nil
}

// payments turns Moneybird's payments into payments here, oldest first. A
// payment made with a bank transaction is made with that transaction here.
func (r *run) payments(ctx context.Context, in []mbPayment) ([]sales.ImportedPayment, error) {
	out := make([]sales.ImportedPayment, 0, len(in))
	for _, p := range in {
		account, err := r.mapped(ctx, kindFinancialAccount, ref(p.FinancialAccountID))
		if err != nil {
			return nil, err
		}
		mutation, err := r.mapped(ctx, kindFinancialMutation, ref(p.FinancialMutationID))
		if err != nil {
			return nil, err
		}
		ledger, err := r.mapped(ctx, kindLedgerAccount, ref(p.LedgerAccountID))
		if err != nil {
			return nil, err
		}
		input := sales.PaymentInput{
			PaymentDate: p.PaymentDate, FinancialAccountID: account, FinancialMutationID: mutation, LedgerAccountID: ledger,
			TransactionIdentifier: deref(p.TransactionIdentifier), ManualPaymentAction: deref(p.ManualPaymentAction), InvoiceID: ref(p.CreditInvoiceID),
		}
		if price := string(p.Price); price != "" {
			input.Price = &price
		}
		if base := string(p.PriceBase); base != "" {
			input.PriceBase = &base
		}
		out = append(out, sales.ImportedPayment{Origin: origin(p.ID, p.stamps), Input: input})
	}
	slices.SortStableFunc(out, func(a, b sales.ImportedPayment) int { return cmp.Compare(a.Input.PaymentDate, b.Input.PaymentDate) })
	return out, nil
}

// mappedOrSame returns the id here of a record imported in another's
// place, or its own id.
func (r *run) mappedOrSame(ctx context.Context, kind string, id resource.ID) (resource.ID, error) {
	here, err := r.mapped(ctx, kind, &id)
	if err != nil || here == nil {
		return id, err
	}
	return *here, nil
}

func ptrTo[T any](v T) *T { return &v }

// day reads a yyyy-mm-dd date, nil when absent or unreadable.
func day(s *string) *time.Time {
	if s == nil {
		return nil
	}
	t, err := time.Parse(time.DateOnly, *s)
	if err != nil {
		return nil
	}
	return &t
}
