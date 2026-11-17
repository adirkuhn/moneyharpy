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

func only(t *testing.T, in string) Statement {
	t.Helper()
	got, _, err := Parse([]byte(in))
	if err != nil || len(got) != 1 {
		t.Fatalf("Parse() = %+v, %v", got, err)
	}
	return got[0]
}

func TestParseMT940BankDialects(t *testing.T) {
	t.Run("ING glues the currency to the IBAN", func(t *testing.T) {
		s := only(t, "{4:\n:20:P261101000000001\n:25:NL20INGB0001234567EUR\n:28C:00000\n:60F:C261031EUR1000,00\n:61:2611011101C250,00NTRFEREF//00000000001\n:86:/EREF/REF-0001//CNTP/NL02ABNA0123456789/ABNANL2A/Example Client B.V.\n/AMSTERDAM/REMI/USTD//Invoice 2026-0001/\n:62F:C261101EUR1250,00\n-}")
		if s.Account != "NL20INGB0001234567" || s.Currency != "EUR" {
			t.Errorf("statement = %+v", s)
		}
		if e := s.Entries[0]; e.TransactionID != "00000000001" || e.ContraName != "Example Client B.V." || e.Message != "Invoice 2026-0001" {
			t.Errorf("entry = %+v", e)
		}
	})
	t.Run("Rabobank's EREF placeholder gives way to the end-to-end reference", func(t *testing.T) {
		s := only(t, ":940:\r\n:20:940S261101\r\n:25:NL91RABO0123456789 EUR\r\n:28C:0\r\n:60F:C261031EUR000000001000,00\r\n:61:261101C000000000250,00N541EREF\r\nREF-0001\r\n:86:/EREF/REF-0001//CNTP/NL02ABNA0123456789/ABNANL2A/Example Client B.V./AMSTERDAM/\r\n/REMI/USTD//Invoice 2026-0001/\r\n:62F:C261101EUR000000001250,00\r\n")
		if s.Account != "NL91RABO0123456789" {
			t.Errorf("account = %q", s.Account)
		}
		if e := s.Entries[0]; e.TransactionID != "REF-0001" || e.ContraAccount != "NL02ABNA0123456789" || e.Message != "Invoice 2026-0001" {
			t.Errorf("entry = %+v", e)
		}
	})
	t.Run("ABN AMRO's account number becomes its IBAN", func(t *testing.T) {
		s := only(t, "ABNANL2A\n940\nABNANL2A\n:20:ABN AMRO BANK NV\n:25:417164300\n:28:10001/1\n:60F:C261031EUR1000,00\n:61:2611011101D10,00N426NONREF\n:86:BEA   NR:12345678 01.11.26/12.30 COFFEE SHOP,PAS123\n:62F:C261101EUR990,00\n-")
		if s.Account != "NL91ABNA0417164300" {
			t.Errorf("account = %q", s.Account)
		}
		if e := s.Entries[0]; e.Amount != -1000 || e.TransactionID != "" || e.Message != "BEA NR:12345678 01.11.26/12.30 COFFEE SHOP,PAS123" {
			t.Errorf("entry = %+v", e)
		}
	})
	t.Run("a day without transactions is a statement too", func(t *testing.T) {
		s := only(t, ":20:STMT-1\n:25:NL91ABNA0417164300\n:28C:1\n:60F:C261031EUR1000,00\n:62F:C261101EUR1000,00\n")
		if s.Account != "NL91ABNA0417164300" || len(s.Entries) != 0 || *s.Balance != 100000 {
			t.Errorf("statement = %+v", s)
		}
	})
}

func TestParseCAMTEncodings(t *testing.T) {
	for _, enc := range []string{"ISO-8859-1", "windows-1252", "ISO-8859-15"} {
		t.Run(enc, func(t *testing.T) {
			s := only(t, `<?xml version="1.0" encoding="`+enc+`"?><Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.02"><BkToCstmrStmt><Stmt><Id>S1</Id><Acct><Id><IBAN>NL91ABNA0417164300</IBAN></Id></Acct><Ntry><Amt Ccy="EUR">10.00</Amt><CdtDbtInd>CRDT</CdtDbtInd><Sts>BOOK</Sts><BookgDt><Dt>2026-11-01</Dt></BookgDt><NtryDtls><TxDtls><RltdPties><Dbtr><Nm>Caf`+"\xe9"+` Voorbeeld</Nm></Dbtr></RltdPties></TxDtls></NtryDtls></Ntry></Stmt></BkToCstmrStmt></Document>`)
			if e := s.Entries[0]; e.ContraName != "Café Voorbeeld" || e.Amount != 1000 {
				t.Errorf("entry = %+v", e)
			}
		})
	}
}

func TestParseKeepsEachAccountsStatement(t *testing.T) {
	got, _, err := Parse([]byte(":20:A\n:25:NL91ABNA0417164300\n:28C:1\n:60F:C261031EUR0,00\n:61:2611011101C1,00NTRFNONREF\n:86:first\n:62F:C261101EUR1,00\n-\n:20:B\n:25:NL20INGB0001234567EUR\n:28C:1\n:60F:C261031EUR0,00\n:61:2611011101C2,00NTRFNONREF\n:86:second\n:62F:C261101EUR2,00\n-"))
	if err != nil || len(got) != 2 {
		t.Fatalf("Parse() = %+v, %v", got, err)
	}
	if got[0].Account != "NL91ABNA0417164300" || got[0].Entries[0].Message != "first" || got[1].Account != "NL20INGB0001234567" || got[1].Entries[0].Message != "second" {
		t.Errorf("statements = %+v", got)
	}
}
