package postgres_test

import (
	"context"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/sales"
)

func TestSentInvoicesKeepTheirPaymentReference(t *testing.T) {
	ctx := context.Background()
	f := newSalesFixture(t)
	inv := f.invoice(t, "121", "2026-10-01")
	if !sales.ValidRF(inv.PaymentReference) {
		t.Fatalf("reference = %q", inv.PaymentReference)
	}
	if got, err := f.svc.Invoice(ctx, f.admin, inv.ID, f.owner); err != nil || got.PaymentReference != inv.PaymentReference {
		t.Errorf("stored reference = %q, %v", got.PaymentReference, err)
	}
	if taken, err := f.store.PaymentReferenceTaken(ctx, f.admin, inv.PaymentReference); err != nil || !taken {
		t.Errorf("PaymentReferenceTaken(own) = %v, %v", taken, err)
	}
	if taken, err := f.store.PaymentReferenceTaken(ctx, f.admin, sales.NewPaymentReference()); err != nil || taken {
		t.Errorf("PaymentReferenceTaken(new) = %v, %v", taken, err)
	}
	if taken, err := f.store.PaymentReferenceTaken(ctx, f.admin, ""); err != nil || taken {
		t.Errorf("PaymentReferenceTaken(empty) = %v, %v", taken, err)
	}
}
