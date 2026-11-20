package mbimport

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"

	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/projects"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// Targets are the services that take in imported records, keeping their
// Moneybird ids. Each checks a record as it checks one made here.
type Targets struct {
	Books interface {
		ImportChart(ctx context.Context, admin, user resource.ID, in bookkeeping.ChartImport) (bookkeeping.ChartIDs, error)
		UpdateSettings(ctx context.Context, admin, user resource.ID, c bookkeeping.SettingsChanges) (bookkeeping.Settings, error)
	}
	Contacts interface {
		Import(ctx context.Context, admin, user resource.ID, o resource.Origin, c contacts.Changes, people []contacts.ImportedPerson) (contacts.Contact, error)
	}
	Products interface {
		Import(ctx context.Context, admin, user resource.ID, o resource.Origin, c products.Changes) (products.Product, error)
	}
	Projects interface {
		Import(ctx context.Context, admin, user resource.ID, o resource.Origin, c projects.Changes) (projects.Project, error)
	}
	Banking interface {
		ImportAccount(ctx context.Context, admin, user resource.ID, in banking.ImportedAccount) (banking.Account, error)
		ImportStatement(ctx context.Context, admin, user resource.ID, in banking.ImportedStatement) ([]banking.Mutation, error)
		LinkBooking(ctx context.Context, admin, id, user resource.ID, l banking.Link) (banking.Mutation, error)
	}
	Sales interface {
		ImportInvoice(ctx context.Context, admin, user resource.ID, in sales.ImportedInvoice) (sales.Invoice, error)
		ImportExternal(ctx context.Context, admin, user resource.ID, in sales.ImportedExternal) (sales.ExternalInvoice, error)
		ImportEstimate(ctx context.Context, admin, user resource.ID, in sales.ImportedEstimate) (sales.Estimate, error)
		ImportRecurring(ctx context.Context, admin, user resource.ID, in sales.ImportedRecurring) (sales.Recurring, error)
		AddAttachment(ctx context.Context, admin, id, user resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error)
		AddExternalAttachment(ctx context.Context, admin, id, user resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error)
		AddEstimateAttachment(ctx context.Context, admin, id, user resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error)
	}
	Purchases interface {
		Import(ctx context.Context, admin, user resource.ID, in purchases.ImportedDocument) (purchases.Document, error)
		ImportTypeless(ctx context.Context, admin, user resource.ID, o resource.Origin, c purchases.TypelessChanges) (purchases.Typeless, error)
		ImportGeneral(ctx context.Context, admin, user resource.ID, o resource.Origin, entryNumber int, c purchases.GeneralChanges) (purchases.General, error)
		AddAttachment(ctx context.Context, admin, user resource.ID, kind string, id resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error)
		AddTypelessAttachment(ctx context.Context, admin, id, user resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error)
		AddGeneralAttachment(ctx context.Context, admin, id, user resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error)
	}
	Journal interface {
		Import(ctx context.Context, admin, user resource.ID, o resource.Origin, c journal.DocumentChanges, rows []resource.Origin) (journal.Document, error)
		AddDocumentAttachment(ctx context.Context, admin, id, user resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error)
	}
}

// batch is how many records a step takes in before it saves its progress,
// the most Moneybird's synchronization returns at once.
const batch = 100

// run is one job at work: its Moneybird reader and a way to save progress.
type run struct {
	s     *Service
	job   *Job
	token string
	read  Reader
	save  func(ctx context.Context) error
}

func (r *run) step(ctx context.Context, kind string) error {
	switch kind {
	case StepChart:
		return r.chart(ctx)
	case StepContacts:
		return r.contacts(ctx)
	case StepProducts:
		return r.products(ctx)
	case StepProjects:
		return r.projects(ctx)
	case StepFinancialAccounts:
		return r.financialAccounts(ctx)
	case StepFinancialMutations:
		return r.financialMutations(ctx)
	case StepEstimates:
		return r.estimates(ctx)
	case StepRecurring:
		return r.recurring(ctx)
	case StepSalesInvoices:
		return r.salesInvoices(ctx)
	case StepExternalInvoices:
		return r.externalInvoices(ctx)
	case StepPurchaseInvoices:
		return r.purchases(ctx, StepPurchaseInvoices, "documents/purchase_invoices", purchases.PurchaseInvoice)
	case StepReceipts:
		return r.purchases(ctx, StepReceipts, "documents/receipts", purchases.Receipt)
	case StepGeneralDocuments:
		return r.generalDocuments(ctx)
	case StepTypelessDocuments:
		return r.typelessDocuments(ctx)
	case StepJournal:
		return r.journal(ctx)
	case StepLockedPeriod:
		return r.lockedPeriod(ctx)
	}
	return fmt.Errorf("unknown import step %q", kind)
}

