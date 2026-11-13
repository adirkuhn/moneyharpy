package purchases

import (
	"errors"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/google/go-cmp/cmp"
)

func TestParseIncomingFilter(t *testing.T) {
	today := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		filter string
		kinds  []string
		states []string
		period bool
		none   bool
	}{
		{filter: "", period: true},
		{filter: "type:all", period: false},
		{filter: "type:receipt|general_journal_document", kinds: []string{Receipt, journal.GeneralJournalDocument}},
		{filter: "type:purchase_invoice|all", period: false},
		{filter: "type:general_document,state:saved|new,period:this_month", kinds: []string{GeneralDocument}, states: []string{Saved, New}, period: true},
		{filter: "state:all,attachment:with"},
		{filter: "recurring:enabled", none: true},
		{filter: "automatic_saving:automatic", none: true},
	}
	for _, tt := range tests {
		q, err := ParseIncomingFilter(tt.filter, today)
		if err != nil {
			t.Errorf("ParseIncomingFilter(%q) error = %v", tt.filter, err)
			continue
		}
		if !cmp.Equal(q.Kinds, tt.kinds) || !cmp.Equal(q.States, tt.states) || (q.Period != nil) != tt.period || q.None != tt.none {
			t.Errorf("ParseIncomingFilter(%q) = kinds %v, states %v, period %v, none %v", tt.filter, q.Kinds, q.States, q.Period, q.None)
		}
	}
	q, _ := ParseIncomingFilter("", today)
	if q.Period.From != time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) || !q.Today.Equal(today) {
		t.Errorf("default = %+v", q)
	}
	if q, _ := ParseIncomingFilter("attachment:without", today); q.Attachment != "without" {
		t.Errorf("attachment = %q", q.Attachment)
	}
}

func TestParseIncomingFilterRefusesUnknownTerms(t *testing.T) {
	for _, filter := range []string{"type:invoice", "state:draft", "reference:INV-1", "ledger_account_id:1", "updated_after:2026-01-01T00:00:00Z", "color:red", "type"} {
		if _, err := ParseIncomingFilter(filter, now); err == nil {
			t.Errorf("ParseIncomingFilter(%q) accepted", filter)
		} else {
			wantInvalid(t, err, "filter")
		}
	}
}

func TestIncomingListPagesAndFillsTheCurrency(t *testing.T) {
	h := newHarness(t)
	h.store.incoming = []Incoming{
		{ID: 1, AdministrationID: adminID, Kind: Receipt, Currency: "USD"},
		{ID: 2, AdministrationID: adminID, Kind: GeneralDocument},
	}
	q, _ := ParseIncomingFilter("type:receipt|general_document", time.Time{})
	q.PerPage = 500
	list, total, err := h.svc.IncomingList(ctx, adminID, owner, q)
	if err != nil || total != 2 || len(list) != 2 {
		t.Fatalf("IncomingList() = %v, %d, %v", list, total, err)
	}
	if list[0].Currency != "USD" || list[1].Currency != "EUR" {
		t.Errorf("currencies = %q, %q", list[0].Currency, list[1].Currency)
	}
	got := h.store.incomingQuery
	if got.Page != 1 || got.PerPage != MaxPerPage || !got.Today.Equal(time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)) || len(got.Kinds) != 2 {
		t.Errorf("store query = %+v", got)
	}
	if _, _, err := h.svc.IncomingList(ctx, adminID, stranger, q); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("stranger error = %v", err)
	}
}

func TestIncomingDocumentFindsAnyKind(t *testing.T) {
	h := newHarness(t)
	h.store.incoming = []Incoming{{ID: 7, AdministrationID: adminID, Kind: journal.GeneralJournalDocument}}
	d, err := h.svc.IncomingDocument(ctx, adminID, 7, owner)
	if err != nil || d.Currency != "EUR" || d.Ref().URI() != "/100/documents/general_journal_documents/7" {
		t.Fatalf("IncomingDocument() = %+v, %v", d, err)
	}
	if _, err := h.svc.IncomingDocument(ctx, adminID, 8, owner); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing error = %v", err)
	}
	if _, err := h.svc.IncomingDocument(ctx, adminID, 7, stranger); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("stranger error = %v", err)
	}
}

