package bankfile

import (
	"errors"
	"os"
	"testing"
	"time"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func day(s string) time.Time {
	d, _ := time.Parse(time.DateOnly, s)
	return d
}

func TestParseCAMT(t *testing.T) {
	got, format, err := Parse(read(t, "camt053.xml"))
	if err != nil || format != CAMT || len(got) != 1 {
		t.Fatalf("Parse() = %d statements, %s, %v", len(got), format, err)
	}
	s := got[0]
	if s.Account != "NL91ABNA0417164300" || s.Currency != "EUR" || s.Reference != "STMT-2026-11-01" || *s.Balance != 107450 || !s.Date.Equal(day("2026-11-01")) {
		t.Errorf("statement = %+v", s)
	}
	if len(s.Entries) != 2 {
		t.Fatalf("entries = %+v, want the two booked ones", s.Entries)
	}
	in, out := s.Entries[0], s.Entries[1]
	if in.Amount != 100000 || in.ContraName != "Example Client B.V." || in.ContraAccount != "NL02RABO0123456789" || in.Message != "Invoice 2026-0001" || in.TransactionID != "REF-0001" || in.Code != "N544" {
		t.Errorf("incoming = %+v", in)
	}
	if out.Amount != -2550 || out.ContraName != "Hosting Company" || out.ContraAccount != "NL44RABO0123456789" || out.Message != "Monthly hosting plan" || out.TransactionID != "" {
		t.Errorf("outgoing = %+v", out)
	}
}

func TestParseMT940(t *testing.T) {
	got, format, err := Parse(read(t, "mt940.sta"))
	if err != nil || format != MT940 || len(got) != 1 {
		t.Fatalf("Parse() = %d statements, %s, %v", len(got), format, err)
	}
	s := got[0]
	if s.Account != "NL91ABNA0417164300" || s.Currency != "EUR" || s.Reference != "STMT-940-1" || *s.Balance != 106450 {
		t.Errorf("statement = %+v", s)
	}
	if len(s.Entries) != 3 {
		t.Fatalf("entries = %+v", s.Entries)
	}
	in, out, card := s.Entries[0], s.Entries[1], s.Entries[2]
	if in.Amount != 100000 || in.ContraName != "Example Client B.V." || in.ContraAccount != "NL02RABO0123456789" || in.Message != "Invoice 2026-0001" || in.TransactionID != "BANKREF-1" || !in.Date.Equal(day("2026-11-01")) {
		t.Errorf("incoming = %+v", in)
	}
	if out.Amount != -2550 || out.ContraName != "Hosting Company" || out.ContraAccount != "NL44RABO0123456789" || out.Message != "Monthly hosting plan" {
		t.Errorf("outgoing = %+v", out)
	}
	if card.Amount != -1000 || card.Message != "Card payment coffee shop" || card.Currency != "EUR" {
		t.Errorf("card = %+v", card)
	}
}

func TestParseRejectsOtherFiles(t *testing.T) {
	for _, in := range []string{"", "name,amount\nx,1", "<html></html>"} {
		if _, _, err := Parse([]byte(in)); !errors.Is(err, ErrUnknownFormat) && !errors.Is(err, ErrInvalid) {
			t.Errorf("Parse(%q) error = %v", in, err)
		}
	}
	if _, _, err := Parse([]byte("<Document><BkToCstmrStmt><Stmt><Ntry><Amt>x</Amt></Ntry></Stmt></BkToCstmrStmt></Document>")); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad amount error = %v", err)
	}
}