// mapped returns the id here of a record Moneybird refers to by id: the same
// id, unless the chart step took a default in its place.
func (r *run) mapped(ctx context.Context, kind string, source *resource.ID) (*resource.ID, error) {
	if source == nil {
		return nil, nil
	}
	target, ok, err := r.s.store.Imported(ctx, r.job.AdministrationID, kind, *source)
	if err != nil || !ok {
		return nil, err
	}
	return &target, nil
}

// The kinds the store remembers imported ids by.
const (
	kindLedgerAccount = "ledger_account"
	kindTaxRate       = "tax_rate"
	kindContact       = "contact"
	kindProduct       = "product"
	kindProject       = "project"
	// kindAccountLedger maps a financial account's Moneybird id to the
	// category its money is in here.
	kindAccountLedger     = "account_ledger"
	kindFinancialAccount  = "financial_account"
	kindFinancialMutation = "financial_mutation"
	kindSalesInvoice      = "sales_invoice"
	kindExternalInvoice   = "external_sales_invoice"
)

func (r *run) chart(ctx context.Context) error {
	st := r.job.Step(StepChart)
	accounts, err := r.read.List(ctx, r.job.SourceID, "ledger_accounts", nil)
	if err != nil {
		return err
	}
	rates, err := r.read.List(ctx, r.job.SourceID, "tax_rates", nil)
	if err != nil {
		return err
	}
	st.Total = len(accounts) + len(rates)
	in := bookkeeping.ChartImport{}
	kinds := map[resource.ID]string{}
	accountLedgers := map[resource.ID]resource.ID{}
	for _, raw := range accounts {
		var a mbLedgerAccount
		if err := json.Unmarshal(raw, &a); err != nil || a.ID == 0 {
			st.problem("category: %v", errOr(err, "no id"))
			continue
		}
		in.Accounts = append(in.Accounts, a.imported())
		kinds[a.ID.id()] = kindLedgerAccount
		if a.FinancialAccountID != nil && *a.FinancialAccountID != 0 {
			accountLedgers[a.FinancialAccountID.id()] = a.ID.id()
		}
	}
	for _, raw := range rates {
		var t mbTaxRate
		if err := json.Unmarshal(raw, &t); err != nil || t.ID == 0 {
			st.problem("VAT rate: %v", errOr(err, "no id"))
			continue
		}
		in.Rates = append(in.Rates, t.imported())
		kinds[t.ID.id()] = kindTaxRate
	}
	ids, err := r.s.targets.Books.ImportChart(ctx, r.job.AdministrationID, r.job.UserID, in)
	if err != nil {
		return err
	}
	for source, target := range ids {
		if err := r.s.store.RecordImported(ctx, r.job.AdministrationID, kinds[source], source, target); err != nil {
			return err
		}
	}
	for account, ledger := range accountLedgers {
		if here, ok := ids[ledger]; ok {
			if err := r.s.store.RecordImported(ctx, r.job.AdministrationID, kindAccountLedger, account, here); err != nil {
				return err
			}
		}
	}
	st.Imported = len(ids)
	st.Skipped = st.Total - st.Imported - st.Failed
	return nil
}

func (r *run) contacts(ctx context.Context) error {
	return r.synced(ctx, StepContacts, "contacts", kindContact, nil, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var c mbContact
		if err := json.Unmarshal(raw, &c); err != nil {
			return 0, err
		}
		_, err := r.s.targets.Contacts.Import(ctx, r.job.AdministrationID, r.job.UserID, c.origin(), c.changes(), c.people())
		return c.ID.id(), err
	})
}

func (r *run) products(ctx context.Context) error {
	return r.listed(ctx, StepProducts, "products", nil, kindProduct, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var p mbProduct
		if err := json.Unmarshal(raw, &p); err != nil {
			return 0, err
		}
		changes := p.changes()
		var err error
		if changes.TaxRateID, err = r.mapped(ctx, kindTaxRate, ref(p.TaxRateID)); err != nil {
			return 0, err
		}
		if changes.LedgerAccountID, err = r.mapped(ctx, kindLedgerAccount, ref(p.LedgerAccountID)); err != nil {
			return 0, err
		}
		_, err = r.s.targets.Products.Import(ctx, r.job.AdministrationID, r.job.UserID, p.origin(), changes)
		return p.ID.id(), err
	})
}

