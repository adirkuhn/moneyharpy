package sales

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// external makes an external invoice of 100 + 21% VAT = 121.00 dated day.
func external(t *testing.T, h harness, day string) ExternalInvoice {
	t.Helper()
	e, err := h.svc.CreateExternal(context.Background(), adminID, owner, ExternalChanges{
		ContactID: ptr(customer), Reference: ptr("WEB-1001"), Date: ptr(day), DueDate: ptr(day), Source: ptr("Example webshop"),
		Details: []DetailInput{line("100", "1", vat21, revenue)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestCreateExternalBooksIt(t *testing.T) {
	h := newHarness(t)
	e := external(t, h, "2026-11-01")
	if e.State != Late || e.EntryNumber != 1 || e.Totals.InclTax != 12100 || e.Currency != "EUR" || e.Origin != OriginAPI {
		t.Errorf("external = %+v", e)
	}
	if e.Ref().URI() != "/100/external_sales_invoices/"+e.ID.String() {
		t.Errorf("uri = %s", e.Ref().URI())
	}
	entries := h.store.externalEntries[e.ID]
	if s := sums(entries); s[debtors] != 12100 || s[revenue] != -10000 || s[payableVAT] != -2100 {
		t.Errorf("posted %v", s)
	}
	for _, en := range entries {
		if en.DocumentType != journal.ExternalSalesInvoice || en.DocumentID != e.ID || en.Date.Format(time.DateOnly) != "2026-11-01" {
			t.Errorf("entry = %+v", en)
		}
	}
	if entries[0].Description != "WEB-1001" || *entries[0].ContactID != customer {
		t.Errorf("debtors entry = %+v", entries[0])
	}
	if second := external(t, h, "2026-11-11"); second.EntryNumber != 2 || second.State != Open {
		t.Errorf("second = %+v", second)
	}
}

func TestCreateExternalValidates(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	for name, c := range map[string]ExternalChanges{
		"contact_id":                       {ContactID: ptr(resource.ID(999))},
		"date":                             {Date: ptr("yesterday")},
		"due_date":                         {Date: ptr("2026-11-01"), DueDate: ptr("2026-10-01")},
		"currency":                         {Currency: ptr("euro")},
		"source_url":                       {SourceURL: ptr("javascript:alert(1)")},
		"details_attributes.0.tax_rate_id": {Details: []DetailInput{line("10", "1", purchaseVAT, revenue)}},
	} {
		if _, err := h.svc.CreateExternal(ctx, adminID, owner, c); fieldsOf(t, err)[name] == nil {
			t.Errorf("no error for %s: %v", name, err)
		}
	}
	h.ledger.locked = "2026-10-31"
	if _, err := h.svc.CreateExternal(ctx, adminID, owner, ExternalChanges{ContactID: ptr(customer), Date: ptr("2026-10-15"), Details: []DetailInput{line("10", "1", vat21, 0)}}); fieldsOf(t, err)["date"] == nil {
		t.Error("booked in the locked period")
	}
	if _, err := h.svc.CreateExternal(ctx, adminID, stranger, ExternalChanges{}); err == nil {
		t.Error("a stranger created an invoice")
	}
}

func TestExternalFromAFileStartsNew(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e, err := h.svc.CreateExternalFromFile(ctx, adminID, owner, "invoice.pdf", "application/pdf", []byte("%PDF-1.4"))
	if err != nil {
		t.Fatal(err)
	}
	if e.State != New || e.ContactID != 0 || e.Date.Format(time.DateOnly) != "2026-11-11" || e.Origin != OriginUpload || len(e.Attachments) != 1 {
		t.Fatalf("new = %+v", e)
	}
	if len(h.store.externalEntries[e.ID]) != 0 {
		t.Errorf("a new invoice posted %v", h.store.externalEntries[e.ID])
	}
	if _, err := h.svc.CreateExternalFromFile(ctx, adminID, owner, "tool.exe", "application/octet-stream", []byte{0x4d, 0x5a, 0x90, 0x00}); err == nil {
		t.Error("accepted a program")
	}
	if list, _, _ := h.svc.ExternalList(ctx, adminID, owner, Query{}); len(list) != 1 {
		t.Errorf("a refused file left an invoice: %d", len(list))
	}

	// Filling in the contact alone keeps it new; a line books it.
	e, err = h.svc.UpdateExternal(ctx, adminID, e.ID, owner, ExternalChanges{ContactID: ptr(customer)})
	if err != nil || e.State != New {
		t.Fatalf("with contact = %s, %v", e.State, err)
	}
	e, err = h.svc.UpdateExternal(ctx, adminID, e.ID, owner, ExternalChanges{Details: []DetailInput{line("50", "1", vat21, 0)}})
	if err != nil || e.State != Open || e.Totals.InclTax != 6050 {
		t.Fatalf("with line = %+v, %v", e, err)
	}
	if s := sums(h.store.externalEntries[e.ID]); s[debtors] != 6050 || s[uncategorized] != -5000 {
		t.Errorf("posted %v", s)
	}
	// Once booked it keeps its contact and lines.
	if _, err := h.svc.UpdateExternal(ctx, adminID, e.ID, owner, ExternalChanges{ContactID: ptr(resource.ID(0))}); fieldsOf(t, err)["contact_id"] == nil {
		t.Error("removed the contact of a booked invoice")
	}
	if _, err := h.svc.UpdateExternal(ctx, adminID, e.ID, owner, ExternalChanges{Details: []DetailInput{{ID: &e.Lines[0].ID, Destroy: true}}}); fieldsOf(t, err)["details_attributes"] == nil {
		t.Error("removed every line of a booked invoice")
	}
}

func TestUpdateExternalRebooksAndRespectsTheLock(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := external(t, h, "2026-11-01")
	e, err := h.svc.UpdateExternal(ctx, adminID, e.ID, owner, ExternalChanges{Date: ptr("2026-11-05"), DueDate: ptr("2026-12-05"), Reference: ptr("WEB-1002"),
		Details: []DetailInput{{ID: &e.Lines[0].ID, Price: ptr("200")}}})
	if err != nil {
		t.Fatal(err)
	}
	if e.State != Open || e.Totals.InclTax != 24200 || e.Reference != "WEB-1002" {
		t.Errorf("updated = %+v", e)
	}
	if s := sums(h.store.externalEntries[e.ID]); s[debtors] != 24200 || h.store.externalEntries[e.ID][0].Date.Format(time.DateOnly) != "2026-11-05" {
		t.Errorf("rebooked %v", s)
	}
	h.ledger.locked = "2026-11-06"
	if _, err := h.svc.UpdateExternal(ctx, adminID, e.ID, owner, ExternalChanges{Reference: ptr("x")}); fieldsOf(t, err)["date"] == nil {
		t.Error("changed an invoice in the locked period")
	}
	h.ledger.locked = ""
	if _, err := h.svc.RegisterExternalPayment(ctx, adminID, e.ID, owner, PaymentInput{PaymentDate: "2026-11-06", Price: ptr("10")}); err != nil {
		t.Fatal(err)
	}
	// As at Moneybird an invoice with payments still changes; what is left to pay sets its state.
	e, err = h.svc.UpdateExternal(ctx, adminID, e.ID, owner, ExternalChanges{Reference: ptr("x")})
	if err != nil || e.Reference != "x" || len(e.Payments) != 1 || e.State != Open {
		t.Fatalf("changed with payments = %+v, %v", e, err)
	}
	if fieldsOf(t, func() error {
		_, err := h.svc.UpdateExternal(ctx, adminID, e.ID, owner, ExternalChanges{Currency: ptr("USD")})
		return err
	}())["currency"] == nil {
		t.Error("changed the currency of an invoice with payments")
	}
}

func TestUpdateExternalFollowsWhatIsPaid(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := external(t, h, "2026-11-01")
	if _, err := h.svc.RegisterExternalPayment(ctx, adminID, e.ID, owner, PaymentInput{PaymentDate: "2026-11-05"}); err != nil {
		t.Fatal(err)
	}
	// A paid invoice stays paid when its total does not grow.
	e, err := h.svc.UpdateExternal(ctx, adminID, e.ID, owner, ExternalChanges{Source: ptr("Example till")})
	if err != nil || e.State != Paid || e.PaidAt == nil {
		t.Fatalf("paid after edit = %+v, %v", e, err)
	}
	// A higher total opens it again.
	e, err = h.svc.UpdateExternal(ctx, adminID, e.ID, owner, ExternalChanges{Details: []DetailInput{{ID: &e.Lines[0].ID, Price: ptr("200")}}})
	if err != nil || e.State != Late || e.PaidAt != nil || e.Unpaid() != 12100 {
		t.Fatalf("after a higher total = %+v, %v", e, err)
	}
	if !slices.Contains(h.notifier.actions, "external_sales_invoice_state_changed_to_late") {
		t.Errorf("events = %v", h.notifier.actions)
	}
	if _, err := h.svc.MarkExternalUncollectible(ctx, adminID, e.ID, owner, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.UpdateExternal(ctx, adminID, e.ID, owner, ExternalChanges{Reference: ptr("y")}); !errors.Is(err, ErrNotEditable) {
		t.Errorf("changed an uncollectible invoice: %v", err)
	}
}

func TestExternalInvoicesKeepTheirHistory(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := external(t, h, "2026-11-01")
	if _, err := h.svc.UpdateExternal(ctx, adminID, e.ID, owner, ExternalChanges{Reference: ptr("WEB-1002")}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.RegisterExternalPayment(ctx, adminID, e.ID, owner, PaymentInput{PaymentDate: "2026-11-05"}); err != nil {
		t.Fatal(err)
	}
	got, err := h.svc.ExternalInvoice(ctx, adminID, e.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	var list []string
	for _, ev := range got.Events {
		list = append(list, ev.Action)
	}
	want := []string{"external_sales_invoice_created", "external_sales_invoice_updated", "payment_registered", "external_sales_invoice_state_changed_to_paid"}
	if !slices.Equal(list, want) {
		t.Errorf("events = %v, want %v", list, want)
	}
	if got.Events[0].UserID != owner {
		t.Errorf("created by %v", got.Events[0].UserID)
	}
	n, _ := h.svc.CreateExternalFromFile(ctx, adminID, owner, "scan.pdf", "application/pdf", []byte("%PDF"))
	if n, _ = h.svc.ExternalInvoice(ctx, adminID, n.ID, owner); len(n.Events) != 1 || n.Events[0].Action != "external_sales_invoice_created" {
		t.Errorf("uploaded events = %+v", n.Events)
	}
}

func TestExternalPayments(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := external(t, h, "2026-11-01")
	p, err := h.svc.RegisterExternalPayment(ctx, adminID, e.ID, owner, PaymentInput{PaymentDate: "2026-11-05"})
	if err != nil {
		t.Fatal(err)
	}
	if p.InvoiceType != journal.ExternalSalesInvoice || p.Price != 12100 {
		t.Errorf("payment = %+v", p)
	}
	if s := sums(h.store.paid[p.ID]); s[toClassify] != 12100 || s[debtors] != -12100 {
		t.Errorf("payment posted %v", s)
	}
	got, _ := h.svc.ExternalInvoice(ctx, adminID, e.ID, owner)
	if got.State != Paid || got.PaidAt.Format(time.DateOnly) != "2026-11-05" || len(got.Payments) != 1 {
		t.Errorf("paid = %+v", got)
	}
	if !slices.Contains(h.notifier.actions, "external_sales_invoice_state_changed_to_paid") {
		t.Errorf("events = %v", h.notifier.actions)
	}
	if err := h.svc.DeleteExternalPayment(ctx, adminID, e.ID, p.ID, owner); err != nil {
		t.Fatal(err)
	}
	if got, _ = h.svc.ExternalInvoice(ctx, adminID, e.ID, owner); got.State != Late || len(got.Payments) != 0 {
		t.Errorf("after delete = %+v", got)
	}
	// A payment on a sales invoice's endpoint does not find an external one.
	if _, err := h.svc.RegisterPayment(ctx, adminID, e.ID, owner, PaymentInput{PaymentDate: "2026-11-05"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("paid an external invoice as a sales invoice: %v", err)
	}
	n, _ := h.svc.CreateExternalFromFile(ctx, adminID, owner, "scan.pdf", "application/pdf", []byte("%PDF"))
	if _, err := h.svc.RegisterExternalPayment(ctx, adminID, n.ID, owner, PaymentInput{PaymentDate: "2026-11-05"}); !errors.Is(err, ErrNotEditable) {
		t.Errorf("paid a new invoice: %v", err)
	}
}

func TestExternalBankPayments(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := external(t, h, "2026-11-01")
	const mutation resource.ID = 900
	h.bank.open[mutation] = 12100
	p, err := h.svc.RegisterExternalPayment(ctx, adminID, e.ID, owner, PaymentInput{FinancialMutationID: ptr(mutation)})
	if err != nil {
		t.Fatal(err)
	}
	if p.InvoiceType != journal.ExternalSalesInvoice || *p.FinancialMutationID != mutation || h.bank.open[mutation] != 0 {
		t.Errorf("bank payment = %+v", p)
	}
	if got, _ := h.svc.ExternalInvoice(ctx, adminID, e.ID, owner); got.State != Paid {
		t.Errorf("state = %s", got.State)
	}
	// Deleting the invoice unlinks its transaction first.
	if err := h.svc.DeleteExternal(ctx, adminID, e.ID, owner); err != nil {
		t.Fatal(err)
	}
	if h.bank.open[mutation] != 12100 || h.store.externals[e.ID].ID != 0 || h.store.externalEntries[e.ID] != nil {
		t.Errorf("after delete: open %s", h.bank.open[mutation])
	}
	if _, err := h.svc.ExternalInvoice(ctx, adminID, e.ID, owner); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted invoice = %v", err)
	}
}

func TestExternalWriteOffs(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := external(t, h, "2026-09-01")
	got, err := h.svc.MarkExternalDubious(ctx, adminID, e.ID, owner, "2026-10-01")
	if err != nil {
		t.Fatal(err)
	}
	if got.MarkedDubiousOn == nil || got.State != Late {
		t.Errorf("dubious = %+v", got)
	}
	if s := sums(h.store.externalEntries[e.ID]); s[allocation] != 10000 || s[provision] != -10000 {
		t.Errorf("dubious posted %v", s)
	}
	got, err = h.svc.MarkExternalUncollectible(ctx, adminID, e.ID, owner, "2026-10-20", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != Uncollectible || !slices.Contains(h.notifier.actions, "external_sales_invoice_state_changed_to_uncollectible") {
		t.Errorf("uncollectible = %+v, %v", got, h.notifier.actions)
	}
	if s := sums(h.store.externalEntries[e.ID]); s[debtors] != 0 || s[payableVAT] != 0 {
		t.Errorf("uncollectible posted %v", s)
	}
	if err := journal.Balance(h.store.externalEntries[e.ID]); err != nil {
		t.Error(err)
	}
}

func TestExternalListNotesAndTheWorker(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	late := external(t, h, "2026-10-01")
	h.store.setExternalState(late.ID, Open)
	open := external(t, h, "2026-11-20")
	if _, err := h.svc.CreateExternalFromFile(ctx, adminID, owner, "scan.pdf", "application/pdf", []byte("%PDF")); err != nil {
		t.Fatal(err)
	}
	q, err := ParseExternalFilter("state:open|late", now)
	if err != nil {
		t.Fatal(err)
	}
	list, total, err := h.svc.ExternalList(ctx, adminID, owner, q)
	if err != nil || total != 2 || len(list) != 2 {
		t.Fatalf("open = %d, %v", total, err)
	}
	if q, _ := ParseExternalFilter("state:new", now); len(q.States) != 1 {
		t.Errorf("new filter = %+v", q)
	}
	for _, bad := range []string{"state:draft", "workflow_id:1", "period:never"} {
		if _, err := ParseExternalFilter(bad, now); err == nil {
			t.Errorf("filter %q accepted", bad)
		}
	}

	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	if h.store.externals[late.ID].State != Late || h.store.externals[open.ID].State != Open {
		t.Errorf("worker: late %s, open %s", h.store.externals[late.ID].State, h.store.externals[open.ID].State)
	}
	if !slices.Contains(h.notifier.actions, "external_sales_invoice_state_changed_to_late") {
		t.Errorf("events = %v", h.notifier.actions)
	}

	n, err := h.svc.AddExternalNote(ctx, adminID, open.ID, owner, notes.Input{Note: "Paid by card at the counter"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := h.svc.ExternalInvoice(ctx, adminID, open.ID, owner)
	if len(got.Notes) != 1 {
		t.Errorf("notes = %+v", got.Notes)
	}
	if err := h.svc.DeleteExternalNote(ctx, adminID, open.ID, n.ID, owner); err != nil {
		t.Fatal(err)
	}
	sum, err := h.svc.Summary(ctx, adminID, owner)
	if err != nil || sum.ToReceive.Count != 2 || sum.ToReceive.Amount != 24200 {
		t.Errorf("summary = %+v, %v", sum, err)
	}
}
