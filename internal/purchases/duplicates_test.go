package purchases

import (
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/google/go-cmp/cmp"
)

func TestDuplicateReasons(t *testing.T) {
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	const shop, other resource.ID = 7, 8
	lines := func(n int) []DuplicateLine {
		out := make([]DuplicateLine, n)
		for i := range out {
			out[i] = DuplicateLine{Description: "Item " + string(rune('A'+i)), Price: money.Amount(100 * (i + 1)), Quantity: 1000}
		}
		return out
	}
	base := DuplicateCandidate{ID: 1, Kind: PurchaseInvoice, ContactID: shop, Reference: "INV-2026-0042", Date: day, TotalInclTax: 12100}
	tests := []struct {
		name string
		a, b func(*DuplicateCandidate)
		want []string
	}{
		{name: "nothing alike", a: func(d *DuplicateCandidate) {}, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax, d.Date = "X-77", 500, day.AddDate(0, 2, 0)
		}},
		{name: "same file, whatever else", a: func(d *DuplicateCandidate) { d.Hashes = []string{"aa", "bb"} }, b: func(d *DuplicateCandidate) {
			d.Kind, d.ContactID, d.Reference, d.TotalInclTax, d.Hashes = TypelessDocument, 0, "scan.pdf", 0, []string{"cc", "bb"}
		}, want: []string{SameFile}},
		{name: "no hashes are not the same file", a: func(d *DuplicateCandidate) { d.Reference, d.TotalInclTax = "A-1", 1 }, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax, d.Hashes = "B-2", 2, nil
		}},
		{name: "same reference and amount and date", a: func(d *DuplicateCandidate) {}, b: func(d *DuplicateCandidate) {}, want: []string{SameReference, SameAmountAndDate}},
		{name: "reference written differently", a: func(d *DuplicateCandidate) {}, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax = "inv 2026/0042", 1
		}, want: []string{SameReference}},
		{name: "reference inside the other", a: func(d *DuplicateCandidate) {}, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax = "Factuur INV-2026-0042 (kopie)", 1
		}, want: []string{SameReference}},
		{name: "number inside the reference", a: func(d *DuplicateCandidate) { d.Reference = "Invoice 2026-0042" }, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax = "0042", 1
		}, want: []string{SameReference}},
		{name: "too short to contain", a: func(d *DuplicateCandidate) { d.Reference = "Invoice 42" }, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax = "42", 1
		}},
		{name: "a number cut short is another number", a: func(d *DuplicateCandidate) { d.Reference = "2026-12" }, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax = "2026-1", 1
		}},
		{name: "typo in the letters", a: func(d *DuplicateCandidate) { d.Reference = "Factuur 2026-0042" }, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax = "Fatcuur 2026-0042", 1
		}, want: []string{SameReference}},
		{name: "the next invoice number is not a typo", a: func(d *DuplicateCandidate) {}, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax = "INV-2026-0043", 1
		}},
		{name: "short references must be equal", a: func(d *DuplicateCandidate) { d.Reference = "AB12" }, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax = "AC12", 1
		}},
		{name: "a reference without a number is a description", a: func(d *DuplicateCandidate) { d.Reference = "Lunch" }, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax = "lunch", 1
		}},
		{name: "a file name is no reference", a: func(d *DuplicateCandidate) { d.Reference = "invoice-0042.pdf" }, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax = "invoice-0042.pdf", 1
		}},
		{name: "same reference of another supplier", a: func(d *DuplicateCandidate) {}, b: func(d *DuplicateCandidate) { d.ContactID = other }},
		{name: "same reference without a supplier", a: func(d *DuplicateCandidate) { d.ContactID = 0 }, b: func(d *DuplicateCandidate) { d.ContactID = 0 }},
		{name: "same amount a week apart", a: func(d *DuplicateCandidate) {}, b: func(d *DuplicateCandidate) {
			d.Reference, d.Date = "Other 9", day.AddDate(0, 0, -7)
		}, want: []string{SameAmountAndDate}},
		{name: "same amount eight days apart", a: func(d *DuplicateCandidate) {}, b: func(d *DuplicateCandidate) {
			d.Reference, d.Date = "Other 9", day.AddDate(0, 0, 8)
		}},
		{name: "zero amounts are not the same amount", a: func(d *DuplicateCandidate) { d.TotalInclTax = 0 }, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax = "Other 9", 0
		}},
		{name: "mostly the same lines", a: func(d *DuplicateCandidate) { d.Lines = lines(5) }, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax, d.Date = "Other 9", 1, day.AddDate(0, 1, 0)
			d.Lines = lines(5)
			d.Lines[4].Price = 1
			d.Lines[0].Description = " item a. "
		}, want: []string{SameLines}},
		{name: "too few lines alike", a: func(d *DuplicateCandidate) { d.Lines = lines(5) }, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax, d.Lines = "Other 9", 1, lines(5)
			d.Lines[3].Quantity, d.Lines[4].Price = 2000, 1
		}},
		{name: "lines of a longer document", a: func(d *DuplicateCandidate) { d.Lines = lines(4) }, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax, d.Lines = "Other 9", 1, lines(23)
		}},
		{name: "same lines of another supplier", a: func(d *DuplicateCandidate) { d.Lines = lines(3) }, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax, d.ContactID, d.Lines = "Other 9", 1, other, lines(3)
		}},
		{name: "a repeated line counts once per copy", a: func(d *DuplicateCandidate) {
			d.Lines = []DuplicateLine{{Description: "Box", Price: 5, Quantity: 1000}, {Description: "Box", Price: 5, Quantity: 1000}}
		}, b: func(d *DuplicateCandidate) {
			d.Reference, d.TotalInclTax = "Other 9", 1
			d.Lines = []DuplicateLine{{Description: "Box", Price: 5, Quantity: 1000}, {Description: "Tape", Price: 5, Quantity: 1000}}
		}},
		{name: "all four", a: func(d *DuplicateCandidate) { d.Hashes, d.Lines = []string{"aa"}, lines(23) }, b: func(d *DuplicateCandidate) {
			d.Hashes, d.Lines = []string{"aa"}, lines(23)
		}, want: []string{SameFile, SameReference, SameAmountAndDate, SameLines}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := base, base
			b.ID = 2
			tt.a(&a)
			tt.b(&b)
			if got := duplicateReasons(a, b); !cmp.Equal(got, tt.want) {
				t.Errorf("duplicateReasons(a, b) = %v, want %v", got, tt.want)
			}
			if got := duplicateReasons(b, a); !cmp.Equal(got, tt.want) {
				t.Errorf("duplicateReasons(b, a) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPossibleDuplicates(t *testing.T) {
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	upload := func(id resource.ID, date time.Time) DuplicateCandidate {
		return DuplicateCandidate{ID: id, Kind: PurchaseInvoice, ContactID: 7, Reference: "INV-0042", Date: date, TotalInclTax: 12100, Hashes: []string{"aa"}}
	}
	pool := []DuplicateCandidate{
		upload(1, day), upload(2, day.AddDate(0, 0, 1)), upload(3, day),
		{ID: 4, Kind: TypelessDocument, Reference: "copy.pdf", Date: day, Hashes: []string{"aa"}},
		{ID: 5, Kind: Receipt, ContactID: 7, Reference: "R-1", Date: day, TotalInclTax: 500},
	}
	got := PossibleDuplicates([]resource.ID{1, 4, 5, 99}, pool)
	want := map[resource.ID][]Duplicate{
		1: {
			{ID: 3, Kind: PurchaseInvoice, Reference: "INV-0042", Date: day, TotalInclTax: 12100, Reasons: []string{SameFile, SameReference, SameAmountAndDate}},
			{ID: 4, Kind: TypelessDocument, Reference: "copy.pdf", Date: day, Reasons: []string{SameFile}},
			{ID: 2, Kind: PurchaseInvoice, Reference: "INV-0042", Date: day.AddDate(0, 0, 1), TotalInclTax: 12100, Reasons: []string{SameFile, SameReference, SameAmountAndDate}},
		},
		4: {
			{ID: 1, Kind: PurchaseInvoice, Reference: "INV-0042", Date: day, TotalInclTax: 12100, Reasons: []string{SameFile}},
			{ID: 3, Kind: PurchaseInvoice, Reference: "INV-0042", Date: day, TotalInclTax: 12100, Reasons: []string{SameFile}},
			{ID: 2, Kind: PurchaseInvoice, Reference: "INV-0042", Date: day.AddDate(0, 0, 1), TotalInclTax: 12100, Reasons: []string{SameFile}},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("PossibleDuplicates() mismatch (-want +got):\n%s", diff)
	}
}

func TestIsFileName(t *testing.T) {
	for ref, want := range map[string]bool{"scan.pdf": true, " IMG_0001.JPG ": true, "INV-2026.0042": false, "a/b.pdf": false, "": false} {
		if got := IsFileName(ref); got != want {
			t.Errorf("IsFileName(%q) = %v, want %v", ref, got, want)
		}
	}
}
