package invoicing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func fieldsOf(t *testing.T, err error) map[string][]string {
	t.Helper()
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error = %v, want *resource.ValidationError", err)
	}
	return verr.Fields
}

func TestSettingsShowExamples(t *testing.T) {
	svc, _, _ := newTestService()
	got, err := svc.Settings(context.Background(), adminID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if got.InvoiceIDExample != "2026-0001" || got.EstimateIDExample != "2026-0001" {
		t.Errorf("examples = %q, %q", got.InvoiceIDExample, got.EstimateIDExample)
	}
	if _, err := svc.Settings(context.Background(), adminID, stranger); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("Settings() by stranger error = %v, want ErrNotFound", err)
	}
}

func TestUpdateSettings(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	got, err := svc.UpdateSettings(ctx, adminID, owner, SettingsChanges{
		InvoiceIDTemplate: ptr("F{year_two_digits}-{id}"), InvoiceSequenceID: ptr("1462"),
		InvoiceBCC: ptr(" a@example.com ,b@example.com,"), InvoiceRemindersBCC: ptr(true),
		EstimateBCC: ptr(""), DefaultEmailUBL: ptr(true), PaymentsWithoutProof: ptr(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.InvoiceIDExample != "F26-1462" || got.InvoiceBCC != "a@example.com, b@example.com" || !got.InvoiceRemindersBCC || !got.DefaultEmailUBL || !got.PaymentsWithoutProof {
		t.Errorf("UpdateSettings() = %+v", got)
	}

	got, err = svc.UpdateSettings(ctx, adminID, owner, SettingsChanges{InvoiceBCC: ptr("")})
	if err != nil {
		t.Fatal(err)
	}
	if got.InvoiceRemindersBCC {
		t.Error("reminder copies stay on without an address to send them to")
	}

	_, err = svc.UpdateSettings(ctx, adminID, owner, SettingsChanges{
		InvoiceIDTemplate: ptr("{year}"), EstimateSequenceID: ptr("x"), OrderBCC: ptr("nope"),
	})
	fields := fieldsOf(t, err)
	for _, f := range []string{"invoice_id_template", "estimate_sequence_id", "order_bcc_email_address"} {
		if fields[f] == nil {
			t.Errorf("fields = %v, want %s", fields, f)
		}
	}
}

func TestDefaultWorkflowsAreCreatedOnce(t *testing.T) {
	svc, store, _ := newTestService()
	ctx := context.Background()
	for range 2 {
		if _, err := svc.Workflows(ctx, adminID, owner); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := svc.Workflows(ctx, adminID, owner)
	if len(got) != 2 || len(store.workflows) != 2 {
		t.Fatalf("Workflows() = %d, stored %d, want 2", len(got), len(store.workflows))
	}
	estimate, invoice := got[0], got[1]
	if estimate.Type != EstimateWorkflow || invoice.Type != InvoiceWorkflow || !invoice.Default || invoice.Name != "Standaard (NL)" {
		t.Errorf("Workflows() = %+v", got)
	}
	// As Moneybird's: step 0 sends the invoice and expires after the payment
	// term; each reminder expires after its own due interval.
	// Moneybird's default: pay in 14 days, then three manual reminders that
	// expire after 10, 5 and 5 days (Day 15, 25 and 30).
	if len(invoice.Steps) != 4 {
		t.Fatalf("invoice steps = %+v, want 4", invoice.Steps)
	}
	for i, want := range []int{14, 10, 5, 5} {
		st := invoice.Steps[i]
		if st.StepOrder != i || st.DueInterval != want || st.ReminderDelay != 0 || st.ReminderAutoSend || (i > 0) != (st.ReminderText != "") {
			t.Errorf("invoice step %d = %+v, want due interval %d", i, st, want)
		}
	}
	for i, want := range []string{"Uit onze administratie blijkt", "Ondanks een eerdere herinnering", "Dit is de derde herinnering"} {
		text := invoice.Steps[i+1].ReminderText
		if !strings.HasPrefix(text, "Geachte {recipient.name},\n\n") || !strings.Contains(text, want) || !strings.Contains(text, "\n\n{reminders_table}\n\n") ||
			!strings.HasSuffix(text, "Met vriendelijke groet,\n\n{company.company_name}") {
			t.Errorf("reminder %d text = %q", i+1, text)
		}
	}
	if len(estimate.Steps) != 1 || estimate.Steps[0].DueInterval != 30 {
		t.Errorf("estimate steps = %+v", estimate.Steps)
	}
}

func TestDefaultEnglishWorkflowHasMoneybirdsRemindersInEnglish(t *testing.T) {
	svc, _, _ := newTestService()
	w := svc.defaultWorkflow(administration.Administration{ID: adminID, Language: "en", Currency: "EUR"}, InvoiceWorkflow)
	if w.Name != "Default (EN)" || len(w.Steps) != 4 {
		t.Fatalf("defaultWorkflow(en) = %+v", w)
	}
	for i, want := range []string{"According to our records", "Despite an earlier reminder", "This is the third reminder"} {
		text := w.Steps[i+1].ReminderText
		if !strings.HasPrefix(text, "Dear {recipient.name},\n\n") || !strings.Contains(text, want) || !strings.Contains(text, "\n\n{reminders_table}\n\n") ||
			!strings.HasSuffix(text, "Kind regards,\n\n{company.company_name}") {
			t.Errorf("reminder %d text = %q", i+1, text)
		}
	}
}

func TestWorkflowLifecycle(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	list, err := svc.Workflows(ctx, adminID, owner)
	if err != nil {
		t.Fatal(err)
	}
	oldDefault := list[1]

	w, err := svc.CreateWorkflow(ctx, adminID, owner, InvoiceWorkflow, WorkflowChanges{
		Name: ptr(" Export "), Currency: ptr("USD"), Language: ptr("en"),
		Steps: &[]StepInput{{DueInterval: 30}, {DueInterval: 10, ReminderText: "Pay"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if w.Name != "Export" || w.Default || w.Currency != "USD" || len(w.Steps) != 2 || w.Steps[0].StepOrder != 0 || w.Steps[1].StepOrder != 1 ||
		w.Steps[1].DueInterval != 10 || w.Steps[1].ID == 0 {
		t.Errorf("CreateWorkflow() = %+v", w)
	}
	if w.Ref().URI() != "/11/workflows/"+w.ID.String() {
		t.Errorf("URI = %s", w.Ref().URI())
	}

	if _, err := svc.UpdateWorkflow(ctx, adminID, w.ID, owner, WorkflowChanges{Default: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if again, _ := svc.Workflow(ctx, adminID, oldDefault.ID, owner); again.Default {
		t.Error("previous default is still the default")
	}
	if err := svc.DeleteWorkflow(ctx, adminID, w.ID, owner); !errors.Is(err, ErrDefaultInUse) {
		t.Errorf("DeleteWorkflow(default) error = %v, want ErrDefaultInUse", err)
	}
	if _, err := svc.UpdateWorkflow(ctx, adminID, w.ID, owner, WorkflowChanges{Active: ptr(false)}); err == nil {
		t.Error("deactivating the default workflow error = nil")
	}
	if err := svc.DeleteWorkflow(ctx, adminID, oldDefault.ID, owner); err != nil {
		t.Errorf("DeleteWorkflow(former default) error = %v", err)
	}
	if _, err := svc.Workflow(ctx, adminID, w.ID, stranger); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("Workflow() by stranger error = %v", err)
	}
}

func TestWorkflowValidation(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, err := svc.CreateWorkflow(ctx, adminID, owner, EstimateWorkflow, WorkflowChanges{
		Name: ptr(""), Currency: ptr("XXX"), Steps: &[]StepInput{{DueInterval: 30}, {DueInterval: 1}},
	})
	fields := fieldsOf(t, err)
	for _, f := range []string{"name", "currency", "steps"} {
		if fields[f] == nil {
			t.Errorf("fields = %v, want %s", fields, f)
		}
	}
	_, err = svc.CreateWorkflow(ctx, adminID, owner, InvoiceWorkflow, WorkflowChanges{
		Name: ptr("x"), Steps: &[]StepInput{{DueInterval: 400}, {DueInterval: 7}},
	})
	if fieldsOf(t, err)["steps"] == nil {
		t.Error("out-of-range steps were accepted")
	}
	// A reminder must run at least a day before the next one is due.
	for _, st := range []StepInput{{DueInterval: 0}, {DueInterval: 366}, {DueInterval: 7, ReminderDelay: -1}} {
		_, err = svc.CreateWorkflow(ctx, adminID, owner, InvoiceWorkflow, WorkflowChanges{Name: ptr("x"), Steps: &[]StepInput{{DueInterval: 14}, st}})
		if fieldsOf(t, err)["steps"] == nil {
			t.Errorf("reminder %+v was accepted", st)
		}
	}
	if _, err := svc.CreateWorkflow(ctx, adminID, owner, "PurchaseWorkflow", WorkflowChanges{Name: ptr("x")}); fieldsOf(t, err)["type"] == nil {
		t.Error("unknown type was accepted")
	}
}
