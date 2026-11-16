package purchases

import "testing"

func TestLabelFollowsTheAdministrationsLanguage(t *testing.T) {
	for _, tc := range []struct {
		kind, reference, language, want string
	}{
		{Receipt, "", "nl", "bon"},
		{Receipt, "", "en", "receipt"},
		{PurchaseInvoice, "", "nl-NL", "inkoopfactuur"},
		{PurchaseInvoice, "", "en", "purchase invoice"},
		{PurchaseInvoice, "F-12", "nl", "F-12"},
	} {
		if got := (Document{Kind: tc.kind, Reference: tc.reference}).label(tc.language); got != tc.want {
			t.Errorf("label(%s %q, %s) = %q, want %q", tc.kind, tc.reference, tc.language, got, tc.want)
		}
	}
}
