package sales

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func ptr[T any](v T) *T { return &v }

func fieldsOf(t *testing.T, err error) map[string][]string {
	t.Helper()
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want a validation error, got %v", err)
	}
	return verr.Fields
}

func line(price, amount string, rate resource.ID, ledger resource.ID) DetailInput {
	in := DetailInput{Description: ptr("Consulting"), Price: ptr(price), Amount: ptr(amount), TaxRateID: ptr(rate)}
	if ledger != 0 {
		in.LedgerAccountID = ptr(ledger)
	}
	return in
}

func draft(t *testing.T, h harness, details ...DetailInput) Invoice {
	t.Helper()
	inv, err := h.svc.Create(context.Background(), adminID, owner, Changes{ContactID: ptr(customer), Reference: ptr("Order 1"), Details: details})
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func TestCreateTakesTheDefaults(t *testing.T) {
	h := newHarness(t)
	inv := draft(t, h, line("100", "2", vat21, revenue))
	if inv.State != Draft || inv.DraftID != 1 || inv.WorkflowID != flow || inv.DocumentStyleID != style || inv.IdentityID != identity {
		t.Errorf("defaults = %+v", inv)
	}
	if inv.Language != "nl" || inv.FirstDueInterval != 14 || inv.Currency != "EUR" || inv.InvoiceID != "" {
		t.Errorf("workflow defaults = %+v", inv)
	}
	if inv.Lines[0].Quantity != 2000 || inv.Lines[0].Price != 10000 || !inv.Lines[0].Selected {
		t.Errorf("line = %+v", inv.Lines[0])
	}
	if second := draft(t, h); second.DraftID != 2 {
		t.Errorf("draft id = %d", second.DraftID)
	}
}

func TestNextDraftIDIsTheNumberCreateWouldGive(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if got, err := h.svc.NextDraftID(ctx, adminID, owner); err != nil || got != 1 {
		t.Fatalf("first = %d, %v", got, err)
	}
	draft(t, h)
	want, err := h.svc.NextDraftID(ctx, adminID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if got := draft(t, h); want != 2 || got.DraftID != want {
		t.Errorf("next = %d, created %d", want, got.DraftID)
	}
	if _, err := h.svc.NextDraftID(ctx, adminID, stranger); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("stranger = %v", err)
	}
}

func TestCreateValidates(t *testing.T) {
	h := newHarness(t)
	_, err := h.svc.Create(context.Background(), adminID, owner, Changes{
		WorkflowID: ptr(estimateFlow), Discount: ptr("120"), Currency: ptr("euro"),
		Details: []DetailInput{line("x", "abc", purchaseVAT, resource.ID(9)), {Description: ptr("No rate"), Period: ptr("2026-11")}},
	})
	f := fieldsOf(t, err)
	for _, key := range []string{"contact_id", "workflow_id", "discount", "currency", "details_attributes.0.price", "details_attributes.0.amount",
		"details_attributes.0.tax_rate_id", "details_attributes.0.ledger_account_id", "details_attributes.1.tax_rate_id", "details_attributes.1.period"} {
		if len(f[key]) == 0 {
			t.Errorf("no error for %s in %v", key, f)
		}
	}
}

func TestSendNumbersBooksAndEmails(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := draft(t, h, line("100", "2", vat21, revenue), line("50", "1", vat9, 0))
	sent, err := h.svc.Send(ctx, adminID, inv.ID, owner, SendInput{})
	if err != nil {
		t.Fatal(err)
	}
	if sent.InvoiceID != "2026-0001" || sent.State != Open || sent.InvoiceDate.Format(time.DateOnly) != "2026-11-11" || sent.DueDate.Format(time.DateOnly) != "2026-11-25" {
		t.Errorf("sent = %+v", sent)
	}
	entries := h.store.entries[inv.ID]
	if err := journal.Balance(entries); err != nil {
		t.Fatal(err)
	}
	sums := map[resource.ID]money.Amount{}
	for _, e := range entries {
		sums[e.LedgerAccountID] += e.Amount
		if e.ContactID == nil || *e.ContactID != customer || e.DocumentType != journal.SalesInvoice {
			t.Errorf("entry = %+v", e)
		}
	}
	// 200 + 21% and 50 + 9%: 242.00 + 54.50 = 296.50.
	if sums[debtors] != 29650 || sums[revenue] != -20000 || sums[uncategorized] != -5000 || sums[payableVAT] != -4650 {
		t.Errorf("booked = %v", sums)
	}
	if len(h.mailer.sent) != 1 {
		t.Fatalf("mails = %d", len(h.mailer.sent))
	}
	m := h.mailer.sent[0]
	if m.To[0] != "billing@example.com" || m.BCC[0] != "copy@example.com" || !strings.Contains(m.Subject, "2026-0001") || len(m.Files) != 1 || !strings.Contains(m.Body, "€ 296,50") {
		t.Errorf("mail = %+v", m)
	}
	if !strings.HasPrefix(string(m.Files[0].Data), "%PDF 2026-0001") || m.Files[0].Name != "invoice-2026-0001.pdf" {
		t.Errorf("file = %s %q", m.Files[0].Name, m.Files[0].Data)
	}
	if m.Look == nil || m.Look.Style.HeaderType != "background_color_with_logo" || m.Look.Style.FontFamily != "georgia" || m.Look.Sender.CompanyName == "" {
		t.Fatalf("look = %+v", m.Look)
	}
	if logo := m.Look.Images[invoicing.EmailLogo]; string(logo.Data) != "GIF89a logo" || logo.ContentType != "image/gif" {
		t.Errorf("email logo = %+v", logo)
	}
	if want := [2]string{"Factuur", "2026-0001"}; len(m.Look.Details) == 0 || m.Look.Details[0] != want {
		t.Errorf("details = %v", m.Look.Details)
	}
	if !slices.Contains(m.Look.Details, [2]string{"Openstaand bedrag", "€ 296,50"}) {
		t.Errorf("details = %v, want the amount due", m.Look.Details)
	}

	// Sending again delivers a copy without a new number or booking.
	again, err := h.svc.Send(ctx, adminID, inv.ID, owner, SendInput{EmailAddress: "other@example.com", DeliverUBL: ptr(true)})
	if err != nil || again.InvoiceID != "2026-0001" || len(h.mailer.sent) != 2 || len(h.mailer.sent[1].Files) != 2 {
		t.Errorf("resend = %+v, %v", again, err)
	}
	next := draft(t, h, line("10", "1", vat21, revenue))
	if n, _ := h.svc.Send(ctx, adminID, next.ID, owner, SendInput{DeliveryMethod: "Manual"}); n.InvoiceID != "2026-0002" || len(h.mailer.sent) != 2 {
		t.Errorf("manual = %+v", n)
	}
}

func TestSendNeedsLinesAndAnAddress(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	empty := draft(t, h)
	if _, err := h.svc.Send(ctx, adminID, empty.ID, owner, SendInput{}); fieldsOf(t, err)["details_attributes"] == nil {
		t.Error("sent without lines")
	}
	inv, _ := h.svc.Create(ctx, adminID, owner, Changes{ContactID: ptr(noEmail), Details: []DetailInput{line("10", "1", vat21, revenue)}})
	if _, err := h.svc.Send(ctx, adminID, inv.ID, owner, SendInput{}); fieldsOf(t, err)["email_address"] == nil {
		t.Error("emailed without an address")
	}
	if _, err := h.svc.Send(ctx, adminID, inv.ID, owner, SendInput{DeliveryMethod: "Peppol"}); !errors.Is(err, ErrDeliveryUnavailable) {
		t.Errorf("peppol = %v", err)
	}
	if got, _ := h.svc.Invoice(ctx, adminID, inv.ID, owner); got.State != Draft {
		t.Errorf("state after refusals = %s", got.State)
	}
}

func TestDeliveryFailureKeepsTheInvoiceSent(t *testing.T) {
	h := newHarness(t)
	h.mailer.fail = true
	inv := draft(t, h, line("10", "1", vat21, revenue))
	if _, err := h.svc.Send(context.Background(), adminID, inv.ID, owner, SendInput{}); !errors.Is(err, ErrDelivery) {
		t.Fatalf("send = %v", err)
	}
	if got, _ := h.svc.Invoice(context.Background(), adminID, inv.ID, owner); got.State != Open || got.InvoiceID == "" {
		t.Errorf("after failed delivery = %+v", got)
	}
}

func TestScheduledSendingWaits(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := draft(t, h, line("10", "1", vat21, revenue))
	if _, err := h.svc.Send(ctx, adminID, inv.ID, owner, SendInput{Scheduled: true, InvoiceDate: "2026-11-11"}); fieldsOf(t, err)["invoice_date"] == nil {
		t.Error("scheduled for today")
	}
	got, err := h.svc.Send(ctx, adminID, inv.ID, owner, SendInput{Scheduled: true, InvoiceDate: "2026-12-01"})
	if err != nil || got.State != Scheduled || got.InvoiceID != "" || len(h.store.entries[inv.ID]) != 0 || len(h.mailer.sent) != 0 {
		t.Errorf("scheduled = %+v, %v", got, err)
	}
}

func TestLateInvoices(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := draft(t, h, line("10", "1", vat21, revenue))
	if _, err := h.svc.Send(ctx, adminID, inv.ID, owner, SendInput{InvoiceDate: "2026-10-01"}); err != nil {
		t.Fatal(err)
	}
	got, _ := h.svc.Invoice(ctx, adminID, inv.ID, owner)
	if got.State != Late {
		t.Errorf("state = %s", got.State)
	}
	q, _ := ParseFilter("state:late", now)
	list, total, _ := h.svc.List(ctx, adminID, owner, q)
	if total != 1 || list[0].ID != inv.ID {
		t.Errorf("late = %d", total)
	}
	q, _ = ParseFilter("state:open", now)
	if _, total, _ := h.svc.List(ctx, adminID, owner, q); total != 0 {
		t.Errorf("open = %d", total)
	}
}

func TestUpdateRebooksAndRespectsTheLock(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := draft(t, h, line("100", "1", vat21, revenue))
	sent, _ := h.svc.Send(ctx, adminID, inv.ID, owner, SendInput{DeliveryMethod: "Manual"})
	updated, err := h.svc.Update(ctx, adminID, inv.ID, owner, Changes{Details: []DetailInput{{ID: &sent.Lines[0].ID, Price: ptr("200")}}})
	if err != nil {
		t.Fatal(err)
	}
	var receivable money.Amount
	for _, e := range h.store.entries[inv.ID] {
		if e.LedgerAccountID == debtors {
			receivable += e.Amount
		}
	}
	if receivable != 24200 || updated.InvoiceID != sent.InvoiceID {
		t.Errorf("rebooked debtors = %s", receivable)
	}
	h.ledger.locked = "2026-11-30"
	if _, err := h.svc.Update(ctx, adminID, inv.ID, owner, Changes{Reference: ptr("x")}); fieldsOf(t, err)["invoice_date"] == nil {
		t.Error("changed in the locked period")
	}
	if err := h.svc.Delete(ctx, adminID, inv.ID, owner); fieldsOf(t, err)["invoice_date"] == nil {
		t.Errorf("deleted in the locked period: %v", err)
	}
}

func TestCreditInvoice(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := draft(t, h, line("100", "2 uur", vat21, revenue), line("5", "1", vat21, revenue))
	if _, err := h.svc.Credit(ctx, adminID, inv.ID, owner, nil); err == nil {
		t.Error("credited a draft")
	}
	sent, _ := h.svc.Send(ctx, adminID, inv.ID, owner, SendInput{DeliveryMethod: "Manual"})
	credit, err := h.svc.Credit(ctx, adminID, inv.ID, owner, []resource.ID{sent.Lines[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if credit.State != Draft || credit.OriginalID == nil || *credit.OriginalID != inv.ID || len(credit.Lines) != 1 || credit.Lines[0].Amount != "-2 uur" || credit.Lines[0].Quantity != -2000 {
		t.Errorf("credit = %+v", credit)
	}
	tot, _ := h.svc.Totals(ctx, adminID, owner, credit)
	if tot.InclTax != -24200 {
		t.Errorf("credit total = %s", tot.InclTax)
	}
}

func TestPauseNotesAndDelete(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := draft(t, h, line("10", "1", vat21, revenue))
	if _, err := h.svc.Pause(ctx, adminID, inv.ID, owner, true); !errors.Is(err, ErrNotEditable) {
		t.Errorf("paused a draft: %v", err)
	}
	n, err := h.svc.AddNote(ctx, adminID, inv.ID, owner, notes.Input{Note: "Call first"})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := h.svc.Invoice(ctx, adminID, inv.ID, owner); len(got.Notes) != 1 {
		t.Errorf("notes = %+v", got.Notes)
	}
	if err := h.svc.DeleteNote(ctx, adminID, inv.ID, n.ID, owner); err != nil {
		t.Fatal(err)
	}
	sent, _ := h.svc.Send(ctx, adminID, inv.ID, owner, SendInput{DeliveryMethod: "Manual"})
	if p, err := h.svc.Pause(ctx, adminID, sent.ID, owner, true); err != nil || !p.Paused {
		t.Errorf("pause = %+v, %v", p, err)
	}
	other := draft(t, h)
	if err := h.svc.Delete(ctx, adminID, other.ID, owner); err != nil {
		t.Errorf("delete draft = %v", err)
	}
	if _, err := h.svc.Invoice(ctx, adminID, other.ID, owner); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted draft = %v", err)
	}
	if _, err := h.svc.Invoice(ctx, adminID, inv.ID, stranger); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("stranger = %v", err)
	}
}

func TestOwnSequenceMustBeFree(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	first := draft(t, h, line("10", "1", vat21, revenue))
	if _, err := h.svc.Send(ctx, adminID, first.ID, owner, SendInput{DeliveryMethod: "Manual"}); err != nil {
		t.Fatal(err)
	}
	second, _ := h.svc.Create(ctx, adminID, owner, Changes{ContactID: ptr(customer), SequenceID: ptr("0001"), Details: []DetailInput{line("10", "1", vat21, revenue)}})
	if _, err := h.svc.Send(ctx, adminID, second.ID, owner, SendInput{DeliveryMethod: "Manual"}); fieldsOf(t, err)["invoice_sequence_id"] == nil {
		t.Error("duplicate number accepted")
	}
}

func TestEmailTemplates(t *testing.T) {
	doc := Document{Invoice: Invoice{InvoiceID: "2026-0001", Language: "en", Currency: "EUR", PaymentReference: "2026-0001"}, Totals: Totals{InclTax: 123450}}
	doc.Identity.CompanyName = "Harpy B.V."
	doc.Contact.CompanyName = "Example Shop"
	subject, body := EmailText(doc)
	if subject != "Invoice 2026-0001 from Harpy B.V." || !strings.Contains(body, "€1,234.50") || !strings.Contains(body, "Dear Example Shop") {
		t.Errorf("default = %q / %q", subject, body)
	}
	doc.Workflow.InvoiceEmailSubject = "{company_name}: {invoice_id}"
	if subject, _ := EmailText(doc); subject != "Harpy B.V.: 2026-0001" {
		t.Errorf("custom = %q", subject)
	}
	if FormatMoney(-123450, "EUR", "nl") != "-€ 1.234,50" {
		t.Errorf("nl = %q", FormatMoney(-123450, "EUR", "nl"))
	}
}

func TestStationeryLeavesTheSenderOut(t *testing.T) {
	h := newHarness(t)
	inv := draft(t, h, line("100", "1", vat21, revenue))
	ctx := context.Background()
	plain, _, err := h.svc.PackingSlip(ctx, adminID, inv.ID, owner, false)
	if err != nil || string(plain) != "%PDF slip" {
		t.Fatalf("plain = %q, %v", plain, err)
	}
	if got, _, _ := h.svc.PackingSlip(ctx, adminID, inv.ID, owner, true); string(got) != "%PDF slip stationery" {
		t.Errorf("stationery = %q", got)
	}
}

func TestLinesTakeTheirProductAndProject(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := draft(t, h, DetailInput{ProductID: ptr(widget), ProjectID: ptr(falcon)}, DetailInput{ProductID: ptr(widget), Price: ptr("30"), Description: ptr("Own words")})
	first, second := inv.Lines[0], inv.Lines[1]
	if first.Description != "Widget, blue" || first.Price != 2500 || first.TaxRateID != vat21 || *first.LedgerAccountID != consulting || *first.ProductID != widget || *first.ProjectID != falcon {
		t.Errorf("product line = %+v", first)
	}
	if second.Description != "Own words" || second.Price != 3000 || second.TaxRateID != vat21 {
		t.Errorf("line with its own price = %+v", second)
	}
	// Naming the same product again keeps what the line says.
	up, err := h.svc.Update(ctx, adminID, inv.ID, owner, Changes{Details: []DetailInput{{ID: &first.ID, ProductID: ptr(widget), ProjectID: ptr(resource.ID(0)), Price: ptr("26")}}})
	if err != nil || up.Lines[0].Price != 2600 || up.Lines[0].ProjectID != nil || *up.Lines[0].ProductID != widget {
		t.Fatalf("updated = %+v, %v", up.Lines, err)
	}
	_, err = h.svc.Create(ctx, adminID, owner, Changes{ContactID: ptr(customer), Details: []DetailInput{
		{ProductID: ptr(retired), TaxRateID: ptr(vat21)}, {ProductID: ptr(resource.ID(5)), TaxRateID: ptr(vat21)},
		{ProjectID: ptr(shelved), TaxRateID: ptr(vat21)}, {ProjectID: ptr(resource.ID(5)), TaxRateID: ptr(vat21)},
	}})
	f := fieldsOf(t, err)
	for key, want := range map[string]string{"details_attributes.0.product_id": "is deactivated", "details_attributes.1.product_id": "does not exist",
		"details_attributes.2.project_id": "is archived", "details_attributes.3.project_id": "does not exist"} {
		if len(f[key]) != 1 || f[key][0] != want {
			t.Errorf("%s = %v, want %s", key, f[key], want)
		}
	}
}
