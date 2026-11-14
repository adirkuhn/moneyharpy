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
	doc := purchases.Document{ID: 41, Kind: purchases.Receipt, Currency: "EUR", PricesAreInclTax: true, Lines: []sales.Detail{
		{Description: "Hosting November 2026", Price: 1000, TaxRateID: 21, LedgerAccountID: ledgerPtr(7)},
		{Description: "Domain name", Price: 200, TaxRateID: 21, LedgerAccountID: ledgerPtr(8)},
		{Description: "Not categorised", Price: 9999, TaxRateID: 21},
	}}
	var m Memory
	m.Learn(doc, at)
	receipts := m.Recall(KindReceipt)
	if m.Kind.V != KindReceipt || m.Kind.Confidence() != Uncertain || receipts.Currency().V != "EUR" || !receipts.PricesInclTax().V {
		t.Fatalf("memory after one document = %+v", m)
	}
	if got := receipts.Currency(); got.Confidence() != Uncertain || got.SourceDocumentID != 41 {
		t.Errorf("currency = %+v", got)
	}
	c, ok := receipts.Category("Hosting December 2026")
	if !ok || c.LedgerAccountID != 7 || c.TaxRateID != 21 || c.Confidence() != Uncertain || c.SourceDocumentID != 41 {
		t.Errorf("Category(hosting) = %+v, %v", c, ok)
	}
	if def, ok := receipts.Category("Something else"); !ok || def.LedgerAccountID != 7 {
		t.Errorf("default category = %+v, %v; want the largest categorised line's", def, ok)
	}

	doc.ID = 42
	m.Learn(doc, at.Add(time.Hour))
	if m.Kind.Confidence() != Certain || m.Kind.Uses != 2 {
		t.Errorf("kind after two documents = %+v", m.Kind)
	}
	if c, _ := m.Recall(KindReceipt).Category("hosting"); c.Confidence() != Certain || c.SourceDocumentID != 42 {
		t.Errorf("category after two documents = %+v", c)
	}

	// A purchase invoice is remembered apart: it leaves what the receipts
	// taught alone, and starts counting of its own.
	doc.ID, doc.Kind = 43, purchases.PurchaseInvoice
	doc.Lines[0].LedgerAccountID = ledgerPtr(9)
	m.Learn(doc, at.Add(2*time.Hour))
	if m.Kind.V != KindPurchaseInvoice || m.Kind.Uses != 1 {
		t.Errorf("kind after a change = %+v", m.Kind)
	}
	if c, _ := m.Recall(KindPurchaseInvoice).Category("hosting"); c.LedgerAccountID != 9 || c.Uses != 1 || c.SourceDocumentID != 43 {
		t.Errorf("purchase invoice category = %+v", c)
	}
	if c, _ := m.Recall(KindReceipt).Category("hosting"); c.LedgerAccountID != 7 || c.Confidence() != Certain {
		t.Errorf("receipt category after a purchase invoice = %+v", c)
	}

	// A different choice for the same kind starts counting again.
	doc.ID = 44
	doc.Lines[0].LedgerAccountID = ledgerPtr(10)
	m.Learn(doc, at.Add(3*time.Hour))
	if c, _ := m.Recall(KindPurchaseInvoice).Category("hosting"); c.LedgerAccountID != 10 || c.Uses != 1 {
		t.Errorf("category after a change = %+v", c)
	}
}

func TestRecallFallsBackToTheOtherKindUncertain(t *testing.T) {
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	receipt := purchases.Document{ID: 51, Kind: purchases.Receipt, Currency: "EUR", PricesAreInclTax: true, Lines: []sales.Detail{
		{Description: "Coffee beans", Price: 1000, TaxRateID: 9, LedgerAccountID: ledgerPtr(7)},
	}}
	var m Memory
	m.Learn(receipt, at)
	receipt.ID = 52
	m.Learn(receipt, at.Add(time.Hour))

	// Nothing was learned of purchase invoices: the receipts tell, but
	// never for certain.
	invoices := m.Recall(KindPurchaseInvoice)
	c, ok := invoices.Category("Coffee beans")
	if !ok || c.LedgerAccountID != 7 || c.TaxRateID != 9 || c.Confidence() != Uncertain || c.SourceDocumentID != 52 {
		t.Errorf("Category() from the other kind = %+v, %v", c, ok)
	}
	if cur := invoices.Currency(); cur.V != "EUR" || cur.Confidence() != Uncertain || cur.SourceDocumentID != 52 {
		t.Errorf("Currency() from the other kind = %+v", cur)
	}
	if incl := invoices.PricesInclTax(); !incl.V || incl.Confidence() != Uncertain {
		t.Errorf("PricesInclTax() from the other kind = %+v", incl)
	}

	// Once purchase invoices were learned, theirs come first: their own
	// description, then the other kind's same description, then their
	// default.
	invoice := purchases.Document{ID: 53, Kind: purchases.PurchaseInvoice, Currency: "USD", Lines: []sales.Detail{
		{Description: "Hosting", Price: 5000, TaxRateID: 21, LedgerAccountID: ledgerPtr(8)},
	}}
	m.Learn(invoice, at.Add(2*time.Hour))
	invoices = m.Recall(KindPurchaseInvoice)
	if c, _ := invoices.Category("Hosting"); c.LedgerAccountID != 8 || c.SourceDocumentID != 53 {
		t.Errorf("own description = %+v", c)
	}
	if c, _ := invoices.Category("Coffee beans"); c.LedgerAccountID != 7 || c.Confidence() != Uncertain {
		t.Errorf("the other kind's description = %+v", c)
	}
	if c, _ := invoices.Category("Something new"); c.LedgerAccountID != 8 {
		t.Errorf("own default = %+v", c)
	}
	if cur := invoices.Currency(); cur.V != "USD" {
		t.Errorf("own currency = %+v", cur)
	}

	var empty Memory
	if _, ok := empty.Recall(KindReceipt).Category("Anything"); ok {
		t.Error("an empty memory recalls a category")
	}
	if empty.Recall(KindReceipt).Currency().Known() {
		t.Error("an empty memory recalls a currency")
	}
}

func TestMemoryForgetsLeastRecentlyUsedLines(t *testing.T) {
	var m Memory
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range maxMemoryLines + 5 {
		desc := "item " + string(rune('a'+i%26)) + string(rune('a'+i/26%26))
		m.Learn(purchases.Document{Kind: purchases.PurchaseInvoice, Lines: []sales.Detail{{Description: desc, Price: 1, TaxRateID: 1, LedgerAccountID: ledgerPtr(1)}}}, at.Add(time.Duration(i)*time.Minute))
	}
	if n := len(m.Invoices.Lines); n != maxMemoryLines+1 {
		t.Errorf("remembered %d lines, want %d plus the default", n, maxMemoryLines)
	}
	if _, ok := m.Invoices.Lines["item aa"]; ok {
		t.Error("the oldest line is still remembered")
	}
}
