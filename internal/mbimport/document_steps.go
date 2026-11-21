package mbimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// The kinds the store remembers imported ids by, for the documents.
const (
	kindEstimate  = "estimate"
	kindRecurring = "recurring_sales_invoice"
	kindPurchase  = "purchase_invoice"
	kindReceipt   = "receipt"
	kindGeneral   = "general_document"
	kindTypeless  = "typeless_document"
	kindJournal   = "general_journal_document"
)

// addFile stores a downloaded file with a record here.
type addFile func(ctx context.Context, filename, contentType string, data []byte) error

// attach downloads a record's files from Moneybird and stores them with it.
// A file that cannot come in is noted; the record stays.
func (r *run) attach(ctx context.Context, st *Step, kind, base string, id resource.ID, files []mbAttachment, add addFile) error {
	for _, f := range files {
		if !r.wants(fmt.Sprintf("file %s left out", f.Filename)) {
			continue
		}
		data, contentType, err := r.read.Download(ctx, r.job.SourceID, fmt.Sprintf("%s/%d/attachments/%d/download", base, id, f.ID))
		if err == nil {
			if f.ContentType != "" {
				contentType = f.ContentType
			}
			err = add(ctx, cmpOr(f.Filename, fmt.Sprintf("attachment-%d", f.ID)), contentType, data)
		}
		if err != nil {
			if isRecordProblem(err) || isFileProblem(err) {
				if err := r.partial(ctx, st, kind, id, fmt.Sprintf("file %s left out: %s", f.Filename, describe(err))); err != nil {
					return err
				}
				continue
			}
			return err
		}
	}
	return nil
}

// keepPDF stores the PDF Moneybird made of a sent invoice or estimate: the
// one the contact received, which a PDF made here would not be.
func (r *run) keepPDF(ctx context.Context, st *Step, kind string, id resource.ID, path, name string, add addFile) error {
	if !r.wants(path + " left out") {
		return nil
	}
	data, _, err := r.read.Download(ctx, r.job.SourceID, path)
	if err == nil {
		err = add(ctx, name, "application/pdf", data)
	}
	if err != nil {
		if isRecordProblem(err) || isFileProblem(err) {
			return r.partial(ctx, st, kind, id, fmt.Sprintf("%s left out: %s", path, describe(err)))
		}
		return err
	}
	return nil
}

func (r *run) estimates(ctx context.Context) error {
	return r.synced(ctx, StepEstimates, "estimates", kindEstimate, everything, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var e mbEstimate
		if err := json.Unmarshal(raw, &e); err != nil {
			return 0, err
		}
		details, err := r.details(ctx, e.Details)
		if err != nil {
			return 0, err
		}
		c := sales.EstimateChanges{
			ContactID: ref(e.ContactID), ContactPersonID: ref(e.ContactPersonID), Reference: opt(e.Reference), EstimateDate: opt(e.EstimateDate),
			Language: opt(e.Language), Currency: opt(e.Currency), ShowTax: e.ShowTax, PricesAreInclTax: e.PricesAreInclTax,
			PreText: e.PreText, PostText: e.PostText,
		}
		if d := string(e.Discount); d != "" {
			c.Discount = &d
		}
		if c.WorkflowID, c.IdentityID, err = r.invoiceRefs(ctx, e.WorkflowID, e.IdentityID); err != nil {
			return 0, err
		}
		fields, err := r.fieldValues(ctx, e.CustomFields)
		if err != nil {
			return 0, err
		}
		for _, f := range fields {
			c.CustomFields = append(c.CustomFields, sales.FieldValue{FieldID: f.id, Value: f.value})
		}
		in := sales.ImportedEstimate{
			Origin: origin(e.ID, e.stamps), Changes: c, Details: details, EstimateID: deref(e.EstimateID), State: e.State,
			DueDate: day(e.DueDate), SentAt: e.SentAt.time(), AcceptedAt: e.AcceptedAt.time(), RejectedAt: e.RejectedAt.time(), ArchivedAt: e.ArchivedAt.time(),
		}
		if e.DraftID != nil {
			in.DraftID = *e.DraftID
		}
		est, err := r.s.targets.Sales.ImportEstimate(ctx, r.job.AdministrationID, r.job.UserID, in)
		if err != nil {
			return 0, err
		}
		st := r.job.Step(StepEstimates)
		add := func(ctx context.Context, name, contentType string, data []byte) error {
			_, err := r.s.targets.Sales.AddEstimateAttachment(ctx, r.job.AdministrationID, est.ID, r.job.UserID, name, contentType, data)
			return err
		}
		if err := r.attach(ctx, st, kindEstimate, "estimates", est.ID, e.Attachments, add); err != nil {
			return 0, err
		}
		if err := r.notes(ctx, st, sales.EstimateEntityType, est.ID, e.Notes); err != nil {
			return 0, err
		}
		if e.State != "draft" && deref(e.EstimateID) != "" {
			if err := r.keepPDF(ctx, st, kindEstimate, e.ID.id(), fmt.Sprintf("estimates/%d/download_pdf", est.ID), fileName(deref(e.EstimateID))+" (Moneybird).pdf", add); err != nil {
				return 0, err
			}
		}
		return e.ID.id(), nil
	})
}

