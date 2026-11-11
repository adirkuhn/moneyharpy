package sepa

import (
	"errors"
	"strings"
	"testing"
	"time"

	gocmp "github.com/google/go-cmp/cmp"
)

func sample() Transfer {
	return Transfer{
		MessageID: "MH-1", Created: time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC), InitiatorName: "Example Shop B.V.",
		Batches: []Batch{{
			ID: "MH-1-1", ExecutionDate: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC), DebtorName: "Example Shop B.V.",
			DebtorIBAN: "NL91ABNA0417164300", DebtorBIC: "ABNANL2A", Country: "NL",
			Transactions: []Transaction{
				{InstructionID: "1", EndToEndID: "E2E-1", Amount: 12100, Currency: "EUR", CreditorName: "Supplier B.V.", CreditorIBAN: "NL44RABO0123456789", CreditorBIC: "RABONL2U", Remittance: "INV-1"},
				{EndToEndID: "E2E-2", Amount: 5050, Currency: "EUR", CreditorName: "Other B.V.", CreditorIBAN: "NL20INGB0001234567"},
			},
		}},
	}
}

func TestWriteThenParseRoundTrips(t *testing.T) {
	raw, err := Write(sample())
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	text := string(raw)
	for _, want := range []string{Namespace, "<NbOfTxs>2</NbOfTxs>", "<CtrlSum>171.50</CtrlSum>", "<InstdAmt Ccy=\"EUR\">121.00</InstdAmt>", "<Dt>2026-11-12</Dt>", "<BICFI>ABNANL2A</BICFI>"} {
		if !strings.Contains(text, want) {
			t.Errorf("Write() lacks %q:\n%s", want, text)
		}
	}
	got, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if diff := gocmp.Diff(sample(), got); diff != "" {
		t.Errorf("round trip diff (-want +got):\n%s", diff)
	}
}

func TestParseReadsVersion03(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.001.001.03">
  <CstmrCdtTrfInitn>
    <GrpHdr><MsgId>OLD-1</MsgId><CreDtTm>2026-11-11T09:00:00</CreDtTm><NbOfTxs>1</NbOfTxs><CtrlSum>10.00</CtrlSum><InitgPty><Nm>Example Shop B.V.</Nm></InitgPty></GrpHdr>
    <PmtInf>
      <PmtInfId>OLD-1-1</PmtInfId><PmtMtd>TRF</PmtMtd><NbOfTxs>1</NbOfTxs><CtrlSum>10.00</CtrlSum>
      <ReqdExctnDt>2026-11-13</ReqdExctnDt>
      <Dbtr><Nm>Example Shop B.V.</Nm></Dbtr>
      <DbtrAcct><Id><IBAN>NL91 ABNA 0417 1643 00</IBAN></Id></DbtrAcct>
      <DbtrAgt><FinInstnId><BIC>ABNANL2A</BIC></FinInstnId></DbtrAgt>
      <CdtTrfTxInf>
        <PmtId><EndToEndId>NOTPROVIDED</EndToEndId></PmtId>
        <Amt><InstdAmt Ccy="EUR">10.00</InstdAmt></Amt>
        <Cdtr><Nm>Supplier B.V.</Nm></Cdtr>
        <CdtrAcct><Id><IBAN>NL44RABO0123456789</IBAN></Id></CdtrAcct>
        <RmtInf><Ustrd>INV-9</Ustrd></RmtInf>
      </CdtTrfTxInf>
    </PmtInf>
  </CstmrCdtTrfInitn>
</Document>`
	got, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	b := got.Batches[0]
	if b.DebtorIBAN != "NL91ABNA0417164300" || b.DebtorBIC != "ABNANL2A" || b.ExecutionDate.Format(time.DateOnly) != "2026-11-13" {
		t.Errorf("batch = %+v", b)
	}
	tx := b.Transactions[0]
	if tx.Amount != 1000 || tx.EndToEndID != "" || tx.Remittance != "INV-9" {
		t.Errorf("transaction = %+v", tx)
	}
}

func TestParseRefusesOtherFiles(t *testing.T) {
	for name, raw := range map[string]string{
		"not xml":      "hello",
		"direct debit": `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.008.001.02"><CstmrDrctDbtInitn/></Document>`,
		"no payments":  `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.001.001.09"><CstmrCdtTrfInitn><GrpHdr><MsgId>x</MsgId></GrpHdr></CstmrCdtTrfInitn></Document>`,
		"bad amount":   `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.001.001.09"><CstmrCdtTrfInitn><PmtInf><ReqdExctnDt><Dt>2026-01-01</Dt></ReqdExctnDt><CdtTrfTxInf><Amt><InstdAmt Ccy="EUR">x</InstdAmt></Amt></CdtTrfTxInf></PmtInf></CstmrCdtTrfInitn></Document>`,
		"no execution": `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.001.001.09"><CstmrCdtTrfInitn><PmtInf></PmtInf></CstmrCdtTrfInitn></Document>`,
		"no creditor":  `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.001.001.09"><CstmrCdtTrfInitn><PmtInf><ReqdExctnDt><Dt>2026-01-01</Dt></ReqdExctnDt><CdtTrfTxInf><Amt><InstdAmt Ccy="EUR">1.00</InstdAmt></Amt></CdtTrfTxInf></PmtInf></CstmrCdtTrfInitn></Document>`,
		"no transfers": `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.001.001.09"><CstmrCdtTrfInitn><PmtInf><ReqdExctnDt><Dt>2026-01-01</Dt></ReqdExctnDt></PmtInf></CstmrCdtTrfInitn></Document>`,
	} {
		if _, err := Parse([]byte(raw)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: Parse() error = %v, want ErrInvalid", name, err)
		}
	}
}
