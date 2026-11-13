package postgres_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestDuplicateCandidatesShareASupplierOrAFile(t *testing.T) {
	ctx := context.Background()
	f := newPurchaseFixture(t)
	first := f.purchase(t, purchases.PurchaseInvoice, "121", "2026-10-01")
	again := f.purchase(t, purchases.PurchaseInvoice, "121", "2026-10-02")
	scan := []byte("%PDF-1.4 Example Shop B.V. invoice")
	upload, err := f.docs.CreateFromFile(ctx, f.admin, f.owner, purchases.Receipt, "a.pdf", "application/pdf", scan)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := f.docs.CreateTypeless(ctx, f.admin, f.owner, purchases.TypelessChanges{Reference: ptr("b.pdf"), Date: ptr("2026-10-03")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.AddTypelessAttachment(ctx, f.admin, copied.ID, f.owner, "b.pdf", "application/pdf", scan); err != nil {
		t.Fatal(err)
	}
	other, err := f.docs.CreateTypeless(ctx, f.admin, f.owner, purchases.TypelessChanges{Reference: ptr("c.pdf"), Date: ptr("2026-10-03")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.AddTypelessAttachment(ctx, f.admin, other.ID, f.owner, "c.pdf", "application/pdf", []byte("%PDF-1.4 another")); err != nil {
		t.Fatal(err)
	}

	got, err := f.store.DuplicateCandidates(ctx, f.admin, []resource.ID{first.ID, upload.ID})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]resource.ID, len(got))
	for i, c := range got {
		ids[i] = c.ID
	}
	want := []resource.ID{first.ID, again.ID, upload.ID, copied.ID}
	slices.Sort(want)
	if !equalIDs(ids, want) {
		t.Fatalf("candidates = %v, want %v", ids, want)
	}
	c := got[slices.Index(ids, first.ID)]
	if c.Kind != purchases.PurchaseInvoice || c.ContactID != f.supplier || c.Reference != "INV-121" || c.TotalInclTax != 12100 ||
		len(c.Lines) != 1 || c.Lines[0].Description != "Paper" || c.Lines[0].Quantity != 1000 || c.Date.Format("2006-01-02") != "2026-10-01" {
		t.Errorf("first = %+v", c)
	}
	if u := got[slices.Index(ids, upload.ID)]; len(u.Hashes) != 1 || u.Hashes[0] == "" || u.Lines != nil {
		t.Errorf("upload = %+v", u)
	}

	// Through the service, the list names the copies on each row.
	d, err := f.docs.IncomingDocument(ctx, f.admin, again.ID, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.PossibleDuplicates) != 1 || d.PossibleDuplicates[0].ID != first.ID ||
		!slices.Equal(d.PossibleDuplicates[0].Reasons, []string{purchases.SameReference, purchases.SameAmountAndDate, purchases.SameLines}) {
		t.Errorf("possible duplicates = %+v", d.PossibleDuplicates)
	}
	d, err = f.docs.IncomingDocument(ctx, f.admin, copied.ID, f.owner)
	if err != nil || len(d.PossibleDuplicates) != 1 || d.PossibleDuplicates[0].ID != upload.ID {
		t.Errorf("copy = %+v, %v", d.PossibleDuplicates, err)
	}
}

func TestDuplicateCandidatesStayInTheAdministration(t *testing.T) {
	ctx := context.Background()
	f := newPurchaseFixture(t)
	g := newPurchaseFixture(t)
	scan := []byte("%PDF-1.4 the same file elsewhere")
	mine, err := f.docs.CreateFromFile(ctx, f.admin, f.owner, purchases.Receipt, "a.pdf", "application/pdf", scan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.docs.CreateFromFile(ctx, g.admin, g.owner, purchases.Receipt, "a.pdf", "application/pdf", scan); err != nil {
		t.Fatal(err)
	}
	got, err := f.store.DuplicateCandidates(ctx, f.admin, []resource.ID{mine.ID})
	if err != nil || len(got) != 1 || got[0].ID != mine.ID {
		t.Errorf("candidates = %+v, %v", got, err)
	}
}

func TestUnhashedAttachmentsGetASum(t *testing.T) {
	ctx := context.Background()
	f := newPurchaseFixture(t)
	store := postgres.NewAttachmentStore(testPool)
	files := attachments.NewService(store, diskless{}, ids, time.Now)
	a, err := files.Add(ctx, f.admin, purchases.Receipt, 1, "a.pdf", "application/pdf", []byte("%PDF-1.4"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, "UPDATE attachments SET sha256 = '' WHERE id = $1", int64(a.ID)); err != nil {
		t.Fatal(err)
	}
	list, err := store.UnhashedAttachments(ctx, []string{purchases.Receipt}, a.ID-1, 10)
	if err != nil || len(list) != 1 || list[0].ID != a.ID {
		t.Fatalf("unhashed = %+v, %v", list, err)
	}
	if none, _ := store.UnhashedAttachments(ctx, []string{"SalesInvoice"}, a.ID-1, 10); len(none) != 0 {
		t.Errorf("unhashed of another type = %+v", none)
	}
	if err := store.SetAttachmentHash(ctx, f.admin, a.ID, "abc"); err != nil {
		t.Fatal(err)
	}
	// A sum is set once.
	if err := store.SetAttachmentHash(ctx, f.admin, a.ID, "def"); err != nil {
		t.Fatal(err)
	}
	got, err := store.AttachmentsOf(ctx, f.admin, purchases.Receipt, []resource.ID{1})
	if err != nil || len(got) != 1 || got[0].SHA256 != "abc" {
		t.Errorf("attachments = %+v, %v", got, err)
	}
}
