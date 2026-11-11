package purchases

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
	"github.com/google/go-cmp/cmp"
)

var ctx = context.Background()

func line(price string, rate, ledger resource.ID) sales.DetailInput {
	return sales.DetailInput{Description: ptr("Paper"), Price: ptr(price), TaxRateID: &rate, LedgerAccountID: &ledger}
}

func invoice(lines ...sales.DetailInput) Changes {
	return Changes{ContactID: ptr(supplier), Reference: ptr("INV-1"), Date: ptr("2026-11-01"), DueDate: ptr("2026-11-15"), Details: lines}
}

type posting struct {
	ledger resource.ID
	amount money.Amount
}

func postings(entries []journal.Entry) []posting {
	out := make([]posting, len(entries))
	for i, e := range entries {
		out[i] = posting{e.LedgerAccountID, e.Amount}
	}
	return out
}

func wantInvalid(t *testing.T, err error, field string) {
	t.Helper()
	var verr *resource.ValidationError
	if !errors.As(err, &verr) || len(verr.Fields[field]) == 0 {
		t.Fatalf("error = %v, want a problem with %s", err, field)
	}
}

func TestCreatePurchaseInvoiceBooksOnCreditors(t *testing.T) {
	h := newHarness(t)
	d, err := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("100", vat21, costs)))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if d.State != Open || d.EntryNumber != 1 || d.Totals.InclTax != 12100 || d.Unpaid() != 12100 {
		t.Errorf("document = %+v", d)
	}
	want := []posting{{creditors, -12100}, {costs, 10000}, {receivableVAT, 2100}}
	if diff := cmp.Diff(want, postings(h.store.entries[d.ID]), cmp.AllowUnexported(posting{})); diff != "" {
		t.Errorf("entries diff (-want +got):\n%s", diff)
	}
	if got := h.store.entries[d.ID][0]; got.DocumentType != PurchaseInvoice || got.ContactID == nil || *got.ContactID != supplier {
		t.Errorf("creditors entry = %+v", got)
	}
	if got := d.EffectiveState(time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)); got != Late {
		t.Errorf("state after the due date = %s, want late", got)
	}
}

func TestDocumentWithoutLinesIsNew(t *testing.T) {
	h := newHarness(t)
	d, err := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, Changes{Reference: ptr("scan")})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if d.State != New || len(h.store.entries[d.ID]) != 0 {
		t.Errorf("new document = %+v, entries %v", d, h.store.entries[d.ID])
	}
	// Filling it in books it.
	d, err = h.svc.Update(ctx, adminID, owner, PurchaseInvoice, d.ID, invoice(line("50", vat21, costs)))
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if d.State != Open || len(h.store.entries[d.ID]) != 3 {
		t.Errorf("booked document = %+v", d)
	}
}

func TestReceiptNeedsNoSupplierAndLinesWithoutCategoryAreUncategorized(t *testing.T) {
	h := newHarness(t)
	rate := vat21
	d, err := h.svc.Create(ctx, adminID, owner, Receipt, Changes{Details: []sales.DetailInput{{Price: ptr("10"), TaxRateID: &rate}}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if d.State != Open || *d.Lines[0].LedgerAccountID != uncategorized {
		t.Errorf("receipt = %+v", d)
	}
}

func TestReverseChargedVATIsBookedBothWays(t *testing.T) {
	h := newHarness(t)
	d, err := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("100", reverse21, costs)))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if d.Totals.InclTax != 10000 {
		t.Errorf("total = %s, want 100.0 without the reverse-charged VAT", d.Totals.InclTax)
	}
	want := []posting{{creditors, -10000}, {costs, 10000}, {receivableVAT, 2100}, {payableVAT, -2100}}
	if diff := cmp.Diff(want, postings(h.store.entries[d.ID]), cmp.AllowUnexported(posting{})); diff != "" {
		t.Errorf("entries diff (-want +got):\n%s", diff)
	}
}

func TestRevenueInvoiceBooksTheOtherWay(t *testing.T) {
	h := newHarness(t)
	c := invoice(line("100", noVAT, costs))
	c.RevenueInvoice = ptr(true)
	d, err := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, c)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	want := []posting{{creditors, 10000}, {costs, -10000}}
	if diff := cmp.Diff(want, postings(h.store.entries[d.ID]), cmp.AllowUnexported(posting{})); diff != "" {
		t.Errorf("entries diff (-want +got):\n%s", diff)
	}
	if d.Unpaid() != -10000 {
		t.Errorf("unpaid = %s, want money to receive", d.Unpaid())
	}
}