func (r *run) projects(ctx context.Context) error {
	// Moneybird lists active projects unless asked for all.
	query := url.Values{"filter": {"state:all"}}
	return r.listed(ctx, StepProjects, "projects", query, kindProject, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var p mbProject
		if err := json.Unmarshal(raw, &p); err != nil {
			return 0, err
		}
		_, err := r.s.targets.Projects.Import(ctx, r.job.AdministrationID, r.job.UserID, p.origin(), p.changes())
		return p.ID.id(), err
	})
}

// importOne takes in one record and returns its Moneybird id.
type importOne func(ctx context.Context, raw json.RawMessage) (resource.ID, error)

// synced imports the records of a kind Moneybird synchronizes: it lists
// their ids and versions, and fetches only the ones not imported yet, a
// batch at a time.
func (r *run) synced(ctx context.Context, step, path, kind string, query url.Values, one importOne) error {
	st := r.job.Step(step)
	versions, err := r.read.SyncIDs(ctx, r.job.SourceID, path, query)
	if err != nil {
		return err
	}
	// Added up: a kind may be listed in parts, such as by state.
	st.Total += len(versions)
	var todo []string
	for _, v := range versions {
		id, err := resource.ParseID(v.ID)
		if err != nil {
			st.problem("%s %s: %v", kind, v.ID, err)
			continue
		}
		_, done, err := r.s.store.Imported(ctx, r.job.AdministrationID, kind, id)
		if err != nil {
			return err
		}
		if done {
			st.Skipped++
			continue
		}
		todo = append(todo, v.ID)
	}
	// Oldest first: a credit invoice comes after the invoice it credits.
	slices.SortFunc(todo, func(a, b string) int { return cmp.Or(cmp.Compare(len(a), len(b)), cmp.Compare(a, b)) })
	for start := 0; start < len(todo); start += batch {
		raws, err := r.read.FetchByIDs(ctx, r.job.SourceID, path, todo[start:min(start+batch, len(todo))])
		if err != nil {
			return err
		}
		slices.SortStableFunc(raws, func(a, b json.RawMessage) int { return cmp.Compare(idOf(a), idOf(b)) })
		if err := r.take(ctx, st, kind, raws, one); err != nil {
			return err
		}
	}
	return nil
}

// listed imports the records of a kind Moneybird only lists, page by page.
func (r *run) listed(ctx context.Context, step, path string, query url.Values, kind string, one importOne) error {
	st := r.job.Step(step)
	raws, err := r.read.List(ctx, r.job.SourceID, path, query)
	if err != nil {
		return err
	}
	st.Total = len(raws)
	for start := 0; start < len(raws); start += batch {
		if err := r.take(ctx, st, kind, raws[start:min(start+batch, len(raws))], one); err != nil {
			return err
		}
	}
	return nil
}

// take imports a batch, skipping what was imported before and noting what
// fails, and saves the progress.
func (r *run) take(ctx context.Context, st *Step, kind string, raws []json.RawMessage, one importOne) error {
	for _, raw := range raws {
		var head struct {
			ID mbID `json:"id"`
		}
		if err := json.Unmarshal(raw, &head); err != nil || head.ID == 0 {
			st.problem("%s: %v", kind, errOr(err, "no id"))
			continue
		}
		if _, done, err := r.s.store.Imported(ctx, r.job.AdministrationID, kind, head.ID.id()); err != nil {
			return err
		} else if done {
			st.Skipped++
			continue
		}
		id, err := one(ctx, raw)
		if err != nil {
			if isRecordProblem(err) {
				st.problem("%s %d: %v", kind, head.ID, err)
				continue
			}
			return err
		}
		// A record that took another's place has recorded where it went.
		if _, mapped, err := r.s.store.Imported(ctx, r.job.AdministrationID, kind, id); err != nil {
			return err
		} else if !mapped {
			if err := r.s.store.RecordImported(ctx, r.job.AdministrationID, kind, id, id); err != nil {
				return err
			}
		}
		st.Imported++
	}
	return r.save(ctx)
}

// isRecordProblem tells a record the import cannot take, which it notes and
// passes, from a failure that stops the import.
func isRecordProblem(err error) bool {
	var verr *resource.ValidationError
	var syntax *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &verr) || errors.As(err, &syntax) || errors.As(err, &typeErr)
}

func errOr(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}

// idOf reads a record's Moneybird id, zero when it has none.
func idOf(raw json.RawMessage) resource.ID {
	var head struct {
		ID mbID `json:"id"`
	}
	_ = json.Unmarshal(raw, &head)
	return head.ID.id()
}
