package recognize

import (
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

func ledgerPtr(id resource.ID) *resource.ID { return &id }

func TestNormalizeDescription(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"Hosting November 2026", "hosting"},
		{"Hosting december 2026 (2x)", "hosting"},
		{"Office supplies: paper A4", "office supplies paper"},
		{"12-2026", ""},
	} {
		if got := NormalizeDescription(tt.in); got != tt.want {
			t.Errorf("NormalizeDescription(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMemoryLearnsAndBecomesCertain(t *testing.T) {
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	doc := purchases.Document{Kind: purchases.Receipt, Currency: "EUR", PricesAreInclTax: true, Lines: []sales.Detail{
		{Description: "Hosting November 2026", Price: 1000, TaxRateID: 21, LedgerAccountID: ledgerPtr(7)},
		{Description: "Domain name", Price: 200, TaxRateID: 21, LedgerAccountID: ledgerPtr(8)},
		{Description: "Not categorised", Price: 9999, TaxRateID: 21},
	}}
	var m Memory
	m.Learn(doc, at)
	if m.Kind.V != KindReceipt || m.Kind.Confidence() != Uncertain || m.Currency.V != "EUR" || !m.PricesInclTax.V {
		t.Fatalf("memory after one document = %+v", m)
	}
	c, ok := m.Category("Hosting December 2026")
	if !ok || c.LedgerAccountID != 7 || c.TaxRateID != 21 || c.Confidence() != Uncertain {
		t.Errorf("Category(hosting) = %+v, %v", c, ok)
	}
	if def, ok := m.Category("Something else"); !ok || def.LedgerAccountID != 7 {
		t.Errorf("default category = %+v, %v; want the largest categorised line's", def, ok)
	}

	m.Learn(doc, at.Add(time.Hour))
	if m.Kind.Confidence() != Certain || m.Kind.Uses != 2 {
		t.Errorf("kind after two documents = %+v", m.Kind)
	}
	if c, _ := m.Category("hosting"); c.Confidence() != Certain {
		t.Errorf("category after two documents = %+v", c)
	}

	// A different choice starts counting again.
	doc.Kind = purchases.PurchaseInvoice
	doc.Lines[0].LedgerAccountID = ledgerPtr(9)
	m.Learn(doc, at.Add(2*time.Hour))
	if m.Kind.V != KindPurchaseInvoice || m.Kind.Uses != 1 {
		t.Errorf("kind after a change = %+v", m.Kind)
	}
	if c, _ := m.Category("hosting"); c.LedgerAccountID != 9 || c.Uses != 1 {
		t.Errorf("category after a change = %+v", c)
	}
}

func TestMemoryForgetsLeastRecentlyUsedLines(t *testing.T) {
	var m Memory
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range maxMemoryLines + 5 {
		desc := "item " + string(rune('a'+i%26)) + string(rune('a'+i/26%26))
		m.Learn(purchases.Document{Lines: []sales.Detail{{Description: desc, Price: 1, TaxRateID: 1, LedgerAccountID: ledgerPtr(1)}}}, at.Add(time.Duration(i)*time.Minute))
	}
	if len(m.Lines) != maxMemoryLines+1 {
		t.Errorf("remembered %d lines, want %d plus the default", len(m.Lines), maxMemoryLines)
	}
	if _, ok := m.Lines["item aa"]; ok {
		t.Error("the oldest line is still remembered")
	}
}
