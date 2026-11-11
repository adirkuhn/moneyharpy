// Package sepa reads and writes SEPA credit transfer files (ISO 20022
// pain.001), the files a bank accepts to pay many transfers at once.
package sepa

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

// Namespace is the version MoneyHarpy writes, as Moneybird does.
const Namespace = "urn:iso:std:iso:20022:tech:xsd:pain.001.001.09"

// ErrInvalid reports a file that is not a credit transfer initiation.
var ErrInvalid = errors.New("not a SEPA credit transfer file")

// Transfer is a credit transfer initiation: payment batches from the
// accounts of one initiator.
type Transfer struct {
	MessageID     string
	Created       time.Time
	InitiatorName string
	Batches       []Batch
}

// Batch is a payment information block: transfers from one account on one
// day.
type Batch struct {
	ID            string
	ExecutionDate time.Time
	DebtorName    string
	DebtorIBAN    string
	DebtorBIC     string
	Country       string
	Transactions  []Transaction
}

// Transaction is one transfer to a creditor.
type Transaction struct {
	InstructionID string
	EndToEndID    string
	// Amount is positive, in cents.
	Amount       money.Amount
	Currency     string
	CreditorName string
	CreditorIBAN string
	CreditorBIC  string
	// Remittance is the unstructured message, such as an invoice number.
	Remittance string
}

// Total is what the batch transfers.
func (b Batch) Total() money.Amount {
	var sum money.Amount
	for _, t := range b.Transactions {
		sum += t.Amount
	}
	return sum
}

func decimal(a money.Amount) string {
	sign := ""
	if a < 0 {
		sign, a = "-", -a
	}
	return fmt.Sprintf("%s%d.%02d", sign, a/100, a%100)
}

// Write renders the transfer as pain.001.001.09.
func Write(t Transfer) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	enc := xml.NewEncoder(&b)
	enc.Indent("", "  ")
	var count int
	var sum money.Amount
	for _, batch := range t.Batches {
		count += len(batch.Transactions)
		sum += batch.Total()
	}
	doc := document{
		XMLNS: Namespace, XSI: "http://www.w3.org/2001/XMLSchema-instance",
		Initiation: initiation{
			Header: header{MessageID: t.MessageID, Created: t.Created.Format(time.RFC3339), Count: count, Sum: decimal(sum), Initiator: party{Name: t.InitiatorName}},
		},
	}
	for i, batch := range t.Batches {
		country := batch.Country
		if country == "" && len(batch.DebtorIBAN) >= 2 {
			country = batch.DebtorIBAN[:2]
		}
		id := batch.ID
		if id == "" {
			id = fmt.Sprintf("%s-%d", t.MessageID, i+1)
		}
		p := paymentInfo{
			ID: id, Method: "TRF", BatchBooking: "true", Count: len(batch.Transactions), Sum: decimal(batch.Total()),
			TypeInfo:      &typeInfo{ServiceLevel: code{Code: "SEPA"}},
			ExecutionDate: &dateChoice{Date: batch.ExecutionDate.Format(time.DateOnly)},
			Debtor:        party{Name: batch.DebtorName, Country: country},
			DebtorAccount: account{ID: accountID{IBAN: batch.DebtorIBAN}},
			DebtorAgent:   agent{Institution: institution{BICFI: batch.DebtorBIC}},
		}
		if batch.DebtorBIC == "" {
			p.DebtorAgent.Institution.Other = &other{ID: "NOTPROVIDED"}
		}
		for _, tx := range batch.Transactions {
			currency := tx.Currency
			if currency == "" {
				currency = "EUR"
			}
			out := creditTransfer{
				PaymentID: paymentID{InstructionID: tx.InstructionID, EndToEndID: firstOf(tx.EndToEndID, "NOTPROVIDED")},
				Amount:    amount{Instructed: instructed{Currency: currency, Value: decimal(tx.Amount)}},
				Creditor:  party{Name: tx.CreditorName}, CreditorAccount: account{ID: accountID{IBAN: tx.CreditorIBAN}},
			}
			if tx.CreditorBIC != "" {
				out.CreditorAgent = &agent{Institution: institution{BICFI: tx.CreditorBIC}}
			}
			if tx.Remittance != "" {
				out.Remittance = &remittance{Unstructured: tx.Remittance}
			}
			p.Transactions = append(p.Transactions, out)
		}
		doc.Initiation.Payments = append(doc.Initiation.Payments, p)
	}
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	b.WriteString("\n")
	return b.Bytes(), nil
}