func TestFiscalAllocationMovesPrivateUse(t *testing.T) {
	h := newHarness(t)
	c := invoice(line("300", vat21, phone))
	c.FiscalAllocations = []AllocationInput{{LedgerAccountID: ptr(phone), PrivateLedgerAccountID: ptr(privateUse), Percentage: ptr("25")}}
	d, err := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, c)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got := d.FiscalAllocations[0].Price; got != 9075 {
		t.Errorf("allocation price = %s, want 90.75", got)
	}
	want := []posting{{creditors, -36300}, {phone, 30000}, {receivableVAT, 6300}, {privateUse, 9075}, {phone, -7500}, {receivableVAT, -1575}}
	if diff := cmp.Diff(want, postings(h.store.entries[d.ID]), cmp.AllowUnexported(posting{})); diff != "" {
		t.Errorf("entries diff (-want +got):\n%s", diff)
	}
}

func TestValidation(t *testing.T) {
	h := newHarness(t)
	for name, tc := range map[string]struct {
		kind  string
		c     Changes
		field string
	}{
		"sales rate":       {PurchaseInvoice, invoice(line("1", salesVAT, costs)), "details_attributes.0.tax_rate_id"},
		"unknown category": {PurchaseInvoice, invoice(line("1", vat21, creditors)), "details_attributes.0.ledger_account_id"},
		"unknown supplier": {PurchaseInvoice, Changes{ContactID: ptr(resource.ID(9))}, "contact_id"},
		"due before date":  {PurchaseInvoice, Changes{Date: ptr("2026-11-10"), DueDate: ptr("2026-11-01")}, "due_date"},
		"bad currency":     {PurchaseInvoice, Changes{Currency: ptr("euro")}, "currency"},
		"revenue receipt":  {Receipt, Changes{RevenueInvoice: ptr(true)}, "revenue_invoice"},
		"invoice payment":  {PurchaseInvoice, Changes{PaymentAccountID: ptr(cashAccount)}, "payment.financial_account_id"},
		"no supplier":      {PurchaseInvoice, Changes{Details: []sales.DetailInput{line("1", vat21, costs)}}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.svc.Create(ctx, adminID, owner, tc.kind, tc.c)
			if tc.field == "" {
				// A purchase invoice without a supplier stays new.
				if err != nil {
					t.Fatalf("Create() error = %v", err)
				}
				return
			}
			wantInvalid(t, err, tc.field)
		})
	}
	if _, err := h.svc.Create(ctx, adminID, stranger, PurchaseInvoice, Changes{}); err == nil {
		t.Error("a stranger created a document")
	}
}

func TestLockedPeriodRefusesBooking(t *testing.T) {
	h := newHarness(t)
	h.ledger.locked = "2026-11-05"
	_, err := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("1", vat21, costs)))
	wantInvalid(t, err, "date")
}

