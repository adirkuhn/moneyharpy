package sales

import "testing"

func TestLabelFollowsTheInvoicesLanguage(t *testing.T) {
	for _, tc := range []struct {
		inv  Invoice
		want string
	}{
		{Invoice{DraftID: 3, Language: "nl"}, "concept 3"},
		{Invoice{DraftID: 3, Language: "en"}, "draft 3"},
		{Invoice{DraftID: 3, InvoiceID: "2026-0001", Language: "nl"}, "2026-0001"},
		{Invoice{Type: ExternalEntityType, Language: "nl"}, "externe factuur"},
		{Invoice{Type: ExternalEntityType, Language: "en"}, "external invoice"},
		{Invoice{Type: ExternalEntityType, Reference: "X-1", Language: "nl"}, "X-1"},
	} {
		if got := tc.inv.label(); got != tc.want {
			t.Errorf("label(%+v) = %q, want %q", tc.inv, got, tc.want)
		}
	}
}

func TestFileNamesFollowTheDocumentsLanguage(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{fileName(Invoice{InvoiceID: "2026/0001", Language: "nl"}, "pdf"), "factuur-2026-0001.pdf"},
		{fileName(Invoice{DraftID: 2, Language: "en"}, "xml"), "invoice-draft-2.xml"},
		{fileName(Invoice{DraftID: 2, Language: "nl"}, "pdf"), "factuur-concept-2.pdf"},
		{packingSlipFileName(Invoice{InvoiceID: "2026-0001", Language: "nl"}), "pakbon-2026-0001.pdf"},
		{packingSlipFileName(Invoice{InvoiceID: "2026-0001", Language: "en"}), "packing-slip-2026-0001.pdf"},
		{estimateFileName(Estimate{EstimateID: "2026-0027", Language: "nl"}, "pdf"), "offerte-2026-0027.pdf"},
		{estimateFileName(Estimate{DraftID: 4, Language: "en"}, "pdf"), "quote-draft-4.pdf"},
	} {
		if tc.got != tc.want {
			t.Errorf("file name = %q, want %q", tc.got, tc.want)
		}
	}
}
