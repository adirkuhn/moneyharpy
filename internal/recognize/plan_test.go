package recognize

import (
	"slices"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

var testRates = []bookkeeping.TaxRate{
	{ID: 1, Percentage: "21.0", Type: "purchase_invoice", ShowTax: true, Active: true},
	{ID: 2, Percentage: "9.0", Type: "purchase_invoice", ShowTax: true, Active: true},
	{ID: 3, Percentage: "0.0", Type: "purchase_invoice", ShowTax: true, Active: true},
	{ID: 4, Percentage: "21.0", Type: "purchase_invoice", ShowTax: false, Active: true}, // reverse charged
	{ID: 5, Percentage: "21.0", Type: "purchase_invoice", ShowTax: true, Active: true, Country: "DE"},
	{ID: 6, Percentage: "21.0", Type: "sales_invoice", ShowTax: true, Active: true},
	{ID: 7, Percentage: "9.0", Type: "purchase_invoice", ShowTax: true, Active: false},
}

func certain[T any](v T) Value[T]   { return Value[T]{V: v, Found: true, Confidence: Certain} }
func uncertain[T any](v T) Value[T] { return Value[T]{V: v, Found: true, Confidence: Uncertain} }

func TestMapTaxRate(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want resource.ID
		ok   bool
	}{
		{"21", 1, true}, {"21.00", 1, true}, {"9", 2, true}, {"9,0", 2, true}, {"0", 3, true}, {"6", 0, false}, {"abc", 0, false},
	} {
		got, ok := MapTaxRate(tt.in, testRates)
		if got != tt.want || ok != tt.ok {
			t.Errorf("MapTaxRate(%q) = %d, %v; want %d, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
	if got, ok := MapTaxRate("21", testRates[3:]); !ok || got != 5 {
		t.Errorf("MapTaxRate without a domestic rate = %d, %v; want the foreign one before the reverse-charged one", got, ok)
	}
	if got := defaultRate(testRates); got != 1 {
		t.Errorf("defaultRate() = %d, want the standard rate", got)
	}
}

func fieldNames(fs []Field) map[string]Confidence {
	out := map[string]Confidence{}
	for _, f := range fs {
		out[f.Name] = f.Confidence
	}
	return out
}

func TestPlanFillsATypelessDocumentFromItsLines(t *testing.T) {
	e := Extraction{
		Source: SourceText, Kind: certain(KindReceipt), Reference: certain("2026-0042"), Date: certain(day("2026-11-02")),
		DueDate: certain(day("2026-11-30")), Currency: certain("eur"), PricesInclTax: certain(false),
		Lines: []Line{
			{Description: "Hosting November", Quantity: certain("2"), Price: certain(money.Amount(1000)), TaxRate: certain("21")},
			{Description: "Setup", Total: uncertain(money.Amount(500)), TaxRate: uncertain("9")},
			{Description: "Without an amount"},
		},
	}
	mem := Memory{Lines: map[string]Category{"hosting": {LedgerAccountID: 70, TaxRateID: 2, Uses: 2}}}
	p := MakePlan(PlanInput{
		Extraction: e, Target: Target{Kind: purchases.TypelessDocument, Date: day("2026-11-13")},
		Supplier: SupplierMatch{ContactID: 9, Name: "Example Shop B.V.", Confidence: Certain}, Memory: mem, Rates: testRates,
	})
	if p.Kind != purchases.Receipt {
		t.Errorf("Kind = %q", p.Kind)
	}
	c := p.Changes
	if *c.ContactID != 9 || *c.Reference != "2026-0042" || *c.Date != "2026-11-02" || *c.DueDate != "2026-11-30" || *c.Currency != "EUR" || *c.PricesAreInclTax {
		t.Errorf("changes = %+v", c)
	}
	if len(c.Details) != 2 {
		t.Fatalf("details = %+v", c.Details)
	}
	d0, d1 := c.Details[0], c.Details[1]
	if *d0.Description != "Hosting November" || *d0.Amount != "2" || *d0.Price != "10.0" || *d0.TaxRateID != 1 || *d0.LedgerAccountID != 70 {
		t.Errorf("line 0 = %+v", d0)
	}
	if *d1.Amount != "1" || *d1.Price != "5.0" || *d1.TaxRateID != 2 || d1.LedgerAccountID != nil {
		t.Errorf("line 1 = %+v", d1)
	}
	got := fieldNames(p.Fields)
	want := map[string]Confidence{
		FieldDocumentType: Certain, FieldContactID: Certain, FieldReference: Certain, FieldDate: Certain, FieldDueDate: Certain,
		FieldCurrency: Certain, FieldPricesAreInclTax: Certain,
		"details.0.description": Certain, "details.0.amount": Certain, "details.0.price": Certain, "details.0.tax_rate_id": Certain,
		"details.0.ledger_account_id": Certain,
		"details.1.description":       Uncertain, "details.1.amount": Uncertain, "details.1.price": Uncertain, "details.1.tax_rate_id": Uncertain,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("field %s = %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("fields = %v", got)
	}
}

func TestPlanMakesLinesOfTheVATSummaryOrTheTotal(t *testing.T) {
	summary := Extraction{
		TotalInclTax: certain(money.Amount(13180)), PricesInclTax: certain(true),
		VAT: []VAT{{Rate: "21", Taxable: 10000, Tax: 2100}, {Rate: "9", Taxable: 1000, Tax: 90}},
	}
	p := MakePlan(PlanInput{Extraction: summary, Target: Target{Kind: purchases.TypelessDocument}, Supplier: SupplierMatch{Name: "Example Shop"}, Rates: testRates})
	if len(p.Changes.Details) != 2 || *p.Changes.Details[0].Price != "121.0" || *p.Changes.Details[0].Description != "Example Shop" ||
		*p.Changes.Details[1].Price != "10.9" || *p.Changes.Details[1].TaxRateID != 2 {
		t.Errorf("summary lines = %+v", p.Changes.Details)
	}

	total := Extraction{TotalInclTax: uncertain(money.Amount(4250))}
	mem := Memory{Kind: Learned[Kind]{V: KindReceipt, Uses: 1}, Lines: map[string]Category{"": {LedgerAccountID: 80, TaxRateID: 2, Uses: 1}}}
	p = MakePlan(PlanInput{Extraction: total, Target: Target{Kind: purchases.TypelessDocument}, Memory: mem, Rates: testRates})
	d := p.Changes.Details
	if len(d) != 1 || *d[0].Price != "42.5" || *d[0].Description != fallbackDescription || *d[0].TaxRateID != 2 || *d[0].LedgerAccountID != 80 ||
		!*p.Changes.PricesAreInclTax || p.Kind != purchases.Receipt {
		t.Errorf("total line = %+v, plan %+v", d, p)
	}
	f := fieldNames(p.Fields)
	if f[FieldDocumentType] != Uncertain || f["details.0.ledger_account_id"] != Uncertain || f["details.0.tax_rate_id"] != Uncertain {
		t.Errorf("fields = %v", f)
	}

	// Nothing about the kind: a purchase invoice, uncertain; no lines
	// without amounts.
	p = MakePlan(PlanInput{Extraction: Extraction{Reference: certain("A1")}, Target: Target{Kind: purchases.TypelessDocument}, Rates: testRates})
	if p.Kind != purchases.PurchaseInvoice || fieldNames(p.Fields)[FieldDocumentType] != Uncertain || p.Changes.Details != nil {
		t.Errorf("plan = %+v", p)
	}
}

func TestPlanKeepsWhatAUserFilledIn(t *testing.T) {
	e := Extraction{Reference: certain("R-2"), Date: certain(day("2026-10-01")), Currency: certain("USD"), TotalInclTax: certain(money.Amount(100))}
	target := Target{
		Kind: purchases.PurchaseInvoice, ContactID: 3, Reference: "Typed by hand", Date: day("2026-11-13"), Currency: "EUR",
		Lines: []sales.Detail{{ID: 50, Description: "By hand", Price: 100}},
	}
	// The first reading fills in the defaults, not what was typed.
	p := MakePlan(PlanInput{Extraction: e, Target: target, Supplier: SupplierMatch{ContactID: 4, Confidence: Certain}, Rates: testRates})
	c := p.Changes
	if c.ContactID != nil || c.Reference != nil || c.Details != nil || *c.Date != "2026-10-01" || *c.Currency != "USD" {
		t.Errorf("first reading changes = %+v", c)
	}
	if p.Kind != purchases.PurchaseInvoice || slices.ContainsFunc(p.Fields, func(f Field) bool { return f.Name == FieldDocumentType }) {
		t.Errorf("a purchase invoice keeps its kind: %+v", p)
	}

	// Reading again replaces what was filled in before, lines too, but
	// not a date the user changed since.
	prev := Recognition{State: StateDone, Fields: []Field{{Name: FieldReference}, {Name: "details.0.price"}}}
	p = MakePlan(PlanInput{Extraction: e, Target: target, Previous: prev, Rates: testRates})
	c = p.Changes
	if *c.Reference != "R-2" || c.Date != nil || c.Currency != nil || len(c.Details) != 2 || !c.Details[0].Destroy || *c.Details[0].ID != 50 {
		t.Errorf("second reading changes = %+v", c)
	}
}

func TestPlanDropsADueDateBeforeTheDate(t *testing.T) {
	e := Extraction{Date: certain(day("2026-10-10")), DueDate: certain(day("2026-10-01"))}
	p := MakePlan(PlanInput{Extraction: e, Target: Target{Kind: purchases.TypelessDocument}})
	if p.Changes.DueDate != nil {
		t.Errorf("due date = %v", *p.Changes.DueDate)
	}
}

// reverseRates adds the reverse-charged purchase rates of the Dutch VAT
// return: EU acquisitions (box 4b) and domestic reverse charge (box 2a).
var reverseRates = append([]bookkeeping.TaxRate{
	{ID: 10, Percentage: "21.0", Type: "purchase_invoice", Active: true, ReportBoxes: []string{"NL/4b", "NL/5b"}},
	{ID: 11, Percentage: "9.0", Type: "purchase_invoice", Active: true, ReportBoxes: []string{"NL/4b", "NL/5b"}},
	{ID: 12, Percentage: "21.0", Type: "purchase_invoice", Active: true, ReportBoxes: []string{"NL/2a", "NL/5b"}},
}, testRates...)

func TestPlanBooksADiscountedLineWithItsTotal(t *testing.T) {
	e := Extraction{
		Source: SourceText, TotalInclTax: certain(money.Amount(12894)), TotalExclTax: certain(money.Amount(12894)),
		Lines: []Line{
			{Description: "Produto Exemplo A", Quantity: certain("20"), Total: certain(money.Amount(9894)), TaxRate: certain("0")},
			{Description: "Transporte", Quantity: certain("1"), Price: certain(money.Amount(3000)), Total: certain(money.Amount(3000)), TaxRate: certain("0")},
		},
	}
	p := MakePlan(PlanInput{Extraction: e, Target: Target{Kind: purchases.TypelessDocument}, Rates: testRates})
	d := p.Changes.Details
	if len(d) != 2 || *d[0].Amount != "1" || *d[0].Price != "98.94" || *d[0].Description != "20 × Produto Exemplo A" ||
		*d[1].Amount != "1" || *d[1].Price != "30.0" || *d[1].Description != "Transporte" {
		t.Errorf("details = %+v", d)
	}
}

func TestPlanBooksAnIntraCommunitySupplyReverseCharged(t *testing.T) {
	supply := func(country string) Extraction {
		return Extraction{
			Source: SourceText, Supplier: Supplier{Country: country},
			TotalInclTax: certain(money.Amount(10000)), TotalExclTax: certain(money.Amount(10000)),
			VAT:   []VAT{{Rate: "0", Taxable: 10000, ReverseCharge: true}},
			Lines: []Line{{Description: "Goods", Quantity: certain("1"), Price: certain(money.Amount(10000)), Total: certain(money.Amount(10000)), TaxRate: certain("0")}},
		}
	}
	for country, want := range map[string]resource.ID{"PT": 10, "NL": 12} {
		p := MakePlan(PlanInput{Extraction: supply(country), Target: Target{Kind: purchases.TypelessDocument}, Rates: reverseRates})
		if d := p.Changes.Details; len(d) != 1 || *d[0].TaxRateID != want {
			t.Errorf("%s supplier: details = %+v, want tax rate %d", country, d, want)
		}
		if f := fieldNames(p.Fields); f["details.0.tax_rate_id"] != Uncertain {
			t.Errorf("%s supplier: the reverse-charged rate is a guess, fields %v", country, f)
		}
	}
	// Without such rates: plain 0%.
	p := MakePlan(PlanInput{Extraction: supply("PT"), Target: Target{Kind: purchases.TypelessDocument}, Rates: testRates})
	if d := p.Changes.Details; len(d) != 1 || *d[0].TaxRateID != 3 {
		t.Errorf("details = %+v, want the 0%% rate", d)
	}
}