func TestPaymentsMakeTheDocumentPaid(t *testing.T) {
	h := newHarness(t)
	d, _ := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("100", vat21, costs)))
	p, err := h.svc.RegisterPayment(ctx, adminID, owner, PurchaseInvoice, d.ID, sales.PaymentInput{PaymentDate: "2026-11-10", Price: ptr("21")})
	if err != nil {
		t.Fatalf("RegisterPayment() error = %v", err)
	}
	if p.InvoiceType != PaymentType || p.Price != 2100 {
		t.Errorf("payment = %+v", p)
	}
	want := []posting{{creditors, 2100}, {toClassify, -2100}}
	if diff := cmp.Diff(want, postings(h.store.paid[p.ID]), cmp.AllowUnexported(posting{})); diff != "" {
		t.Errorf("payment entries diff (-want +got):\n%s", diff)
	}
	_, err = h.svc.RegisterPayment(ctx, adminID, owner, PurchaseInvoice, d.ID, sales.PaymentInput{PaymentDate: "2026-11-11", Price: ptr("101")})
	wantInvalid(t, err, "price")
	if _, err := h.svc.RegisterPayment(ctx, adminID, owner, PurchaseInvoice, d.ID, sales.PaymentInput{PaymentDate: "2026-11-11", ManualPaymentAction: sales.PrivatePayment}); err != nil {
		t.Fatalf("RegisterPayment() error = %v", err)
	}
	got, _ := h.svc.Document(ctx, adminID, PurchaseInvoice, d.ID, owner)
	if got.State != Paid || got.PaidAt.Format(time.DateOnly) != "2026-11-11" {
		t.Errorf("paid document = %+v", got)
	}
	if _, err := h.svc.Update(ctx, adminID, owner, PurchaseInvoice, d.ID, Changes{}); !errors.Is(err, ErrNotEditable) {
		t.Errorf("Update() of a paid document error = %v", err)
	}
	if err := h.svc.DeletePayment(ctx, adminID, owner, PurchaseInvoice, d.ID, p.ID); err != nil {
		t.Fatalf("DeletePayment() error = %v", err)
	}
	got, _ = h.svc.Document(ctx, adminID, PurchaseInvoice, d.ID, owner)
	if got.State != Open || got.Unpaid() != 2100 || h.store.paid[p.ID] != nil {
		t.Errorf("after removing a payment = %+v", got)
	}
	if err := h.svc.DeletePayment(ctx, adminID, owner, Receipt, d.ID, p.ID); !IsNotFound(err) {
		t.Errorf("DeletePayment() on the wrong kind error = %v", err)
	}
	_, err = h.svc.RegisterPayment(ctx, adminID, owner, PurchaseInvoice, d.ID, sales.PaymentInput{PaymentDate: "2026-11-11", ManualPaymentAction: sales.InvoicesSettlement})
	wantInvalid(t, err, "invoice_id")
}

func TestInvoicesSettlementSettlesAgainstAnotherDocument(t *testing.T) {
	h := newHarness(t)
	d, _ := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("100", vat21, costs)))
	credit, _ := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("-50", vat21, costs)))
	settle := func(id, other resource.ID, price string) (sales.Payment, error) {
		return h.svc.RegisterPayment(ctx, adminID, owner, PurchaseInvoice, id, sales.PaymentInput{
			PaymentDate: "2026-11-11", Price: ptr(price), ManualPaymentAction: sales.InvoicesSettlement, InvoiceID: &other,
		})
	}
	_, err := settle(d.ID, d.ID, "10")
	wantInvalid(t, err, "invoice_id")
	_, err = settle(d.ID, 999, "10")
	wantInvalid(t, err, "invoice_id")
	_, err = settle(d.ID, credit.ID, "61")
	wantInvalid(t, err, "price")

	p, err := settle(d.ID, credit.ID, "60.5")
	if err != nil {
		t.Fatalf("RegisterPayment() error = %v", err)
	}
	if p.ManualPaymentAction != sales.InvoicesSettlement || p.LinkedPaymentID == nil || p.CreditInvoiceID == nil || *p.CreditInvoiceID != credit.ID {
		t.Errorf("payment = %+v", p)
	}
	other, _ := h.svc.Document(ctx, adminID, PurchaseInvoice, credit.ID, owner)
	if other.State != Paid || len(other.Payments) != 1 || other.Payments[0].Price != -6050 || *other.Payments[0].LinkedPaymentID != p.ID {
		t.Errorf("credit after settlement = %+v", other)
	}
	if diff := cmp.Diff([]posting{{creditors, 6050}, {settlements, -6050}}, postings(h.store.paid[p.ID]), cmp.AllowUnexported(posting{})); diff != "" {
		t.Errorf("payment entries diff (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]posting{{creditors, -6050}, {settlements, 6050}}, postings(h.store.paid[*p.LinkedPaymentID]), cmp.AllowUnexported(posting{})); diff != "" {
		t.Errorf("linked payment entries diff (-want +got):\n%s", diff)
	}

	// Removing one half takes the other along.
	if err := h.svc.DeletePayment(ctx, adminID, owner, PurchaseInvoice, credit.ID, *p.LinkedPaymentID); err != nil {
		t.Fatalf("DeletePayment() error = %v", err)
	}
	got, _ := h.svc.Document(ctx, adminID, PurchaseInvoice, d.ID, owner)
	other, _ = h.svc.Document(ctx, adminID, PurchaseInvoice, credit.ID, owner)
	if len(got.Payments) != 0 || len(other.Payments) != 0 || other.State != Open || h.store.paid[p.ID] != nil {
		t.Errorf("after removing a settlement: %+v / %+v", got, other)
	}
}

