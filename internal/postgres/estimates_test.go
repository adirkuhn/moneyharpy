package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

func TestReserveEstimateNumbers(t *testing.T) {
	ctx := context.Background()
	a, _, _ := newAdministrationWithIdentity(t)
	store := postgres.NewSalesStore(testPool)
	for _, want := range []string{"0001", "0002"} {
		if got, err := store.ReserveEstimateNumber(ctx, a.ID); err != nil || got != want {
			t.Fatalf("number = %q, %v; want %s", got, err, want)
		}
	}
	// Invoices number on their own.
	if got, _ := store.ReserveNumber(ctx, a.ID); got != "0001" {
		t.Errorf("invoice number = %q", got)
	}
}

func TestEstimatesAreStoredSentAndBilled(t *testing.T) {
	ctx := context.Background()
	f := newSalesFixture(t)
	contact, err := f.people.Create(ctx, f.admin, f.owner, contacts.Changes{CompanyName: ptr("Example Quotes B.V.")})
	if err != nil {
		t.Fatal(err)
	}
	quoted := contact.ID
	widget, err := f.products.Create(ctx, f.admin, f.owner, products.Changes{Title: ptr("Widget"), Price: ptr("300"), TaxRateID: &f.vat21, LedgerAccountID: &f.revenue})
	if err != nil {
		t.Fatal(err)
	}
	e, err := f.svc.CreateEstimate(ctx, f.admin, f.owner, sales.EstimateChanges{
		ContactID: &quoted, Reference: ptr("Project X"), PreText: ptr("Hello"), PostText: ptr("Please sign"), Discount: ptr("10"),
		Details: []sales.DetailInput{{ProductID: &widget.ID, Amount: ptr("1")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.store.Estimate(ctx, f.admin, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Reference != "Project X" || got.PreText != "Hello" || got.PostText != "Please sign" || got.Discount != 1000 || got.State != sales.Draft ||
		!got.ShowTax || !got.SignOnline || got.DraftID != 1 || got.EstimateDate != nil || got.WorkflowID == 0 {
		t.Fatalf("stored = %+v", got)
	}
	// 300 - 10% = 270 + 21% = 326.70.
	if len(got.Lines) != 1 || *got.Lines[0].ProductID != widget.ID || got.Totals.InclTax != 32670 || len(got.Totals.ByRate) != 1 || len(got.Totals.LineBases) != 1 {
		t.Errorf("lines and totals = %+v, %+v", got.Lines, got.Totals)
	}
	for what, used := range map[string]func() (bool, error){
		"contact": func() (bool, error) { return f.store.ContactInUse(ctx, f.admin, quoted) },
		"product": func() (bool, error) { return f.store.ProductInUse(ctx, f.admin, widget.ID) },
	} {
		if ok, err := used(); err != nil || !ok {
			t.Errorf("%s in use = %v, %v", what, ok, err)
		}
	}
	if _, err := f.svc.AddEstimateNote(ctx, f.admin, e.ID, f.owner, notes.Input{Note: "Called"}); err != nil {
		t.Fatal(err)
	}

	sent, err := f.svc.SendEstimate(ctx, f.admin, e.ID, f.owner, sales.EstimateSendInput{DeliveryMethod: "Manual"})
	if err != nil {
		t.Fatal(err)
	}
	if sent.State != sales.Open || sent.EstimateID == "" || sent.EstimateDate == nil || sent.DueDate == nil || sent.Delivery.Method != "Manual" || len(sent.Notes) != 1 {
		t.Fatalf("sent = %+v", sent)
	}
	if byNumber, err := f.store.EstimateByNumber(ctx, f.admin, sent.EstimateID); err != nil || byNumber.ID != e.ID {
		t.Errorf("by number = %v", err)
	}
	today := adminNow()
	for filter, want := range map[string]int{"state:open": 1, "state:draft|late": 0, "contact_id:" + quoted.String(): 1, "period:prev_year": 0} {
		q, err := sales.ParseEstimateFilter(filter, today)
		if err != nil {
			t.Fatal(err)
		}
		if _, total, err := f.store.Estimates(ctx, f.admin, q); err != nil || total != want {
			t.Errorf("%s = %d, %v; want %d", filter, total, err, want)
		}
	}
	// Past its due date it is late.
	q, _ := sales.ParseEstimateFilter("state:late", today.AddDate(0, 3, 0))
	if _, total, _ := f.store.Estimates(ctx, f.admin, q); total != 1 {
		t.Errorf("late = %d", total)
	}

	accepted, err := f.svc.ChangeEstimateState(ctx, f.admin, e.ID, f.owner, sales.Accepted)
	if err != nil || accepted.AcceptedAt == nil {
		t.Fatalf("accepted = %+v, %v", accepted, err)
	}
	inv, err := f.svc.BillEstimate(ctx, f.admin, e.ID, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := f.store.Invoice(ctx, f.admin, inv.ID)
	if err != nil || stored.OriginalEstimateID == nil || *stored.OriginalEstimateID != e.ID || stored.Totals.InclTax != 32670 || stored.State != sales.Draft {
		t.Fatalf("invoice = %+v, %v", stored, err)
	}
	if billed, _ := f.store.Estimate(ctx, f.admin, e.ID); billed.State != sales.Billed {
		t.Errorf("estimate = %s", billed.State)
	}
	if list, _ := f.store.EstimatesByID(ctx, f.admin, []resource.ID{e.ID, 1}); len(list) != 1 {
		t.Errorf("by id = %d", len(list))
	}

	// Deleting the estimate keeps the invoice without the link.
	if err := f.svc.DeleteEstimate(ctx, f.admin, e.ID, f.owner); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Estimate(ctx, f.admin, e.ID); err != sales.ErrEstimateNotFound {
		t.Errorf("deleted = %v", err)
	}
	if kept, err := f.store.Invoice(ctx, f.admin, inv.ID); err != nil || kept.OriginalEstimateID != nil {
		t.Errorf("invoice after delete = %+v, %v", kept.OriginalEstimateID, err)
	}
}

func TestEstimateSigningsAreStored(t *testing.T) {
	ctx := context.Background()
	f := newSalesFixture(t)
	e, err := f.svc.CreateEstimate(ctx, f.admin, f.owner, sales.EstimateChanges{
		ContactID: &f.customer, Details: []sales.DetailInput{{Description: ptr("Advice"), Price: ptr("100"), Amount: ptr("1"), TaxRateID: &f.vat21, LedgerAccountID: &f.revenue}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := f.store.EstimateSigning(ctx, f.admin, e.ID); err != nil || ok {
		t.Fatalf("signing before sending = %v, %v", ok, err)
	}
	at := adminNow().UTC().Truncate(time.Second)
	expires := at.Add(time.Hour)
	g := sales.Signing{
		AdministrationID: f.admin, EstimateID: e.ID, ViewCode: "123456789012", ViewCodeExpiresAt: &expires, SentByUserID: &f.owner,
		SenderSignature: []byte("\x89PNG me"), SenderSignedAt: &at, SenderUserID: &f.owner, SignerName: "Jan Voorbeeld", SignerEmail: "jan@example.com",
		SignerIP: "203.0.113.7", SignerUserAgent: "Test", Signature: []byte("\x89PNG you"), AuditTrail: "[]", SignedAt: &at, UpdatedAt: at,
	}
	if err := f.store.SaveSigning(ctx, g); err != nil {
		t.Fatal(err)
	}
	// Found by the estimate alone too, as the contact's online page does.
	for _, admin := range []resource.ID{f.admin, 0} {
		got, ok, err := f.store.EstimateSigning(ctx, admin, e.ID)
		if err != nil || !ok || got.ViewCode != g.ViewCode || !got.ViewCodeExpiresAt.Equal(expires) || *got.SentByUserID != f.owner ||
			string(got.Signature) != string(g.Signature) || string(got.SenderSignature) != string(g.SenderSignature) || got.SignerIP != g.SignerIP || !got.SignedAt.Equal(at) {
			t.Errorf("stored (admin %d) = %+v, %v, %v", admin, got, ok, err)
		}
	}
	if _, ok, _ := f.store.EstimateSigning(ctx, f.admin+1, e.ID); ok {
		t.Error("found from another administration")
	}
}