// Parse reads a credit transfer initiation of any pain.001.001 version,
// such as .03 or .09.
func Parse(data []byte) (Transfer, error) {
	var doc document
	if err := xml.Unmarshal(data, &doc); err != nil {
		return Transfer{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if doc.XMLName.Local != "Document" || !strings.Contains(doc.XMLName.Space, "pain.001.001") || len(doc.Initiation.Payments) == 0 {
		return Transfer{}, ErrInvalid
	}
	h := doc.Initiation.Header
	t := Transfer{MessageID: strings.TrimSpace(h.MessageID), InitiatorName: strings.TrimSpace(h.Initiator.Name)}
	if created, err := time.Parse("2006-01-02T15:04:05", strings.TrimSpace(h.Created)[:min(19, len(strings.TrimSpace(h.Created)))]); err == nil {
		t.Created = created
	}
	for i, p := range doc.Initiation.Payments {
		day := ""
		if p.ExecutionDate != nil {
			day = firstOf(strings.TrimSpace(p.ExecutionDate.Date), strings.TrimSpace(p.ExecutionDate.Text))
		}
		when, err := time.Parse(time.DateOnly, day)
		if err != nil {
			return Transfer{}, fmt.Errorf("%w: payment information %d has no requested execution date", ErrInvalid, i+1)
		}
		b := Batch{
			ID: strings.TrimSpace(p.ID), ExecutionDate: when, DebtorName: strings.TrimSpace(p.Debtor.Name), Country: strings.TrimSpace(p.Debtor.Country),
			DebtorIBAN: compact(p.DebtorAccount.ID.IBAN), DebtorBIC: strings.TrimSpace(firstOf(p.DebtorAgent.Institution.BICFI, p.DebtorAgent.Institution.BIC)),
		}
		for j, tx := range p.Transactions {
			a, err := money.ParseAmount(strings.TrimSpace(tx.Amount.Instructed.Value))
			if err != nil || a <= 0 {
				return Transfer{}, fmt.Errorf("%w: transaction %d of payment information %d has no positive amount", ErrInvalid, j+1, i+1)
			}
			out := Transaction{
				InstructionID: strings.TrimSpace(tx.PaymentID.InstructionID), EndToEndID: strings.TrimSpace(tx.PaymentID.EndToEndID), Amount: a,
				Currency: strings.TrimSpace(tx.Amount.Instructed.Currency), CreditorName: strings.TrimSpace(tx.Creditor.Name),
				CreditorIBAN: compact(tx.CreditorAccount.ID.IBAN),
			}
			if tx.CreditorAgent != nil {
				out.CreditorBIC = strings.TrimSpace(firstOf(tx.CreditorAgent.Institution.BICFI, tx.CreditorAgent.Institution.BIC))
			}
			if tx.Remittance != nil {
				out.Remittance = strings.TrimSpace(tx.Remittance.Unstructured)
			}
			if out.EndToEndID == "NOTPROVIDED" {
				out.EndToEndID = ""
			}
			if out.CreditorIBAN == "" {
				return Transfer{}, fmt.Errorf("%w: transaction %d of payment information %d has no creditor IBAN", ErrInvalid, j+1, i+1)
			}
			b.Transactions = append(b.Transactions, out)
		}
		if len(b.Transactions) == 0 {
			return Transfer{}, fmt.Errorf("%w: payment information %d has no transactions", ErrInvalid, i+1)
		}
		t.Batches = append(t.Batches, b)
	}
	return t, nil
}

func compact(s string) string { return strings.ToUpper(strings.Join(strings.Fields(s), "")) }

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

type document struct {
	XMLName    xml.Name   `xml:"Document"`
	XMLNS      string     `xml:"xmlns,attr,omitempty"`
	XSI        string     `xml:"xmlns:xsi,attr,omitempty"`
	Initiation initiation `xml:"CstmrCdtTrfInitn"`
}

type initiation struct {
	Header   header        `xml:"GrpHdr"`
	Payments []paymentInfo `xml:"PmtInf"`
}

type header struct {
	MessageID string `xml:"MsgId"`
	Created   string `xml:"CreDtTm"`
	Count     int    `xml:"NbOfTxs"`
	Sum       string `xml:"CtrlSum"`
	Initiator party  `xml:"InitgPty"`
}

type party struct {
	Name    string `xml:"Nm"`
	Country string `xml:"CtryOfRes,omitempty"`
}

type paymentInfo struct {
	ID            string           `xml:"PmtInfId"`
	Method        string           `xml:"PmtMtd"`
	BatchBooking  string           `xml:"BtchBookg,omitempty"`
	Count         int              `xml:"NbOfTxs"`
	Sum           string           `xml:"CtrlSum"`
	TypeInfo      *typeInfo        `xml:"PmtTpInf,omitempty"`
	ExecutionDate *dateChoice      `xml:"ReqdExctnDt"`
	Debtor        party            `xml:"Dbtr"`
	DebtorAccount account          `xml:"DbtrAcct"`
	DebtorAgent   agent            `xml:"DbtrAgt"`
	Transactions  []creditTransfer `xml:"CdtTrfTxInf"`
}

type typeInfo struct {
	ServiceLevel code `xml:"SvcLvl"`
}

type code struct {
	Code string `xml:"Cd"`
}

// dateChoice is a date: in pain.001.001.03 the element's text, from .09 a
// Dt element inside it.
type dateChoice struct {
	Date string `xml:"Dt,omitempty"`
	Text string `xml:",chardata"`
}

type account struct {
	ID accountID `xml:"Id"`
}

type accountID struct {
	IBAN string `xml:"IBAN"`
}

type agent struct {
	Institution institution `xml:"FinInstnId"`
}

type institution struct {
	BICFI string `xml:"BICFI,omitempty"`
	// BIC is the element pain.001.001.03 uses.
	BIC   string `xml:"BIC,omitempty"`
	Other *other `xml:"Othr,omitempty"`
}

type other struct {
	ID string `xml:"Id"`
}

type creditTransfer struct {
	PaymentID       paymentID   `xml:"PmtId"`
	Amount          amount      `xml:"Amt"`
	CreditorAgent   *agent      `xml:"CdtrAgt,omitempty"`
	Creditor        party       `xml:"Cdtr"`
	CreditorAccount account     `xml:"CdtrAcct"`
	Remittance      *remittance `xml:"RmtInf,omitempty"`
}

type paymentID struct {
	InstructionID string `xml:"InstrId,omitempty"`
	EndToEndID    string `xml:"EndToEndId"`
}

type amount struct {
	Instructed instructed `xml:"InstdAmt"`
}

type instructed struct {
	Currency string `xml:"Ccy,attr"`
	Value    string `xml:",chardata"`
}

type remittance struct {
	Unstructured string `xml:"Ustrd"`
}