func TestReceiptPaidFromAFinancialAccountOnCreation(t *testing.T) {
	h := newHarness(t)
	c := Changes{Date: ptr("2026-11-10"), Details: []sales.DetailInput{line("10", vat21, costs)}, PaymentAccountID: ptr(cashAccount)}
	d, err := h.svc.Create(ctx, adminID, owner, Receipt, c)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if d.State != Paid || len(d.Payments) != 1 || *d.Payments[0].FinancialAccountID != cashAccount {
		t.Fatalf("receipt = %+v", d)
	}
	want := []posting{{creditors, 1210}, {cashLedger, -1210}}
	if diff := cmp.Diff(want, postings(h.store.paid[d.Payments[0].ID]), cmp.AllowUnexported(posting{})); diff != "" {
		t.Errorf("payment entries diff (-want +got):\n%s", diff)
	}
}

func TestBankTransactionsPayDocuments(t *testing.T) {
	h := newHarness(t)
	d, _ := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("100", vat21, costs)))
	mutation := resource.ID(900)
	h.bank.open[mutation] = -12100
	p, err := h.svc.RegisterPayment(ctx, adminID, owner, PurchaseInvoice, d.ID, sales.PaymentInput{PaymentDate: "2026-11-11", FinancialMutationID: &mutation})
	if err != nil {
		t.Fatalf("RegisterPayment() with a transaction error = %v", err)
	}
	if p.Price != 12100 || *p.FinancialMutationID != mutation || h.bank.open[mutation] != 0 || h.store.paid[p.ID] != nil {
		t.Errorf("bank payment = %+v, open %s", p, h.bank.open[mutation])
	}
	got, _ := h.svc.Document(ctx, adminID, PurchaseInvoice, d.ID, owner)
	if got.State != Paid {
		t.Errorf("state = %s, want paid", got.State)
	}
	if err := h.svc.Delete(ctx, adminID, owner, PurchaseInvoice, d.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if h.bank.open[mutation] != -12100 {
		t.Errorf("transaction open = %s, want it unlinked", h.bank.open[mutation])
	}
	if _, err := h.svc.Document(ctx, adminID, PurchaseInvoice, d.ID, owner); !errors.Is(err, ErrNotFound) {
		t.Errorf("Document() after delete error = %v", err)
	}
}

func TestRegisterBankPaymentChecks(t *testing.T) {
	h := newHarness(t)
	d, _ := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("100", noVAT, costs)))
	_, err := h.svc.RegisterBankPayment(ctx, adminID, owner, "SalesInvoice", d.ID, 1, bankAccount, now, 10000, nil)
	wantInvalid(t, err, "booking_type")
	_, err = h.svc.RegisterBankPayment(ctx, adminID, owner, PaymentType, 12345, 1, bankAccount, now, 10000, nil)
	wantInvalid(t, err, "booking_id")
	// A smaller transaction pays part.
	id, err := h.svc.RegisterBankPayment(ctx, adminID, owner, PaymentType, d.ID, 1, bankAccount, now, 4000, nil)
	if err != nil {
		t.Fatalf("RegisterBankPayment() error = %v", err)
	}
	p, _ := h.store.Payment(ctx, adminID, id)
	if p.Price != 4000 {
		t.Errorf("payment = %s, want 40.0", p.Price)
	}
	_, err = h.svc.RegisterBankPayment(ctx, adminID, owner, PaymentType, d.ID, 2, bankAccount, now, -500, nil)
	wantInvalid(t, err, "price_base")
	if err := h.svc.RemoveBankPayment(ctx, adminID, owner, id); err != nil {
		t.Fatalf("RemoveBankPayment() error = %v", err)
	}
	// A payment waiting for its transaction is taken over by it.
	if _, err := h.svc.RegisterPayment(ctx, adminID, owner, PurchaseInvoice, d.ID, sales.PaymentInput{PaymentDate: "2026-11-11", ManualPaymentAction: sales.BankTransfer}); err != nil {
		t.Fatal(err)
	}
	id, err = h.svc.RegisterBankPayment(ctx, adminID, owner, PaymentType, d.ID, 3, bankAccount, now, 10000, nil)
	if err != nil {
		t.Fatalf("RegisterBankPayment() over a waiting payment error = %v", err)
	}
	if p, _ := h.store.Payment(ctx, adminID, id); p.FinancialMutationID == nil || h.store.paid[id] != nil {
		t.Errorf("taken over payment = %+v, entries %v", p, h.store.paid[id])
	}
}