func (r *run) recurring(ctx context.Context) error {
	one := func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var rec mbRecurring
		if err := json.Unmarshal(raw, &rec); err != nil {
			return 0, err
		}
		details, err := r.details(ctx, rec.Details)
		if err != nil {
			return 0, err
		}
		c := sales.RecurringChanges{
			ContactID: ref(rec.ContactID), ContactPersonID: ref(rec.ContactPersonID), Reference: opt(rec.Reference), PaymentConditions: rec.PaymentConditions,
			Currency: opt(rec.Currency), FirstDueInterval: rec.FirstDueInterval, PricesAreInclTax: rec.PricesAreInclTax, InvoiceDate: opt(rec.InvoiceDate),
			FrequencyType: opt(rec.FrequencyType), Frequency: rec.Frequency, LastDate: opt(rec.LastDate),
		}
		if d := string(rec.Discount); d != "" {
			c.Discount = &d
		}
		if c.WorkflowID, err = r.mapped(ctx, kindWorkflow, ref(rec.WorkflowID)); err != nil {
			return 0, err
		}
		got, err := r.s.targets.Sales.ImportRecurring(ctx, r.job.AdministrationID, r.job.UserID, sales.ImportedRecurring{
			Origin: origin(rec.ID, rec.stamps), Changes: c, Details: details, StartDate: day(rec.StartDate),
		})
		if err != nil {
			return 0, err
		}
		return rec.ID.id(), r.notes(ctx, r.job.Step(StepRecurring), sales.RecurringEntityType, got.ID, rec.Notes)
	}
	// Moneybird lists only the active ones unless asked for each state.
	for _, state := range []string{"active", "inactive"} {
		if err := r.synced(ctx, StepRecurring, "recurring_sales_invoices", kindRecurring, url.Values{"filter": {"state:" + state}}, one); err != nil {
			return err
		}
	}
	return nil
}

