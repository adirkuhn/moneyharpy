package reports

import (
	"testing"
	"time"
)

func TestAuditFileTextIsDutch(t *testing.T) {
	if got := dutchMonth(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)); got != "maart 2026" {
		t.Errorf("month = %q", got)
	}
	for kind, want := range map[string]string{"SalesInvoice": "Verkoopfactuur", "Receipt": "Bon", "PurchaseInvoice": "Inkoopfactuur", "Something": "Something"} {
		if got := documentName(kind); got != want {
			t.Errorf("documentName(%s) = %q, want %q", kind, got, want)
		}
	}
}
