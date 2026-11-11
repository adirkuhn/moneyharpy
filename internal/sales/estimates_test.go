package sales

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func estimate(t *testing.T, h harness, c EstimateChanges) Estimate {
	t.Helper()
	if c.ContactID == nil {
		c.ContactID = ptr(customer)
	}
	if c.Details == nil {
		c.Details = []DetailInput{line("300", "1", vat21, revenue)}
	}
	e, err := h.svc.CreateEstimate(context.Background(), adminID, owner, c)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func sentEstimate(t *testing.T, h harness) Estimate {
	t.Helper()
	e := estimate(t, h, EstimateChanges{Reference: ptr("Project X")})
	sent, err := h.svc.SendEstimate(context.Background(), adminID, e.ID, owner, EstimateSendInput{})
	if err != nil {
		t.Fatal(err)
	}
	return sent
}

func TestCreateEstimateFollowsTheEstimateWorkflow(t *testing.T) {
	h := newHarness(t)
	e := estimate(t, h, EstimateChanges{PreText: ptr(" Hello "), PostText: ptr("Please sign"), Language: ptr("nl")})
	if e.State != Draft || e.DraftID != 1 || e.WorkflowID != estimateFlow || e.DocumentStyleID != style || e.IdentityID != identity {
		t.Errorf("defaults = %+v", e)
	}
	if e.FirstDueInterval != 30 || e.Currency != "EUR" || e.Language != "nl" || !e.ShowTax || !e.SignOnline || e.EstimateDate != nil || e.EstimateID != "" {
		t.Errorf("workflow defaults = %+v", e)
	}
	if e.PreText != "Hello" || e.PostText != "Please sign" || e.Totals.InclTax != 36300 || e.Ref().URI() != "/100/estimates/"+e.ID.String() {
		t.Errorf("texts and totals = %+v", e)
	}
	if second := estimate(t, h, EstimateChanges{}); second.DraftID != 2 {
		t.Errorf("draft id = %d", second.DraftID)
	}
}

func TestCreateEstimateValidates(t *testing.T) {
	h := newHarness(t)
	_, err := h.svc.CreateEstimate(context.Background(), adminID, owner, EstimateChanges{
		WorkflowID: ptr(flow), Language: ptr("abracadabra"), EstimateDate: ptr("yesterday"), SequenceID: ptr("x"),
		OriginalSalesInvoiceID: ptr(resource.ID(9)), Details: []DetailInput{line("x", "1", vat21, 0)},
	})
	f := fieldsOf(t, err)
	for _, key := range []string{"contact_id", "workflow_id", "language", "estimate_date", "estimate_sequence_id", "original_sales_invoice_id", "details_attributes.0.price"} {
		if len(f[key]) == 0 {
			t.Errorf("no error for %s in %v", key, f)
		}
	}
}

func TestSendEstimateNumbersAndEmails(t *testing.T) {
	h := newHarness(t)
	e := sentEstimate(t, h)
	if e.EstimateID != "Q2026-0001" || e.State != Open || e.EstimateDate.Format(time.DateOnly) != "2026-11-11" || e.DueDate.Format(time.DateOnly) != "2026-12-11" || e.SentAt == nil {
		t.Errorf("sent = %+v", e)
	}
	if len(h.mailer.sent) != 1 {
		t.Fatalf("mails = %d", len(h.mailer.sent))
	}
	m := h.mailer.sent[0]
	if m.To[0] != "billing@example.com" || m.BCC[0] != "quotes@example.com" || m.Subject != "Quote Q2026-0001 from Harpy B.V." || !strings.Contains(m.Body, "€363.00") {
		t.Errorf("mail = %+v", m)
	}
	if len(m.Files) != 1 || m.Files[0].Name != "estimate-Q2026-0001.pdf" || !strings.HasPrefix(string(m.Files[0].Data), "%PDF Q2026-0001") {
		t.Errorf("files = %+v", m.Files)
	}
	// Sending again keeps the number and state, and takes a message.
	again, err := h.svc.SendEstimate(context.Background(), adminID, e.ID, owner, EstimateSendInput{EmailAddress: "alternative@example.com", EmailMessage: "Hi, this is an estimate with id {invoice_id}!"})
	if err != nil {
		t.Fatal(err)
	}
	if again.EstimateID != "Q2026-0001" || h.mailer.sent[1].To[0] != "alternative@example.com" || h.mailer.sent[1].Body != "Hi, this is an estimate with id Q2026-0001!" {
		t.Errorf("again = %+v, mail %+v", again, h.mailer.sent[1])
	}
	// The estimate's sequence id picks the number.
	other := estimate(t, h, EstimateChanges{SequenceID: ptr("0042")})
	if sent, err := h.svc.SendEstimate(context.Background(), adminID, other.ID, owner, EstimateSendInput{DeliveryMethod: "Manual"}); err != nil || sent.EstimateID != "Q2026-0042" || len(h.mailer.sent) != 2 {
		t.Errorf("manual = %+v, %v", sent, err)
	}
}

func TestSendEstimateRefusesWhatCannotGo(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	empty := estimate(t, h, EstimateChanges{Details: []DetailInput{}})
	if _, err := h.svc.SendEstimate(ctx, adminID, empty.ID, owner, EstimateSendInput{}); !errors.Is(err, ErrEstimateNoDetails) {
		t.Errorf("empty = %v", err)
	}
	ann := estimate(t, h, EstimateChanges{ContactID: ptr(noEmail)})
	if _, err := h.svc.SendEstimate(ctx, adminID, ann.ID, owner, EstimateSendInput{}); len(fieldsOf(t, err)["email_address"]) == 0 {
		t.Errorf("no email = %v", err)
	}
	if _, err := h.svc.SendEstimate(ctx, adminID, ann.ID, owner, EstimateSendInput{DeliveryMethod: "Post"}); len(fieldsOf(t, err)["delivery_method"]) == 0 {
		t.Errorf("post = %v", err)
	}
	h.mailer.fail = true
	if _, err := h.svc.SendEstimate(ctx, adminID, ann.ID, owner, EstimateSendInput{EmailAddress: "ann@example.com"}); !errors.Is(err, ErrEstimateDelivery) {
		t.Errorf("failed mail = %v", err)
	}
	if got, _ := h.svc.Estimate(ctx, adminID, ann.ID, owner); got.State != Open {
		t.Errorf("an estimate that could not be mailed is still sent: %s", got.State)
	}
}

func TestChangeEstimateState(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	draft := estimate(t, h, EstimateChanges{})
	var serr *StateError
	if _, err := h.svc.ChangeEstimateState(ctx, adminID, draft.ID, owner, Open); !errors.As(err, &serr) || serr.Symbol() != "can_not_change_draft_to_open" {
		t.Errorf("draft to open = %v", err)
	}
	if _, err := h.svc.ChangeEstimateState(ctx, adminID, draft.ID, owner, "draft"); !errors.As(err, &serr) || serr.Symbol() != "in" {
		t.Errorf("to draft = %v", err)
	}
	e := sentEstimate(t, h)
	accepted, err := h.svc.ChangeEstimateState(ctx, adminID, e.ID, owner, Accepted)
	if err != nil || accepted.State != Accepted || accepted.AcceptedAt == nil {
		t.Fatalf("accepted = %+v, %v", accepted, err)
	}
	rejected, err := h.svc.ChangeEstimateState(ctx, adminID, e.ID, owner, Rejected)
	if err != nil || rejected.RejectedAt == nil || rejected.AcceptedAt != nil {
		t.Errorf("rejected = %+v, %v", rejected, err)
	}
	if _, err := h.svc.ChangeEstimateState(ctx, adminID, e.ID, owner, Billed); !errors.As(err, &serr) || serr.Symbol() != "can_not_change_rejected_to_billed" {
		t.Errorf("rejected to billed = %v", err)
	}
	archived, err := h.svc.ChangeEstimateState(ctx, adminID, e.ID, owner, Archived)
	if err != nil || archived.ArchivedAt == nil {
		t.Errorf("archived = %+v, %v", archived, err)
	}
	if _, err := h.svc.UpdateEstimate(ctx, adminID, e.ID, owner, EstimateChanges{Reference: ptr("x")}); !errors.Is(err, ErrEstimateNotEditable) {
		t.Errorf("update archived = %v", err)
	}
	reopened, err := h.svc.ChangeEstimateState(ctx, adminID, e.ID, owner, Open)
	if err != nil || reopened.State != Open || reopened.ArchivedAt != nil || reopened.RejectedAt != nil {
		t.Errorf("reopened = %+v, %v", reopened, err)
	}
}

func TestOpenEstimatePastItsDueDateIsLate(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := sentEstimate(t, h)
	*h.clock = now.AddDate(0, 2, 0)
	late, err := h.svc.Estimate(ctx, adminID, e.ID, owner)
	if err != nil || late.State != Late {
		t.Fatalf("late = %+v, %v", late, err)
	}
	q, err := ParseEstimateFilter("state:late,period:prev_year..next_year", *h.clock)
	if err == nil {
		t.Errorf("a malformed period was accepted: %+v", q)
	}
	q, err = ParseEstimateFilter("state:late|draft,contact_id:300", *h.clock)
	if err != nil {
		t.Fatal(err)
	}
	if list, total, err := h.svc.Estimates(ctx, adminID, owner, q); err != nil || total != 1 || list[0].ID != e.ID {
		t.Errorf("late list = %d, %v", total, err)
	}
	if q, _ := ParseEstimateFilter("state:open", *h.clock); q.Matches(late) {
		t.Error("a late estimate counts as open")
	}
	if _, err := ParseEstimateFilter("state:paid", now); err == nil {
		t.Error("an invoice state was accepted")
	}
	if _, err := ParseEstimateFilter("recurring_sales_invoice_id:1", now); err == nil {
		t.Error("an invoice key was accepted")
	}
}

func TestUpdateSentEstimateMovesTheDueDate(t *testing.T) {
	h := newHarness(t)
	e := sentEstimate(t, h)
	got, err := h.svc.UpdateEstimate(context.Background(), adminID, e.ID, owner, EstimateChanges{EstimateDate: ptr("2026-11-20"), ShowTax: ptr(false)})
	if err != nil || got.DueDate.Format(time.DateOnly) != "2026-12-20" || got.ShowTax {
		t.Errorf("updated = %+v, %v", got, err)
	}
	if _, err := h.svc.UpdateEstimate(context.Background(), adminID, e.ID, owner, EstimateChanges{EstimateDate: ptr("")}); len(fieldsOf(t, err)["estimate_date"]) == 0 {
		t.Errorf("blank date = %v", err)
	}
}

func TestBillEstimateMakesALinkedInvoice(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	draft := estimate(t, h, EstimateChanges{})
	if _, err := h.svc.BillEstimate(ctx, adminID, draft.ID, owner); !errors.Is(err, ErrEstimateNotBillable) {
		t.Errorf("bill draft = %v", err)
	}
	e := sentEstimate(t, h)
	if _, err := h.svc.ChangeEstimateState(ctx, adminID, e.ID, owner, Accepted); err != nil {
		t.Fatal(err)
	}
	inv, err := h.svc.BillEstimate(ctx, adminID, e.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if inv.State != Draft || inv.OriginalEstimateID == nil || *inv.OriginalEstimateID != e.ID || inv.WorkflowID != flow || inv.Reference != "Project X" {
		t.Errorf("invoice = %+v", inv)
	}
	if inv.Totals.InclTax != e.Totals.InclTax || len(inv.Lines) != 1 || inv.Lines[0].ID == e.Lines[0].ID || inv.Language != e.Language || inv.FirstDueInterval != 14 {
		t.Errorf("invoice lines and totals = %+v", inv)
	}
	billed, _ := h.svc.Estimate(ctx, adminID, e.ID, owner)
	if billed.State != Billed {
		t.Errorf("estimate state = %s", billed.State)
	}
	if _, err := h.svc.BillEstimate(ctx, adminID, e.ID, owner); !errors.Is(err, ErrEstimateNotBillable) {
		t.Errorf("bill again = %v", err)
	}
	// Deleting the estimate keeps the invoice.
	if err := h.svc.DeleteEstimate(ctx, adminID, e.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Invoice(ctx, adminID, inv.ID, owner); err != nil {
		t.Errorf("invoice after delete = %v", err)
	}
	if _, err := h.svc.Estimate(ctx, adminID, e.ID, owner); !IsNotFound(err) {
		t.Errorf("deleted estimate = %v", err)
	}
}

func TestEstimateNotesAttachmentsAndPDFs(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := estimate(t, h, EstimateChanges{})
	n, err := h.svc.AddEstimateNote(ctx, adminID, e.ID, owner, notes.Input{Note: "Called"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := h.svc.AddEstimateAttachment(ctx, adminID, e.ID, owner, "brief.pdf", "application/pdf", []byte("%PDF-1.4"))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := h.svc.Estimate(ctx, adminID, e.ID, owner)
	if len(got.Notes) != 1 || len(got.Attachments) != 1 || got.Notes[0].EntityType != EstimateEntityType {
		t.Errorf("estimate = %+v", got)
	}
	if err := h.svc.DeleteEstimateNote(ctx, adminID, e.ID, n.ID, owner); err != nil {
		t.Error(err)
	}
	if err := h.svc.DeleteEstimateAttachment(ctx, adminID, e.ID, a.ID, owner); err != nil {
		t.Error(err)
	}
	data, name, err := h.svc.EstimatePDF(ctx, adminID, e.ID, owner, false)
	if err != nil || name != "estimate-draft-1.pdf" || !strings.HasPrefix(string(data), "%PDF") {
		t.Errorf("pdf = %s, %v", name, err)
	}
	if _, _, err := h.svc.SignedEstimatePDF(ctx, adminID, e.ID, owner); !errors.Is(err, ErrEstimateNotSigned) {
		t.Errorf("signed = %v", err)
	}
	if _, _, err := h.svc.SignedEstimatePDF(ctx, adminID, 1, owner); !IsNotFound(err) {
		t.Errorf("signed of none = %v", err)
	}
	if _, err := h.svc.Estimate(ctx, adminID, e.ID, stranger); err == nil {
		t.Error("a stranger read an estimate")
	}
}

func TestEstimatesSyncAndFind(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := sentEstimate(t, h)
	other := estimate(t, h, EstimateChanges{})
	versions, err := h.svc.EstimateVersions(ctx, adminID, owner, EstimateQuery{})
	if err != nil || len(versions) != 2 {
		t.Errorf("versions = %d, %v", len(versions), err)
	}
	list, err := h.svc.EstimatesByID(ctx, adminID, owner, []resource.ID{other.ID, 1})
	if err != nil || len(list) != 1 || list[0].ID != other.ID {
		t.Errorf("by id = %+v, %v", list, err)
	}
	found, err := h.svc.EstimateByNumber(ctx, adminID, owner, "Q2026-0001")
	if err != nil || found.ID != e.ID {
		t.Errorf("by number = %+v, %v", found, err)
	}
	page, total, err := h.svc.Estimates(ctx, adminID, owner, EstimateQuery{PerPage: 1})
	if err != nil || total != 2 || len(page) != 1 {
		t.Errorf("page = %d of %d, %v", len(page), total, err)
	}
}