func (r *run) purchases(ctx context.Context, step, path, kind string) error {
	idKind := kindPurchase
	if kind == purchases.Receipt {
		idKind = kindReceipt
	}
	return r.synced(ctx, step, path, idKind, everything, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var d mbPurchase
		if err := json.Unmarshal(raw, &d); err != nil {
			return 0, err
		}
		details, err := r.details(ctx, d.Details)
		if err != nil {
			return 0, err
		}
		payments, err := r.payments(ctx, d.Payments)
		if err != nil {
			return 0, err
		}
		in := purchases.ImportedDocument{
			Origin: origin(d.ID, d.stamps), Kind: kind, Details: details, Payments: payments,
			Changes: purchases.Changes{
				ContactID: ref(d.ContactID), Reference: opt(d.Reference), Date: opt(d.Date), DueDate: opt(d.DueDate), Currency: opt(d.Currency),
				PricesAreInclTax: d.PricesAreInclTax, RevenueInvoice: d.RevenueInvoice,
			},
		}
		if d.EntryNumber != nil {
			in.EntryNumber = *d.EntryNumber
		}
		doc, err := r.s.targets.Purchases.Import(ctx, r.job.AdministrationID, r.job.UserID, in)
		if err != nil {
			return 0, err
		}
		err = r.attach(ctx, r.job.Step(step), idKind, path, doc.ID, d.Attachments, func(ctx context.Context, name, contentType string, data []byte) error {
			_, err := r.s.targets.Purchases.AddAttachment(ctx, r.job.AdministrationID, r.job.UserID, kind, doc.ID, name, contentType, data)
			return err
		})
		if err != nil {
			return 0, err
		}
		return d.ID.id(), r.notes(ctx, r.job.Step(step), kind, doc.ID, d.Notes)
	})
}

func (r *run) generalDocuments(ctx context.Context) error {
	return r.synced(ctx, StepGeneralDocuments, "documents/general_documents", kindGeneral, everything, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var d mbDocumentBase
		if err := json.Unmarshal(raw, &d); err != nil {
			return 0, err
		}
		number := 0
		if d.EntryNumber != nil {
			number = *d.EntryNumber
		}
		g, err := r.s.targets.Purchases.ImportGeneral(ctx, r.job.AdministrationID, r.job.UserID, origin(d.ID, d.stamps), number,
			purchases.GeneralChanges{ContactID: ref(d.ContactID), Reference: opt(d.Reference), Date: opt(d.Date), DueDate: opt(d.DueDate)})
		if err != nil {
			return 0, err
		}
		err = r.attach(ctx, r.job.Step(StepGeneralDocuments), kindGeneral, "documents/general_documents", g.ID, d.Attachments, func(ctx context.Context, name, contentType string, data []byte) error {
			_, err := r.s.targets.Purchases.AddGeneralAttachment(ctx, r.job.AdministrationID, g.ID, r.job.UserID, name, contentType, data)
			return err
		})
		if err != nil {
			return 0, err
		}
		return d.ID.id(), r.notes(ctx, r.job.Step(StepGeneralDocuments), purchases.GeneralDocument, g.ID, d.Notes)
	})
}

func (r *run) typelessDocuments(ctx context.Context) error {
	return r.synced(ctx, StepTypelessDocuments, "documents/typeless_documents", kindTypeless, everything, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var d mbDocumentBase
		if err := json.Unmarshal(raw, &d); err != nil {
			return 0, err
		}
		t, err := r.s.targets.Purchases.ImportTypeless(ctx, r.job.AdministrationID, r.job.UserID, origin(d.ID, d.stamps),
			purchases.TypelessChanges{ContactID: ref(d.ContactID), Reference: opt(d.Reference), Date: opt(d.Date)})
		if err != nil {
			return 0, err
		}
		err = r.attach(ctx, r.job.Step(StepTypelessDocuments), kindTypeless, "documents/typeless_documents", t.ID, d.Attachments, func(ctx context.Context, name, contentType string, data []byte) error {
			_, err := r.s.targets.Purchases.AddTypelessAttachment(ctx, r.job.AdministrationID, t.ID, r.job.UserID, name, contentType, data)
			return err
		})
		return d.ID.id(), err
	})
}