func TestListFilterNotesAndAttachments(t *testing.T) {
	h := newHarness(t)
	a, _ := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("100", vat21, costs)))
	r, _ := h.svc.Create(ctx, adminID, owner, Receipt, Changes{Date: ptr("2026-11-01")})
	q, err := ParseFilter("state:open,period:this_month", now)
	if err != nil {
		t.Fatal(err)
	}
	list, total, err := h.svc.List(ctx, adminID, owner, PurchaseInvoice, q)
	if err != nil || total != 1 || list[0].ID != a.ID {
		t.Errorf("List() = %v, %d, %v", list, total, err)
	}
	if list, _, _ := h.svc.List(ctx, adminID, owner, Receipt, Query{}); len(list) != 1 || list[0].ID != r.ID {
		t.Errorf("receipts = %v", list)
	}
	if _, err := h.svc.Document(ctx, adminID, Receipt, a.ID, owner); !errors.Is(err, ErrNotFound) {
		t.Errorf("a purchase invoice as receipt error = %v", err)
	}
	n, err := h.svc.AddNote(ctx, adminID, owner, PurchaseInvoice, a.ID, notes.Input{Note: "Check the amount"})
	if err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
	if _, err := h.svc.AddAttachment(ctx, adminID, owner, PurchaseInvoice, a.ID, "bill.pdf", "application/pdf", []byte("%PDF")); err != nil {
		t.Fatalf("AddAttachment() error = %v", err)
	}
	got, _ := h.svc.Document(ctx, adminID, PurchaseInvoice, a.ID, owner)
	if len(got.Notes) != 1 || len(got.Attachments) != 1 {
		t.Errorf("document notes %v, attachments %v", got.Notes, got.Attachments)
	}
	if err := h.svc.DeleteNote(ctx, adminID, owner, PurchaseInvoice, a.ID, n.ID); err != nil {
		t.Fatalf("DeleteNote() error = %v", err)
	}
	byID, err := h.svc.DocumentsByID(ctx, adminID, owner, PurchaseInvoice, []resource.ID{a.ID, r.ID})
	if err != nil || len(byID) != 1 {
		t.Errorf("DocumentsByID() = %v, %v", byID, err)
	}
	up, err := h.svc.CreateFromFile(ctx, adminID, owner, Receipt, "scan.png", "image/png", []byte("png"))
	if err != nil || up.State != New || up.Origin != OriginUpload || len(up.Attachments) != 1 {
		t.Errorf("CreateFromFile() = %+v, %v", up, err)
	}
}

func TestParseFilter(t *testing.T) {
	q, err := ParseFilter("", now)
	if err != nil || q.Period == nil || q.Period.From.Year() != 2026 {
		t.Errorf("default filter = %+v, %v", q, err)
	}
	q, err = ParseFilter("state:open|late,reference:A|B,contact_id:5,ledger_account_id:6,attachment:with,updated_after:2026-01-01T00:00:00Z,recurring:disabled,automatic_saving:manual", now)
	if err != nil || len(q.States) != 2 || len(q.References) != 2 || *q.ContactID != 5 || *q.LedgerAccountID != 6 || q.Attachment != "with" || q.UpdatedAfter == nil || q.None || q.Period != nil {
		t.Errorf("filter = %+v, %v", q, err)
	}
	if q, _ := ParseFilter("recurring:enabled", now); !q.None {
		t.Error("recurring:enabled selects documents")
	}
	for _, bad := range []string{"state:draft", "period:x", "color:red", "attachment:maybe", "contact_id:x", "updated_after:yesterday", "nocolon"} {
		if _, err := ParseFilter(bad, now); err == nil {
			t.Errorf("ParseFilter(%q) accepted", bad)
		}
	}
}

