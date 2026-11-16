package banking

import (
	"testing"
	"time"
)

func TestSuggestRanksWhatTheTransactionPays(t *testing.T) {
	day := func(s string) *time.Time { d, _ := time.Parse(time.DateOnly, s); return &d }
	// 51.06 comes in, naming the invoice's payment reference.
	in := Mutation{Amount: 5106, Date: *day("2026-09-25"), Message: "Payment RF18 0000 0000 0000 01 order", ContraName: "Example Client BV"}
	candidates := []Candidate{
		{Type: "SalesInvoice", ID: 1, Open: 4130, Number: "2026-0007", PaymentReference: "RF71 0000 0000 0000 02", ContactName: "Other Customer B.V.", Date: day("2026-09-24")},
		{Type: "SalesInvoice", ID: 2, Open: 5106, Number: "2026-0008", PaymentReference: "RF18 0000 0000 0000 01", ContactName: "Example Client B.V.", Date: day("2026-09-24")},
		{Type: "Document", ID: 3, Open: 2510, Reference: "PUR-9", ContactName: "Curtain Shop", Date: day("2026-09-23")},
		{Type: "LedgerAccount", ID: 4, Text: "Revenue"},
	}
	got := Suggest(in, candidates, "", 0)
	if got[0].ID != 2 || got[0].Price != 5106 {
		t.Fatalf("first = %+v", got[0])
	}
	// An invoice for more than came in gets what came in; a purchase document,
	// paid the wrong way, a negative price.
	for _, s := range got {
		switch s.ID {
		case 1:
			if s.Price != 4130 {
				t.Errorf("smaller invoice price = %s", s.Price)
			}
		case 3:
			if s.Price != -2510 {
				t.Errorf("document price = %s", s.Price)
			}
		case 4:
			if s.Price != 5106 {
				t.Errorf("category price = %s", s.Price)
			}
		}
	}
	// A search keeps what holds every word.
	if found := Suggest(in, candidates, "other 2026", 0); len(found) != 1 || found[0].ID != 1 {
		t.Errorf("search = %+v", found)
	}
	if found := Suggest(in, candidates, "rf71 0000 00", 0); len(found) != 1 || found[0].ID != 1 {
		t.Errorf("compact search = %+v", found)
	}
	if limited := Suggest(in, candidates, "", 2); len(limited) != 2 {
		t.Errorf("limit = %d", len(limited))
	}
}

func TestSuggestPrefersTheAmountAndCounterpartyWithoutAReference(t *testing.T) {
	d, _ := time.Parse(time.DateOnly, "2026-09-23")
	out := Mutation{Amount: -3900, Date: d, Message: "Card payment", ContraName: "Example Shop 2186"}
	got := Suggest(out, []Candidate{
		{Type: "Document", ID: 1, Open: 80753, Reference: "583", ContactName: "Other Supplier", Date: &d},
		{Type: "Document", ID: 2, Open: 3900, Reference: "REC 23-09", ContactName: "Example Shop", Date: &d},
	}, "", 0)
	if got[0].ID != 2 || got[0].Price != 3900 || got[1].Price != 3900 {
		t.Errorf("ranked = %+v", got)
	}
}