func TestIncomingRefsNameTheTypedResource(t *testing.T) {
	want := map[string]resource.Type{
		PurchaseInvoice: resource.TypePurchaseInvoice, Receipt: resource.TypeReceipt, GeneralDocument: resource.TypeGeneralDocument,
		journal.GeneralJournalDocument: resource.TypeGeneralJournalDocument, TypelessDocument: resource.TypeTypelessDocument,
	}
	for kind, typ := range want {
		if got := (Incoming{ID: 1, AdministrationID: 2, Kind: kind}).Ref(); got.Type != typ {
			t.Errorf("%s ref type = %s, want %s", kind, got.Type, typ)
		}
	}
}

func TestIncomingStatisticsSelectEverything(t *testing.T) {
	h := newHarness(t)
	h.store.statistics = IncomingStatistics{CostsExclTax: 1000, AmountPayable: 1210, Categories: []CostCategory{{LedgerAccountID: costs, Name: "Costs", Amount: 1000}}}
	q, _ := ParseIncomingFilter("state:open", time.Time{})
	q.Page, q.PerPage = 3, 10
	st, err := h.svc.IncomingStatistics(ctx, adminID, owner, q)
	if err != nil || st.Currency != "EUR" || st.CostsExclTax != 1000 || len(st.Categories) != 1 {
		t.Fatalf("IncomingStatistics() = %+v, %v", st, err)
	}
	if got := h.store.incomingQuery; got.PerPage != 0 || got.Today.IsZero() {
		t.Errorf("store query = %+v", got)
	}
}

func TestIncomingDocumentsNamePossibleDuplicates(t *testing.T) {
	h := newHarness(t)
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	h.store.incoming = []Incoming{
		{ID: 1, AdministrationID: adminID, Kind: PurchaseInvoice, Reference: "INV-0042", Date: day},
		{ID: 2, AdministrationID: adminID, Kind: TypelessDocument, Reference: "scan.pdf", Date: day},
		{ID: 3, AdministrationID: adminID, Kind: journal.GeneralJournalDocument, Date: day},
	}
	h.store.candidates = []DuplicateCandidate{
		{ID: 1, Kind: PurchaseInvoice, Reference: "INV-0042", Date: day, Hashes: []string{"aa"}},
		{ID: 2, Kind: TypelessDocument, Reference: "scan.pdf", Date: day, Hashes: []string{"bb"}},
		{ID: 9, Kind: Receipt, Reference: "Old", Date: day.AddDate(0, -1, 0), TotalInclTax: 500, Hashes: []string{"aa"}},
	}
	q, _ := ParseIncomingFilter("", time.Time{})
	list, _, err := h.svc.IncomingList(ctx, adminID, owner, q)
	if err != nil {
		t.Fatalf("IncomingList() error = %v", err)
	}
	want := []Duplicate{{ID: 9, Kind: Receipt, Reference: "Old", Date: day.AddDate(0, -1, 0), TotalInclTax: 500, Reasons: []string{SameFile}}}
	if !cmp.Equal(list[0].PossibleDuplicates, want) || list[1].PossibleDuplicates != nil || list[2].PossibleDuplicates != nil {
		t.Errorf("possible duplicates = %+v, %+v, %+v", list[0].PossibleDuplicates, list[1].PossibleDuplicates, list[2].PossibleDuplicates)
	}
	// Journal entries are no upload; they are not compared.
	if got := h.store.candidateIDs; !cmp.Equal(got, []resource.ID{1, 2}) {
		t.Errorf("compared ids = %v", got)
	}
	d, err := h.svc.IncomingDocument(ctx, adminID, 1, owner)
	if err != nil || !cmp.Equal(d.PossibleDuplicates, want) {
		t.Errorf("IncomingDocument() = %+v, %v", d.PossibleDuplicates, err)
	}
}