func TestGeneralAndTypelessDocuments(t *testing.T) {
	h := newHarness(t)
	g, err := h.svc.CreateGeneral(ctx, adminID, owner, GeneralChanges{Reference: ptr("Lease"), ContactID: ptr(supplier), ReminderDate: ptr("2026-12-01"), ReminderMessage: ptr("Renew")})
	if err != nil {
		t.Fatalf("CreateGeneral() error = %v", err)
	}
	if g.State != Saved || g.ContactID != supplier || g.ReminderDate == nil || g.Date != time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC) {
		t.Errorf("general document = %+v", g)
	}
	g, err = h.svc.UpdateGeneral(ctx, adminID, g.ID, owner, GeneralChanges{RemoveContact: true, DueDate: ptr("2026-12-31")})
	if err != nil || g.ContactID != 0 || g.DueDate == nil {
		t.Errorf("UpdateGeneral() = %+v, %v", g, err)
	}
	_, err = h.svc.UpdateGeneral(ctx, adminID, g.ID, owner, GeneralChanges{Date: ptr("soon")})
	wantInvalid(t, err, "date")
	if _, err := h.svc.AddGeneralNote(ctx, adminID, g.ID, owner, notes.Input{Note: "Signed"}); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.DeleteGeneral(ctx, adminID, g.ID, owner); err != nil {
		t.Fatalf("DeleteGeneral() error = %v", err)
	}

	tl, err := h.svc.Upload(ctx, adminID, owner, "bill.pdf", "application/pdf", []byte("%PDF"))
	if err != nil || tl.State != New || tl.Origin != OriginUpload || len(tl.Attachments) != 1 {
		t.Fatalf("Upload() = %+v, %v", tl, err)
	}
	sum, err := h.svc.Summary(ctx, adminID, owner)
	if err != nil || sum.ToProcess != 1 || sum.Currency != "EUR" {
		t.Errorf("Summary() = %+v, %v", sum, err)
	}
	_, err = h.svc.CreateTypeless(ctx, adminID, owner, TypelessChanges{ContactID: ptr(resource.ID(9))})
	wantInvalid(t, err, "contact_id")
	if err := h.svc.DeleteTypeless(ctx, adminID, tl.ID, owner); err != nil {
		t.Fatalf("DeleteTypeless() error = %v", err)
	}
}

func TestPayingByCreditTransferBatch(t *testing.T) {
	h := newHarness(t)
	d, _ := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("100", vat21, costs)))
	_, err := h.svc.Pay(ctx, adminID, owner, PayInput{DocumentID: d.ID, FinancialAccountID: cashAccount})
	wantInvalid(t, err, "financial_account_id")
	tx, err := h.svc.Pay(ctx, adminID, owner, PayInput{DocumentID: d.ID, FinancialAccountID: bankAccount})
	if err != nil {
		t.Fatalf("Pay() error = %v", err)
	}
	if tx.State() != Open || tx.Amount != 12100 || tx.IBAN != "NL44RABO0123456789" || tx.IBANName != "Supplier B.V." {
		t.Errorf("transaction = %+v", tx)
	}
	b, err := h.svc.CreateBatch(ctx, adminID, owner, BatchInput{FinancialAccountID: bankAccount, ExecutionDate: "2026-11-12"})
	if err != nil {
		t.Fatalf("CreateBatch() error = %v", err)
	}
	got, _ := h.svc.Document(ctx, adminID, PurchaseInvoice, d.ID, owner)
	if got.EffectiveState(now) != PendingPayment {
		t.Errorf("document state = %s, want pending_payment", got.EffectiveState(now))
	}
	if err := h.svc.DeleteTransaction(ctx, adminID, tx.ID, owner); err == nil {
		t.Error("deleted a batched transaction")
	}
	file, b, err := h.svc.DownloadBatch(ctx, adminID, b.ID, owner)
	if err != nil {
		t.Fatalf("DownloadBatch() error = %v", err)
	}
	for _, want := range []string{"pain.001.001.09", "<IBAN>NL91ABNA0417164300</IBAN>", "<IBAN>NL44RABO0123456789</IBAN>", "121.00", "<Ustrd>INV-1</Ustrd>", "<Dt>2026-11-12</Dt>"} {
		if !strings.Contains(string(file), want) {
			t.Errorf("file lacks %q", want)
		}
	}
	if b.State != Exported {
		t.Errorf("batch state = %s, want exported", b.State)
	}
	q, _ := ParseTransactionFilter("purchase_transaction_batch_id:"+b.ID.String(), now)
	list, _, err := h.svc.Transactions(ctx, adminID, owner, q)
	if err != nil || len(list) != 1 || list[0].State() != PendingPayment {
		t.Errorf("batch transactions = %+v, %v", list, err)
	}
	// Once the bank pays, the transaction is paid.
	mutation := resource.ID(901)
	h.bank.open[mutation] = -12100
	if _, err := h.svc.RegisterPayment(ctx, adminID, owner, PurchaseInvoice, d.ID, sales.PaymentInput{PaymentDate: "2026-11-11", FinancialMutationID: &mutation}); err != nil {
		t.Fatal(err)
	}
	if tx, _ := h.svc.Transaction(ctx, adminID, tx.ID, owner); tx.State() != Paid {
		t.Errorf("transaction state = %s, want paid", tx.State())
	}
}