func (r *run) journal(ctx context.Context) error {
	return r.synced(ctx, StepJournal, "documents/general_journal_documents", kindJournal, everything, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var j mbJournal
		if err := json.Unmarshal(raw, &j); err != nil {
			return 0, err
		}
		c := journal.DocumentChanges{Reference: opt(j.Reference), Date: opt(j.Date), JournalType: opt(j.JournalType)}
		var rows []resource.Origin
		for _, e := range j.Entries {
			ledger, err := r.mapped(ctx, kindLedgerAccount, ref(e.LedgerAccountID))
			if err != nil {
				return 0, err
			}
			row := journal.RowInput{LedgerAccountID: ledger, ContactID: ref(e.ContactID), ProjectID: ref(e.ProjectID), Description: e.Description, RowOrder: e.RowOrder}
			if d := string(e.Debit); d != "" {
				row.Debit = &d
			}
			if cr := string(e.Credit); cr != "" {
				row.Credit = &cr
			}
			c.Rows = append(c.Rows, row)
			rows = append(rows, resource.Origin{ID: e.ID.id()})
		}
		doc, err := r.s.targets.Journal.Import(ctx, r.job.AdministrationID, r.job.UserID, origin(j.ID, j.stamps), c, rows)
		if err != nil {
			return 0, err
		}
		err = r.attach(ctx, r.job.Step(StepJournal), kindJournal, "documents/general_journal_documents", doc.ID, j.Attachments, func(ctx context.Context, name, contentType string, data []byte) error {
			_, err := r.s.targets.Journal.AddDocumentAttachment(ctx, r.job.AdministrationID, doc.ID, r.job.UserID, name, contentType, data)
			return err
		})
		if err != nil {
			return 0, err
		}
		return j.ID.id(), r.notes(ctx, r.job.Step(StepJournal), journal.GeneralJournalDocument, doc.ID, j.Notes)
	})
}

// bookOnCategories books a transaction on the categories Moneybird booked
// it on. Its payments of invoices and documents come in with those.
func (r *run) bookOnCategories(ctx context.Context, st *Step, m mbMutation) error {
	for _, b := range m.Bookings {
		ledger, err := r.mapped(ctx, kindLedgerAccount, ref(b.LedgerAccountID))
		if err != nil {
			return err
		}
		if ledger == nil {
			if err := r.partial(ctx, st, kindFinancialMutation, m.ID.id(), "a booking left out: its category is not here"); err != nil {
				return err
			}
			continue
		}
		// A booking's amount goes the transaction's way here, however Moneybird signs it.
		price := strings.TrimPrefix(string(b.Price), "-")
		link := banking.Link{Type: "LedgerAccount", BookingID: ledger, Description: deref(b.Description), ProjectID: ref(b.ProjectID)}
		if price != "" {
			link.Price = &price
		}
		if _, err := r.s.targets.Banking.LinkBooking(ctx, r.job.AdministrationID, m.ID.id(), r.job.UserID, link); err != nil {
			if isRecordProblem(err) {
				if err := r.partial(ctx, st, kindFinancialMutation, m.ID.id(), fmt.Sprintf("booking %d left out: %s", b.ID, describe(err))); err != nil {
					return err
				}
				continue
			}
			return err
		}
	}
	return nil
}

// lockedPeriod locks the books up to the day Moneybird had them locked.
func (r *run) lockedPeriod(ctx context.Context) error {
	st := r.job.Step(StepLockedPeriod)
	sources, err := r.s.sourceAdministrations(ctx, r.token)
	if err != nil {
		return err
	}
	for _, a := range sources {
		if a.ID != r.job.SourceID || a.PeriodLockedUntil == "" {
			continue
		}
		st.Total = 1
		locked := a.PeriodLockedUntil
		if _, err := r.s.targets.Books.UpdateSettings(ctx, r.job.AdministrationID, r.job.UserID, bookkeeping.SettingsChanges{LockedUntil: &locked}); err != nil {
			if isRecordProblem(err) {
				return r.fail(ctx, st, "locked_period", 0, err)
			}
			return err
		}
		st.Imported = 1
	}
	return nil
}

func fileName(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, s)
}

func cmpOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// isFileProblem tells a file that cannot come in from a failure that stops
// the import.
func isFileProblem(err error) bool { return errors.Is(err, ErrFileUnavailable) }
