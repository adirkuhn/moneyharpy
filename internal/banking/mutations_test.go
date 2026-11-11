package banking

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestBookingSplitsAndSettles(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	acc := bank(t, svc)
	m := transactions(t, svc, acc, "-95")[0]
	got, err := svc.LinkBooking(ctx, adminID, m.ID, owner, Link{Type: "LedgerAccount", BookingID: ptr(rentCategory), Price: ptr("100"), Description: "Rent"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Open() != 500 || got.State != Unprocessed {
		t.Errorf("after 100: open %s, %s", got.Open(), got.State)
	}
	got, err = svc.LinkBooking(ctx, adminID, m.ID, owner, Link{Type: "LedgerAccount", BookingID: ptr(rentCategory), Price: ptr("-5")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Open() != 0 || got.State != Processed || got.ProcessedAt == nil || len(got.Bookings) != 2 {
		t.Errorf("after -5: %+v", got)
	}
	got, err = svc.UnlinkBooking(ctx, adminID, m.ID, owner, "LedgerAccountBooking", got.Bookings[1].ID)
	if err != nil || got.State != Unprocessed || got.Open() != 500 {
		t.Errorf("unlink = %+v, %v", got, err)
	}
	// Without a price, what is open is booked.
	got, err = svc.LinkBooking(ctx, adminID, m.ID, owner, Link{Type: "LedgerAccount", BookingID: ptr(rentCategory)})
	if err != nil || got.State != Processed {
		t.Errorf("book the rest = %+v, %v", got, err)
	}
}

func TestBookingRefusals(t *testing.T) {
	svc, _, ledger := newTestService()
	ctx := context.Background()
	acc := bank(t, svc)
	m := transactions(t, svc, acc, "10")[0]
	if _, err := svc.LinkBooking(ctx, adminID, m.ID, owner, Link{Type: "SalesInvoice", BookingID: ptr(rentCategory)}); !errors.Is(err, ErrBookingUnavailable) {
		t.Errorf("SalesInvoice without invoices error = %v", err)
	}
	if _, err := svc.LinkBooking(ctx, adminID, m.ID, owner, Link{Type: "ExternalSalesInvoice", BookingID: ptr(rentCategory)}); !errors.Is(err, ErrBookingUnavailable) {
		t.Errorf("ExternalSalesInvoice error = %v", err)
	}
	if _, err := svc.LinkBooking(ctx, adminID, m.ID, owner, Link{Type: "Nope"}); fieldsOf(t, err)["booking_type"] == nil {
		t.Error("accepted an unknown booking type")
	}
	if _, err := svc.LinkBooking(ctx, adminID, m.ID, owner, Link{Type: "LedgerAccount", BookingID: ptr(closedCategory)}); fieldsOf(t, err)["booking_id"] == nil {
		t.Error("booked on a deactivated category")
	}
	if _, err := svc.LinkBooking(ctx, adminID, m.ID, owner, Link{Type: "LedgerAccount", BookingID: ptr(rentCategory), Price: ptr("0")}); fieldsOf(t, err)["price_base"] == nil {
		t.Error("booked zero")
	}
	if _, err := svc.UnlinkBooking(ctx, adminID, m.ID, owner, "Payment", 1); !errors.Is(err, ErrBookingUnavailable) {
		t.Errorf("unlink Payment error = %v", err)
	}
	if _, err := svc.UnlinkBooking(ctx, adminID, m.ID, owner, "LedgerAccountBooking", 1); !errors.Is(err, ErrBookingNotFound) {
		t.Errorf("unlink unknown error = %v", err)
	}
	ledger.locked = "2026-11-01"
	if _, err := svc.LinkBooking(ctx, adminID, m.ID, owner, Link{Type: "LedgerAccount", BookingID: ptr(rentCategory)}); fieldsOf(t, err)["date"] == nil {
		t.Error("booked in the locked period")
	}
}

func TestMutationFilter(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	acc := bank(t, svc)
	ms := transactions(t, svc, acc, "10", "-20", "30")
	if _, err := svc.LinkBooking(ctx, adminID, ms[2].ID, owner, Link{Type: "LedgerAccount", BookingID: ptr(rentCategory)}); err != nil {
		t.Fatal(err)
	}
	today := now
	for raw, want := range map[string]int{
		"":                                    3,
		"state:unprocessed":                   2,
		"state:processed,period:this_month":   1,
		"mutation_type:debit":                 1,
		"mutation_type:credit,amount_from:15": 1,
		"amount_to:20":                        2,
		"period:prev_month":                   0,
		"financial_account_id:" + acc.ID.String() + ",state:unprocessed|processed": 3,
	} {
		f, err := ParseMutationFilter(raw, today)
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		got, err := svc.Mutations(ctx, adminID, owner, f)
		if err != nil || len(got) != want {
			t.Errorf("%q: %d mutations, %v; want %d", raw, len(got), err, want)
		}
	}
	for _, raw := range []string{"colour:red", "state:lost", "period:yesterday", "amount_from:x", "financial_account_id:abc", "nokey"} {
		if _, err := ParseMutationFilter(raw, today); fieldsOf(t, err)["filter"] == nil {
			t.Errorf("%q accepted", raw)
		}
	}
}

func TestSynchronizationAndBalances(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	acc := bank(t, svc)
	ms := transactions(t, svc, acc, "10", "-2.5")
	thisYear, _ := ParseMutationFilter("", now)
	versions, err := svc.MutationVersions(ctx, adminID, owner, thisYear)
	if err != nil || len(versions) != 2 || versions[0].Version != now.Unix() {
		t.Errorf("versions = %+v, %v", versions, err)
	}
	credit, _ := ParseMutationFilter("period:this_month,mutation_type:credit", now)
	if versions, err := svc.MutationVersions(ctx, adminID, owner, credit); err != nil || len(versions) != 1 || versions[0].ID != ms[0].ID {
		t.Errorf("credit versions = %+v, %v", versions, err)
	}
	got, err := svc.MutationsByID(ctx, adminID, owner, []resource.ID{ms[0].ID, 1})
	if err != nil || len(got) != 1 {
		t.Errorf("by id = %d, %v", len(got), err)
	}
	balances, total, err := svc.Balances(ctx, adminID, owner)
	if err != nil || len(balances) != 1 || total != 750 || balances[0].Account.ID != acc.ID {
		t.Errorf("balances = %+v, total %s, %v", balances, total, err)
	}
}

func TestBalanceReport(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	acc := bank(t, svc)
	transactions(t, svc, acc, "10", "-2.5")

	got, err := svc.BalanceReport(ctx, adminID, owner, "", "")
	if err != nil || got.Total != 750 || len(got.Balances) != 1 || got.Period.Monthly() != "202611..202611" || got.Periods != nil {
		t.Errorf("this month = %+v, %v", got, err)
	}
	if got, _ := svc.BalanceReport(ctx, adminID, owner, "202610", ""); len(got.Balances) != 0 || got.Total != 0 {
		t.Errorf("before the transactions = %+v", got)
	}
	got, err = svc.BalanceReport(ctx, adminID, owner, "202610..202611", "month")
	if err != nil || got.Total != 750 || len(got.Periods) != 2 {
		t.Fatalf("by month = %+v, %v", got, err)
	}
	// The account has a balance at some end, so every period lists it.
	if p := got.Periods[0]; p.Period.Monthly() != "202610..202610" || len(p.Balances) != 1 || p.Total != 0 {
		t.Errorf("october = %+v", p)
	}
	if p := got.Periods[1]; p.Total != 750 {
		t.Errorf("november = %+v", p)
	}
	for _, in := range [][2]string{{"202501..202602", ""}, {"20251102..20251130", ""}, {"nonsense", ""}, {"", "week"}} {
		if _, err := svc.BalanceReport(ctx, adminID, owner, in[0], in[1]); err == nil {
			t.Errorf("BalanceReport(%q, %q) accepted", in[0], in[1])
		}
	}
}

func TestSummaryCountsWhatIsLeftToLink(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	acc := bank(t, svc)
	ms := transactions(t, svc, acc, "10", "-2.5", "4")
	if _, err := svc.LinkBooking(ctx, adminID, ms[1].ID, owner, Link{Type: "LedgerAccount", BookingID: ptr(rentCategory)}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Summary(ctx, adminID, owner, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got.Balance != 1150 || got.Currency != "EUR" || got.Unprocessed != 2 {
		t.Errorf("summary = %+v", got)
	}
	if len(got.RecentlyProcessed) != 1 || got.RecentlyProcessed[0].ID != ms[1].ID {
		t.Errorf("recently processed = %+v", got.RecentlyProcessed)
	}
	if _, err := svc.Summary(ctx, adminID, stranger, 10); err == nil {
		t.Error("a stranger got a summary")
	}
}

func TestLinkingInvoicePayments(t *testing.T) {
	svc, store, ledger := newTestService()
	invoices := &fakeInvoices{store: store, unpaid: map[resource.ID]Amount{9001: 6000, 9002: 10000}}
	svc.UseInvoices(invoices)
	ctx := context.Background()
	acc := bank(t, svc)
	m := transactions(t, svc, acc, "100")[0]
	got, err := svc.LinkBooking(ctx, adminID, m.ID, owner, Link{Type: "SalesInvoice", BookingID: ptr(resourceID(9001))})
	if err != nil {
		t.Fatal(err)
	}
	// The invoice had 60.00 unpaid; 40.00 stays open.
	if got.Open() != 4000 || got.State != Unprocessed || len(got.Payments) != 1 {
		t.Errorf("after first invoice: open %s, %s", got.Open(), got.State)
	}
	id, err := svc.LinkInvoicePayment(ctx, adminID, m.ID, owner, "SalesInvoice", 9002, ptr(Amount(2000)))
	if err != nil || id == 0 {
		t.Fatalf("link = %d, %v", id, err)
	}
	// An external invoice takes the rest.
	got, err = svc.LinkBooking(ctx, adminID, m.ID, owner, Link{Type: "ExternalSalesInvoice", BookingID: ptr(resourceID(9002))})
	if err != nil {
		t.Fatal(err)
	}
	if got.Open() != 0 || got.State != Processed || got.Payments[2].InvoiceType != "ExternalSalesInvoice" {
		t.Errorf("after external invoice: open %s, %s, %+v", got.Open(), got.State, got.Payments)
	}
	if _, err := svc.LinkBooking(ctx, adminID, m.ID, owner, Link{Type: "SalesInvoice", BookingID: ptr(resourceID(9001))}); fieldsOf(t, err)["price_base"] == nil {
		t.Error("linked a transaction with nothing open")
	}
	if err := svc.DeleteStatement(ctx, adminID, m.StatementID, owner); !errors.Is(err, ErrStatementBooked) {
		t.Errorf("deleted a statement paying invoices: %v", err)
	}
	if _, err := svc.UnlinkBooking(ctx, adminID, m.ID, owner, "Payment", 1); !errors.Is(err, ErrBookingNotFound) {
		t.Errorf("unlink unknown payment = %v", err)
	}
	ledger.locked = "2026-11-30"
	if _, err := svc.UnlinkBooking(ctx, adminID, m.ID, owner, "Payment", id); fieldsOf(t, err)["date"] == nil {
		t.Error("unlinked in the locked period")
	}
	ledger.locked = ""
	if err := svc.UnlinkPayment(ctx, adminID, m.ID, owner, id); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.Mutation(ctx, adminID, m.ID, owner)
	if got.Open() != 2000 || got.State != Unprocessed || invoices.unpaid[9002] != 8000 {
		t.Errorf("after unlink: open %s, %s", got.Open(), got.State)
	}
	// An outgoing transaction pays back a credit invoice: the price is in
	// the transaction's direction, the payment signed like the invoice.
	out := transactions(t, svc, acc, "-25")[0]
	invoices.unpaid[9003] = -2500
	got, err = svc.LinkBooking(ctx, adminID, out.ID, owner, Link{Type: "SalesInvoice", BookingID: ptr(resourceID(9003)), Price: ptr("25")})
	if err != nil || got.State != Processed || got.Payments[0].Price != -2500 {
		t.Errorf("credit = %+v, %v", got, err)
	}
	if ledgerID, err := svc.AccountLedger(ctx, adminID, acc.ID, owner); err != nil || ledgerID != acc.LedgerAccountID {
		t.Errorf("account ledger = %d, %v", ledgerID, err)
	}
}

func TestLinkingPurchaseDocuments(t *testing.T) {
	svc, store, _ := newTestService()
	sales := &fakeInvoices{store: store, unpaid: map[resource.ID]Amount{}}
	documents := &fakeInvoices{store: store, unpaid: map[resource.ID]Amount{9101: 6000}}
	svc.UseInvoices(sales)
	ctx := context.Background()
	acc := bank(t, svc)
	out := transactions(t, svc, acc, "-100")[0]
	if _, err := svc.LinkBooking(ctx, adminID, out.ID, owner, Link{Type: DocumentType, BookingID: ptr(resourceID(9101))}); !errors.Is(err, ErrBookingUnavailable) {
		t.Errorf("Document without documents error = %v", err)
	}
	svc.UseDocuments(documents)
	// Money out pays a purchase invoice: the payment is signed like the
	// document, positive, and takes from what is open of the transaction.
	got, err := svc.LinkBooking(ctx, adminID, out.ID, owner, Link{Type: DocumentType, BookingID: ptr(resourceID(9101))})
	if err != nil {
		t.Fatal(err)
	}
	if got.Payments[0].Price != 6000 || got.Open() != -4000 || got.State != Unprocessed {
		t.Errorf("after the document: payments %+v, open %s", got.Payments, got.Open())
	}
	documents.unpaid[9102] = 10000
	got, err = svc.LinkBooking(ctx, adminID, out.ID, owner, Link{Type: DocumentType, BookingID: ptr(resourceID(9102)), Price: ptr("40")})
	if err != nil || got.Open() != 0 || got.State != Processed || got.Payments[1].Price != 4000 {
		t.Fatalf("second document = %+v, %v", got, err)
	}
	if _, err := svc.UnlinkBooking(ctx, adminID, out.ID, owner, "Payment", got.Payments[0].ID); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.Mutation(ctx, adminID, out.ID, owner)
	if got.Open() != -6000 || documents.unpaid[9101] != 6000 || len(sales.unpaid) != 0 {
		t.Errorf("after unlink: open %s, unpaid %v", got.Open(), documents.unpaid)
	}
}

func TestLinkingNewPurchaseDocuments(t *testing.T) {
	svc, store, _ := newTestService()
	documents := &fakeInvoices{store: store, unpaid: map[resource.ID]Amount{}}
	ctx := context.Background()
	acc := bank(t, svc)
	out := transactions(t, svc, acc, "-121")[0]
	if _, err := svc.LinkBooking(ctx, adminID, out.ID, owner, Link{Type: "NewReceipt"}); !errors.Is(err, ErrBookingUnavailable) {
		t.Errorf("NewReceipt without purchases error = %v", err)
	}
	purchases := &fakePurchases{documents: documents}
	svc.UseDocuments(documents)
	svc.UsePurchases(purchases)
	svc.UseProjects(fakeProjects{})
	if _, err := svc.LinkBooking(ctx, adminID, out.ID, owner, Link{Type: "NewReceipt", BookingID: ptr(resourceID(1))}); fieldsOf(t, err)["booking_id"] == nil {
		t.Error("accepted a booking_id for a new receipt")
	}
	if _, err := svc.LinkBooking(ctx, adminID, out.ID, owner, Link{Type: "NewReceipt", Price: ptr("-21")}); fieldsOf(t, err)["price_base"] == nil {
		t.Error("accepted an amount against the transaction's direction")
	}
	if _, err := svc.LinkBooking(ctx, adminID, out.ID, owner, Link{Type: "NewReceipt", Price: ptr("200")}); fieldsOf(t, err)["price_base"] == nil {
		t.Error("accepted more than the transaction has open")
	}
	if _, err := svc.LinkBooking(ctx, adminID, out.ID, owner, Link{Type: "NewReceipt", ProjectID: ptr(resourceID(1))}); fieldsOf(t, err)["project_id"] == nil {
		t.Error("accepted an unknown project")
	}
	// Money out of 121 makes a receipt of 21 and a purchase invoice of the
	// rest, each paid with the transaction.
	got, err := svc.LinkBooking(ctx, adminID, out.ID, owner, Link{Type: "NewReceipt", Price: ptr("21"), ProjectID: ptr(resourceID(9900))})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Payments) != 1 || got.Payments[0].InvoiceType != DocumentType || got.Payments[0].Price != 2100 || got.Open() != -10000 {
		t.Errorf("after the receipt: %+v, open %s", got.Payments, got.Open())
	}
	got, err = svc.LinkBooking(ctx, adminID, out.ID, owner, Link{Type: "NewPurchaseInvoice"})
	if err != nil || got.State != Processed || len(got.Payments) != 2 || got.Payments[1].Price != 10000 {
		t.Fatalf("purchase invoice = %+v, %v", got, err)
	}
	want := []NewDocument{
		{Kind: "Receipt", Date: out.Date, Amount: 2100, Description: "Payment -121", ContraName: "Hosting Company", ProjectID: ptr(resourceID(9900))},
		{Kind: "PurchaseInvoice", Date: out.Date, Amount: 10000, Description: "Payment -121", ContraName: "Hosting Company"},
	}
	if !reflect.DeepEqual(want, purchases.created) {
		t.Errorf("created documents = %+v, want %+v", purchases.created, want)
	}
}

func TestLinkingPurchaseTransactions(t *testing.T) {
	svc, store, _ := newTestService()
	documents := &fakeInvoices{store: store, unpaid: map[resource.ID]Amount{9101: 6000, 9102: 4000}}
	svc.UseDocuments(documents)
	svc.UsePurchases(&fakePurchases{documents: documents})
	ctx := context.Background()
	acc := bank(t, svc)
	ms := transactions(t, svc, acc, "-60", "-100", "-50")
	if _, err := svc.LinkBooking(ctx, adminID, ms[0].ID, owner, Link{Type: "PurchaseTransaction"}); fieldsOf(t, err)["booking_id"] == nil {
		t.Error("linked without a booking_id")
	}
	if _, err := svc.LinkBooking(ctx, adminID, ms[0].ID, owner, Link{Type: "PurchaseTransaction", BookingID: ptr(resourceID(1))}); fieldsOf(t, err)["booking_id"] == nil {
		t.Error("linked an unknown purchase transaction")
	}
	// The batch pays 100, more than the transaction of 50 has open.
	if _, err := svc.LinkBooking(ctx, adminID, ms[2].ID, owner, Link{Type: "PurchaseTransactionBatch", BookingID: ptr(resourceID(9300))}); fieldsOf(t, err)["price_base"] == nil {
		t.Error("linked a batch larger than the transaction")
	}
	// The amount comes from the batch; price_base is ignored.
	got, err := svc.LinkBooking(ctx, adminID, ms[1].ID, owner, Link{Type: "PurchaseTransactionBatch", BookingID: ptr(resourceID(9300)), Price: ptr("1")})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != Processed || len(got.Payments) != 2 || got.Payments[0].InvoiceID != 9101 || got.Payments[1].Price != 4000 {
		t.Errorf("batch = %+v", got)
	}
	// Its transactions are paid now.
	if _, err := svc.LinkBooking(ctx, adminID, ms[0].ID, owner, Link{Type: "PurchaseTransaction", BookingID: ptr(resourceID(9201))}); fieldsOf(t, err)["booking_id"] == nil {
		t.Error("linked a paid purchase transaction")
	}
}

func TestBookingOnAProject(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	acc := bank(t, svc)
	m := transactions(t, svc, acc, "-10")[0]
	svc.UseProjects(fakeProjects{})
	if _, err := svc.LinkBooking(ctx, adminID, m.ID, owner, Link{Type: "LedgerAccount", BookingID: ptr(rentCategory), ProjectID: ptr(resourceID(1))}); fieldsOf(t, err)["project_id"] == nil {
		t.Error("booked on an unknown project")
	}
	got, err := svc.LinkBooking(ctx, adminID, m.ID, owner, Link{Type: "LedgerAccount", BookingID: ptr(rentCategory), ProjectID: ptr(resourceID(9900))})
	if err != nil || got.Bookings[0].ProjectID == nil || *got.Bookings[0].ProjectID != 9900 {
		t.Errorf("booking = %+v, %v", got.Bookings, err)
	}
}