func TestImportCreditTransferMatchesDocuments(t *testing.T) {
	h := newHarness(t)
	d, _ := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("100", vat21, costs)))
	file := `<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.001.001.09"><CstmrCdtTrfInitn>
<GrpHdr><MsgId>M1</MsgId><CreDtTm>2026-11-11T10:00:00</CreDtTm><NbOfTxs>2</NbOfTxs><CtrlSum>131.00</CtrlSum><InitgPty><Nm>Example Shop B.V.</Nm></InitgPty></GrpHdr>
<PmtInf><PmtInfId>M1-STD</PmtInfId><PmtMtd>TRF</PmtMtd><NbOfTxs>2</NbOfTxs><CtrlSum>131.00</CtrlSum><ReqdExctnDt><Dt>2026-11-12</Dt></ReqdExctnDt>
<Dbtr><Nm>Example Shop B.V.</Nm></Dbtr><DbtrAcct><Id><IBAN>NL91ABNA0417164300</IBAN></Id></DbtrAcct><DbtrAgt><FinInstnId><BICFI>ABNANL2A</BICFI></FinInstnId></DbtrAgt>
<CdtTrfTxInf><PmtId><EndToEndId>E1</EndToEndId></PmtId><Amt><InstdAmt Ccy="EUR">121.00</InstdAmt></Amt><Cdtr><Nm>Supplier B.V.</Nm></Cdtr><CdtrAcct><Id><IBAN>NL44RABO0123456789</IBAN></Id></CdtrAcct><RmtInf><Ustrd>INV-1</Ustrd></RmtInf></CdtTrfTxInf>
<CdtTrfTxInf><PmtId><EndToEndId>E2</EndToEndId></PmtId><Amt><InstdAmt Ccy="EUR">10.00</InstdAmt></Amt><Cdtr><Nm>Other</Nm></Cdtr><CdtrAcct><Id><IBAN>NL20INGB0001234567</IBAN></Id></CdtrAcct></CdtTrfTxInf>
</PmtInf></CstmrCdtTrfInitn></Document>`
	batches, err := h.svc.ImportCreditTransfer(ctx, adminID, owner, ImportInput{File: []byte(file)})
	if err != nil {
		t.Fatalf("ImportCreditTransfer() error = %v", err)
	}
	if len(batches) != 1 || batches[0].Identifier != "M1-STD" || batches[0].FinancialAccountID != bankAccount || batches[0].State != Open {
		t.Fatalf("batches = %+v", batches)
	}
	list, _, _ := h.svc.Transactions(ctx, adminID, owner, TransactionQuery{BatchID: &batches[0].ID})
	paying := slices.IndexFunc(list, func(t Transaction) bool { return t.PayableID != nil && *t.PayableID == d.ID })
	if len(list) != 2 || paying < 0 {
		t.Errorf("transactions = %+v", list)
	}
	_, err = h.svc.ImportCreditTransfer(ctx, adminID, owner, ImportInput{File: []byte("nope")})
	wantInvalid(t, err, "file")
	_, err = h.svc.ImportCreditTransfer(ctx, adminID, owner, ImportInput{File: []byte(strings.ReplaceAll(file, "NL91ABNA0417164300", "NL20INGB0001234567"))})
	wantInvalid(t, err, "financial_account_id")
}

