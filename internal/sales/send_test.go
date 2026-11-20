package sales

import (
	"context"
	"testing"
)

func TestACreditInvoicesDocumentNamesWhatItCredits(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := sent(t, h, "2026-11-01")
	credit, err := h.svc.Credit(ctx, adminID, inv.ID, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := h.svc.Document(ctx, adminID, credit.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Credits == nil || doc.Credits.InvoiceID != inv.InvoiceID || doc.Credits.InvoiceDate == nil || !doc.Credits.InvoiceDate.Equal(*inv.InvoiceDate) {
		t.Errorf("credits = %+v, want %s", doc.Credits, inv.InvoiceID)
	}
}
