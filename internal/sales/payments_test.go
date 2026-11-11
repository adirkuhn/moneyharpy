package sales

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// sent makes an invoice of 100 + 21% VAT = 121.00 sent by hand on day.
func sent(t *testing.T, h harness, day string) Invoice {
	t.Helper()
	inv := draft(t, h, line("100", "1", vat21, revenue))
	out, err := h.svc.Send(context.Background(), adminID, inv.ID, owner, SendInput{DeliveryMethod: "Manual", InvoiceDate: day})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sums(entries []journal.Entry) map[resource.ID]money.Amount {
	out := map[resource.ID]money.Amount{}
	for _, e := range entries {
		out[e.LedgerAccountID] += e.Amount
	}
	return out
}

func TestRegisterPaymentPaysTheInvoice(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := sent(t, h, "2026-11-01")
	part, err := h.svc.RegisterPayment(ctx, adminID, inv.ID, owner, PaymentInput{PaymentDate: "2026-11-05", Price: ptr("21")})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := h.svc.Invoice(ctx, adminID, inv.ID, owner)
	if got.State != Open || got.Unpaid() != 10000 || got.PaidAt != nil {
		t.Errorf("after part = %s, unpaid %s", got.State, got.Unpaid())
	}
	if s := sums(h.store.paid[part.ID]); s[toClassify] != 2100 || s[debtors] != -2100 {
		t.Errorf("part posted %v", s)
	}
	rest, err := h.svc.RegisterPayment(ctx, adminID, inv.ID, owner, PaymentInput{PaymentDate: "2026-11-10T10:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ = h.svc.Invoice(ctx, adminID, inv.ID, owner)
	if got.State != Paid || got.PaidAt.Format(time.DateOnly) != "2026-11-10" || rest.Price != 10000 || rest.UserID != owner || len(got.Payments) != 2 {
		t.Errorf("paid = %+v, rest %+v", got, rest)
	}
	if !slices.Contains(h.notifier.actions, "sales_invoice_state_changed_to_paid") {
		t.Errorf("events = %v", h.notifier.actions)
	}
	if _, err := h.svc.RegisterPayment(ctx, adminID, inv.ID, owner, PaymentInput{PaymentDate: "2026-11-10"}); !errors.Is(err, ErrNotEditable) {
		t.Errorf("paid twice: %v", err)
	}
	if p, err := h.svc.Payment(ctx, adminID, part.ID, owner); err != nil || p.InvoiceID != inv.ID || p.Price != 2100 {
		t.Errorf("Payment() = %+v, %v", p, err)
	}
	if _, err := h.svc.Payment(ctx, adminID, 9999, owner); !errors.Is(err, ErrPaymentNotFound) {
		t.Errorf("unknown Payment() = %v", err)
	}
	if err := h.svc.DeletePayment(ctx, adminID, inv.ID, rest.ID, owner); err != nil {
		t.Fatal(err)
	}
	got, _ = h.svc.Invoice(ctx, adminID, inv.ID, owner)
	if got.State != Open || got.PaidAt != nil || len(got.Payments) != 1 || h.store.paid[rest.ID] != nil {
		t.Errorf("after delete = %+v", got)
	}
	if err := h.svc.DeletePayment(ctx, adminID, inv.ID, 9999, owner); !errors.Is(err, ErrPaymentNotFound) {
		t.Errorf("unknown payment = %v", err)
	}
}

func TestRegisterPaymentValidates(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := sent(t, h, "2026-11-01")
	for name, in := range map[string]PaymentInput{
		"price":                 {PaymentDate: "2026-11-05", Price: ptr("200")},
		"payment_date":          {PaymentDate: "2026-12-01"},
		"manual_payment_action": {PaymentDate: "2026-11-05", ManualPaymentAction: "barter"},
		"ledger_account_id":     {PaymentDate: "2026-11-05", ManualPaymentAction: BalanceSettlement, LedgerAccountID: ptr(revenue)},
		"invoice_id":            {PaymentDate: "2026-11-05", ManualPaymentAction: InvoicesSettlement},
	} {
		if _, err := h.svc.RegisterPayment(ctx, adminID, inv.ID, owner, in); fieldsOf(t, err)[name] == nil {
			t.Errorf("no error for %s: %v", name, err)
		}
	}
	if _, err := h.svc.RegisterPayment(ctx, adminID, inv.ID, owner, PaymentInput{PaymentDate: "2026-11-05", Price: ptr("-1")}); fieldsOf(t, err)["price"] == nil {
		t.Error("negative payment on an invoice accepted")
	}
	h.ledger.locked = "2026-11-06"
	if _, err := h.svc.RegisterPayment(ctx, adminID, inv.ID, owner, PaymentInput{PaymentDate: "2026-11-05"}); fieldsOf(t, err)["payment_date"] == nil {
		t.Error("paid in the locked period")
	}
	d := draft(t, h, line("10", "1", vat21, revenue))
	if _, err := h.svc.RegisterPayment(ctx, adminID, d.ID, owner, PaymentInput{PaymentDate: "2026-11-07"}); !errors.Is(err, ErrNotEditable) {
		t.Errorf("paid a draft: %v", err)
	}
}

func TestPaymentActionsChooseTheContraAccount(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	for _, c := range []struct {
		in   PaymentInput
		want resource.ID
	}{
		{PaymentInput{ManualPaymentAction: CashPayment}, cashLedger},
		{PaymentInput{ManualPaymentAction: CashPayment, FinancialAccountID: ptr(bankAccount)}, bankLedger},
		{PaymentInput{ManualPaymentAction: PrivatePayment}, privateDeposits},
		{PaymentInput{ManualPaymentAction: RoundingError}, rounding},
		{PaymentInput{ManualPaymentAction: BalanceSettlement, LedgerAccountID: ptr(loan)}, loan},
		{PaymentInput{ManualPaymentAction: PaymentWithoutProof}, toClassify},
	} {
		inv := sent(t, h, "2026-11-01")
		c.in.PaymentDate = "2026-11-02"
		p, err := h.svc.RegisterPayment(ctx, adminID, inv.ID, owner, c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.in.ManualPaymentAction, err)
		}
		if s := sums(h.store.paid[p.ID]); s[c.want] != 12100 || s[debtors] != -12100 || *p.LedgerAccountID != c.want {
			t.Errorf("%s posted %v", c.in.ManualPaymentAction, s)
		}
	}
}

func TestBankPayments(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := sent(t, h, "2026-11-01")
	const mutation resource.ID = 900
	h.bank.open[mutation] = 5000
	p, err := h.svc.RegisterPayment(ctx, adminID, inv.ID, owner, PaymentInput{FinancialMutationID: ptr(mutation)})
	if err != nil {
		t.Fatal(err)
	}
	// The transaction has 50.00 open, less than the invoice; it pays that.
	if p.Price != 5000 || *p.FinancialMutationID != mutation || *p.FinancialAccountID != bankAccount || h.store.paid[p.ID] != nil || h.bank.open[mutation] != 0 {
		t.Errorf("bank payment = %+v, open %s", p, h.bank.open[mutation])
	}
	if _, err := h.svc.RegisterPayment(ctx, adminID, inv.ID, owner, PaymentInput{FinancialMutationID: ptr(mutation)}); fieldsOf(t, err)["price_base"] == nil {
		t.Errorf("linked a transaction with nothing open: %v", err)
	}
	if err := h.svc.DeletePayment(ctx, adminID, inv.ID, p.ID, owner); err != nil {
		t.Fatal(err)
	}
	if got, _ := h.svc.Invoice(ctx, adminID, inv.ID, owner); len(got.Payments) != 0 || h.bank.open[mutation] != 5000 {
		t.Errorf("after unlink = %+v", got.Payments)
	}

	// A payment registered by hand waits for its transaction, which takes it over.
	manual, _ := h.svc.RegisterPayment(ctx, adminID, inv.ID, owner, PaymentInput{PaymentDate: "2026-11-03", ManualPaymentAction: BankTransfer})
	h.bank.open[901] = 12100
	id, err := h.bank.LinkInvoicePayment(ctx, adminID, 901, owner, EntityType, inv.ID, nil)
	if err != nil || id != manual.ID {
		t.Fatalf("take over = %d, %v", id, err)
	}
	got, _ := h.svc.Invoice(ctx, adminID, inv.ID, owner)
	if got.State != Paid || len(got.Payments) != 1 || got.Payments[0].FinancialMutationID == nil || h.store.paid[manual.ID] != nil {
		t.Errorf("taken over = %+v", got)
	}
}

func TestSettleCreditInvoice(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := sent(t, h, "2026-11-01")
	credit, _ := h.svc.Credit(ctx, adminID, inv.ID, owner, nil)
	if _, err := h.svc.SettleCredit(ctx, adminID, inv.ID, owner); fieldsOf(t, err)["base"] == nil {
		t.Error("settled an invoice that is not a credit invoice")
	}
	if _, err := h.svc.Send(ctx, adminID, credit.ID, owner, SendInput{DeliveryMethod: "Manual"}); err != nil {
		t.Fatal(err)
	}
	got, err := h.svc.SettleCredit(ctx, adminID, credit.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	orig, _ := h.svc.Invoice(ctx, adminID, inv.ID, owner)
	if got.State != Paid || orig.State != Paid || got.Payments[0].Price != -12100 || orig.Payments[0].Price != 12100 {
		t.Fatalf("credit %s %+v, original %s %+v", got.State, got.Payments, orig.State, orig.Payments)
	}
	if p := orig.Payments[0]; p.CreditInvoiceID == nil || *p.CreditInvoiceID != credit.ID || *p.LinkedPaymentID != got.Payments[0].ID || *p.LedgerAccountID != settlements {
		t.Errorf("original's payment = %+v", p)
	}
	all := append(slices.Clone(h.store.paid[got.Payments[0].ID]), h.store.paid[orig.Payments[0].ID]...)
	if s := sums(all); s[settlements] != 0 || s[debtors] != 0 || len(all) != 4 {
		t.Errorf("settlement posted %v", s)
	}
	// Deleting one half takes the other along.
	if err := h.svc.DeletePayment(ctx, adminID, inv.ID, orig.Payments[0].ID, owner); err != nil {
		t.Fatal(err)
	}
	got, _ = h.svc.Invoice(ctx, adminID, credit.ID, owner)
	orig, _ = h.svc.Invoice(ctx, adminID, inv.ID, owner)
	if got.State != Open || orig.State != Open || len(got.Payments)+len(orig.Payments) != 0 {
		t.Errorf("after delete: credit %s, original %s", got.State, orig.State)
	}
}

func TestMarkDubiousThenPaid(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := sent(t, h, "2026-09-01")
	if _, err := h.svc.MarkDubious(ctx, adminID, inv.ID, owner, "2026-09-01"); fieldsOf(t, err)["dubious_date"] == nil {
		t.Error("dubious on the invoice date")
	}
	got, err := h.svc.MarkDubious(ctx, adminID, inv.ID, owner, "2026-10-15")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != Late || got.MarkedDubiousOn.Format(time.DateOnly) != "2026-10-15" {
		t.Errorf("dubious = %+v", got)
	}
	if s := sums(h.store.entries[inv.ID]); s[allocation] != 10000 || s[provision] != -10000 || s[debtors] != 12100 {
		t.Errorf("dubious posted %v", s)
	}
	if _, err := h.svc.MarkDubious(ctx, adminID, inv.ID, owner, ""); !errors.Is(err, ErrNotEditable) {
		t.Errorf("dubious twice: %v", err)
	}
	if _, err := h.svc.RegisterPayment(ctx, adminID, inv.ID, owner, PaymentInput{PaymentDate: "2026-11-01"}); err != nil {
		t.Fatal(err)
	}
	// The payment released the provision.
	if s := sums(h.store.entries[inv.ID]); s[allocation] != 0 || s[provision] != 0 {
		t.Errorf("paid dubious posted %v", s)
	}
	if err := journal.Balance(h.store.entries[inv.ID]); err != nil {
		t.Error(err)
	}
}

func TestMarkUncollectible(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := sent(t, h, "2026-09-01")
	if _, err := h.svc.RegisterPayment(ctx, adminID, inv.ID, owner, PaymentInput{PaymentDate: "2026-09-10", Price: ptr("60.50")}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.MarkUncollectible(ctx, adminID, inv.ID, owner, "2026-10-01", "cash"); fieldsOf(t, err)["book_method"] == nil {
		t.Error("unknown book method accepted")
	}
	got, err := h.svc.MarkUncollectible(ctx, adminID, inv.ID, owner, "2026-10-01", "revenue")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != Uncollectible || got.MarkedUncollectibleOn == nil || !slices.Contains(h.notifier.actions, "sales_invoice_state_changed_to_uncollectible") {
		t.Errorf("uncollectible = %+v", got)
	}
	// 60.50 open: 50.00 revenue written off and 10.50 VAT back.
	s := sums(h.store.entries[inv.ID])
	if s[uncollectible] != 5000 || s[payableVAT] != -2100+1050 || s[debtors] != 12100-6050 {
		t.Errorf("uncollectible posted %v", s)
	}
	if err := h.svc.DeletePayment(ctx, adminID, inv.ID, got.Payments[0].ID, owner); !errors.Is(err, ErrNotEditable) {
		t.Errorf("deleted a payment of an uncollectible invoice: %v", err)
	}
	if _, err := h.svc.MarkUncollectible(ctx, adminID, inv.ID, owner, "", ""); !errors.Is(err, ErrNotEditable) {
		t.Errorf("uncollectible twice: %v", err)
	}
}

func TestUncollectibleAfterDubious(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := sent(t, h, "2026-09-01")
	if _, err := h.svc.MarkDubious(ctx, adminID, inv.ID, owner, "2026-10-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.MarkUncollectible(ctx, adminID, inv.ID, owner, "2026-10-01", ""); fieldsOf(t, err)["uncollectible_date"] == nil {
		t.Error("uncollectible on the dubious date")
	}
	if _, err := h.svc.MarkUncollectible(ctx, adminID, inv.ID, owner, "2026-10-20", ""); err != nil {
		t.Fatal(err)
	}
	s := sums(h.store.entries[inv.ID])
	if s[allocation] != 10000 || s[provision] != 0 || s[uncollectible] != 0 || s[payableVAT] != 0 || s[debtors] != 0 {
		t.Errorf("posted %v", s)
	}
}

func TestRemindersAndTheBackgroundJob(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	late := sent(t, h, "2026-10-01") // due 2026-10-15, first reminder 2026-10-22
	if late.NextReminder == nil || late.NextReminder.Format(time.DateOnly) != "2026-10-22" || late.State != Late {
		t.Fatalf("late = %s, next %v", late.State, late.NextReminder)
	}
	open := sent(t, h, "2026-11-10")
	if open.NextReminder.Format(time.DateOnly) != "2026-12-01" {
		t.Errorf("open next = %v", open.NextReminder)
	}
	later := draft(t, h, line("10", "1", vat21, revenue))
	if _, err := h.svc.Send(ctx, adminID, later.ID, owner, SendInput{Scheduled: true, InvoiceDate: "2026-11-12", EmailMessage: "Hello {invoice_id}"}); err != nil {
		t.Fatal(err)
	}
	h.mailer.sent = nil

	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := h.svc.Invoice(ctx, adminID, late.ID, owner)
	if got.State != Reminded || got.ReminderCount != 1 || got.NextReminder.Format(time.DateOnly) != "2026-11-05" {
		t.Errorf("reminded = %s %d %v", got.State, got.ReminderCount, got.NextReminder)
	}
	if h.store.invoices[late.ID].State != Reminded || len(h.mailer.sent) != 1 {
		t.Fatalf("mails = %d", len(h.mailer.sent))
	}
	m := h.mailer.sent[0]
	if !strings.Contains(m.Subject, late.InvoiceID) || !strings.Contains(m.Body, "Please pay "+late.InvoiceID) || !strings.Contains(m.Body, "€ 121,00") || len(m.Files) != 1 {
		t.Errorf("reminder = %q / %q", m.Subject, m.Body)
	}
	for _, want := range []string{"sales_invoice_state_changed_to_late", "sales_invoice_send_reminder_email", "sales_invoice_state_changed_to_reminded"} {
		if !slices.Contains(h.notifier.actions, want) {
			t.Errorf("no %s in %v", want, h.notifier.actions)
		}
	}
	// The second reminder is not sent automatically; nothing happens again.
	if err := h.svc.Work(ctx); err != nil || len(h.mailer.sent) != 1 {
		t.Errorf("second round sent %d, %v", len(h.mailer.sent), err)
	}

	// The next day the scheduled invoice goes out with its own message.
	*h.clock = now.AddDate(0, 0, 1)
	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ = h.svc.Invoice(ctx, adminID, later.ID, owner)
	if got.State != Open || got.InvoiceID == "" || len(h.mailer.sent) != 2 || !strings.Contains(h.mailer.sent[1].Body, "Hello "+got.InvoiceID) {
		t.Errorf("scheduled = %s %q, mails %d", got.State, got.InvoiceID, len(h.mailer.sent))
	}
}

func TestSendReminders(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	a, b := sent(t, h, "2026-10-01"), sent(t, h, "2026-10-02")
	in := ReminderInput{ContactID: customer, WorkflowID: flow, DocumentStyleID: style, IdentityID: identity, InvoiceIDs: []resource.ID{a.ID, b.ID}, ReminderText: "Pay now:\n{invoices}"}
	if _, err := h.svc.SendReminders(ctx, adminID, owner, nil); fieldsOf(t, err)["sales_invoice_reminders"] == nil {
		t.Error("no reminders accepted")
	}
	bad := in
	bad.ContactID = noEmail
	if _, err := h.svc.SendReminders(ctx, adminID, owner, []ReminderInput{bad}); fieldsOf(t, err)["sales_invoice_reminders.0.sales_invoice_ids"] == nil {
		t.Error("invoices of another contact accepted")
	}
	list, err := h.svc.SendReminders(ctx, adminID, owner, []ReminderInput{in})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].State != Reminded || len(h.mailer.sent) != 1 || len(h.mailer.sent[0].Files) != 2 {
		t.Fatalf("reminded %d, mails %d", len(list), len(h.mailer.sent))
	}
	if m := h.mailer.sent[0]; !strings.Contains(m.Subject, "2 facturen") || !strings.Contains(m.Body, a.InvoiceID) || !strings.Contains(m.Body, b.InvoiceID) {
		t.Errorf("mail = %q / %q", m.Subject, m.Body)
	}
	in.DeliveryMethod, in.InvoiceIDs = "Manual", []resource.ID{a.ID}
	if list, err := h.svc.SendReminders(ctx, adminID, owner, []ReminderInput{in}); err != nil || list[0].ReminderCount != 2 || len(h.mailer.sent) != 1 {
		t.Errorf("manual = %+v, %v", list, err)
	}
	sum, err := h.svc.Summary(ctx, adminID, owner)
	if err != nil || sum.ToReceive.Count != 2 || sum.ToReceive.Amount != 24200 || sum.ToRemind.Count != 2 || sum.Currency != "EUR" {
		t.Errorf("summary = %+v, %v", sum, err)
	}
}
