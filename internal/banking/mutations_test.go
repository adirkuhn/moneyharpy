package banking

import (
	"context"
	"errors"
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
	versions, err := svc.MutationVersions(ctx, adminID, owner)
	if err != nil || len(versions) != 2 || versions[0].Version != now.Unix() {
		t.Errorf("versions = %+v, %v", versions, err)
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