func TestParseTransactionFilter(t *testing.T) {
	q, err := ParseTransactionFilter("", now)
	if err != nil || !q.Unbatched {
		t.Errorf("default = %+v, %v", q, err)
	}
	q, err = ParseTransactionFilter("state:open|paid,unbatched:false,period:this_year", now)
	if err != nil || q.Unbatched || len(q.States) != 2 || q.Period == nil {
		t.Errorf("filter = %+v, %v", q, err)
	}
	for _, bad := range []string{"state:x", "unbatched:maybe", "unbatched:true,purchase_transaction_batch_id:1", "foo:bar"} {
		if _, err := ParseTransactionFilter(bad, now); err == nil {
			t.Errorf("ParseTransactionFilter(%q) accepted", bad)
		}
	}
}

func TestCreateBankDocument(t *testing.T) {
	h := newHarness(t)
	day := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	in := banking.NewDocument{Kind: PurchaseInvoice, Date: day, Amount: 2550, Description: "Hosting November", ContraName: "Hosting", ContraAccount: "NL44 RABO 0123 4567 89", ProjectID: ptr(falcon)}
	id, err := h.svc.CreateBankDocument(ctx, adminID, owner, in)
	if err != nil {
		t.Fatalf("CreateBankDocument() error = %v", err)
	}
	d, _ := h.svc.Document(ctx, adminID, PurchaseInvoice, id, owner)
	if d.State != Open || d.ContactID != supplier || !d.Date.Equal(day) || d.Total() != 2550 || len(d.Lines) != 1 ||
		d.Lines[0].TaxRateID != noVAT || *d.Lines[0].ProjectID != falcon || *d.Lines[0].LedgerAccountID != uncategorized || d.Lines[0].Description != "Hosting November" {
		t.Errorf("document = %+v", d)
	}
	// Without a contact with the IBAN, only a receipt can be made.
	in.ContraAccount, in.ProjectID = "NL91ABNA0417164300", nil
	_, err = h.svc.CreateBankDocument(ctx, adminID, owner, in)
	wantInvalid(t, err, "booking_type")
	in.Kind = Receipt
	id, err = h.svc.CreateBankDocument(ctx, adminID, owner, in)
	if err != nil {
		t.Fatalf("CreateBankDocument() of a receipt error = %v", err)
	}
	if d, _ := h.svc.Document(ctx, adminID, Receipt, id, owner); d.State != Open || d.ContactID != 0 {
		t.Errorf("receipt = %+v", d)
	}
}

func TestPayables(t *testing.T) {
	h := newHarness(t)
	inv, _ := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("100", vat21, costs)))
	tx, err := h.svc.Pay(ctx, adminID, owner, PayInput{DocumentID: inv.ID, FinancialAccountID: bankAccount})
	if err != nil {
		t.Fatalf("Pay() error = %v", err)
	}
	got, err := h.svc.Payables(ctx, adminID, owner, "PurchaseTransaction", tx.ID)
	if err != nil || len(got) != 1 || got[0].DocumentID != inv.ID || got[0].Amount != 12100 {
		t.Errorf("Payables() = %+v, %v", got, err)
	}
	_, err = h.svc.Payables(ctx, adminID, owner, "PurchaseTransaction", 999)
	wantInvalid(t, err, "booking_id")
	b, err := h.svc.CreateBatch(ctx, adminID, owner, BatchInput{FinancialAccountID: bankAccount})
	if err != nil {
		t.Fatalf("CreateBatch() error = %v", err)
	}
	if got, err := h.svc.Payables(ctx, adminID, owner, "PurchaseTransactionBatch", b.ID); err != nil || len(got) != 1 {
		t.Errorf("Payables() of a batch = %+v, %v", got, err)
	}
	if _, err := h.svc.RegisterPayment(ctx, adminID, owner, PurchaseInvoice, inv.ID, sales.PaymentInput{PaymentDate: "2026-11-11"}); err != nil {
		t.Fatal(err)
	}
	_, err = h.svc.Payables(ctx, adminID, owner, "PurchaseTransactionBatch", b.ID)
	wantInvalid(t, err, "booking_id")
}
