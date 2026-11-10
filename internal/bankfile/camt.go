package bankfile

import (
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// The elements below match by local name, so every CAMT version and
// namespace reads alike.
type camtDocument struct {
	Statements    []camtStatement `xml:"BkToCstmrStmt>Stmt"`
	Reports       []camtStatement `xml:"BkToCstmrAcctRpt>Rpt"`
	Notifications []camtStatement `xml:"BkToCstmrDbtCdtNtfctn>Ntfctn"`
}

type camtStatement struct {
	ID       string        `xml:"Id"`
	IBAN     string        `xml:"Acct>Id>IBAN"`
	Other    string        `xml:"Acct>Id>Othr>Id"`
	Currency string        `xml:"Acct>Ccy"`
	Balances []camtBalance `xml:"Bal"`
	Entries  []camtEntry   `xml:"Ntry"`
}

type camtBalance struct {
	Code   string     `xml:"Tp>CdOrPrtry>Cd"`
	Amount camtAmount `xml:"Amt"`
	Sign   string     `xml:"CdtDbtInd"`
	Date   camtDate   `xml:"Dt"`
}

type camtAmount struct {
	Currency string `xml:"Ccy,attr"`
	Value    string `xml:",chardata"`
}

type camtDate struct {
	Date     string `xml:"Dt"`
	DateTime string `xml:"DtTm"`
}

type camtEntry struct {
	Amount    camtAmount   `xml:"Amt"`
	Sign      string       `xml:"CdtDbtInd"`
	Status    camtStatus   `xml:"Sts"`
	Booking   camtDate     `xml:"BookgDt"`
	Value     camtDate     `xml:"ValDt"`
	Reference string       `xml:"AcctSvcrRef"`
	Code      string       `xml:"BkTxCd>Prtry>Cd"`
	Domain    string       `xml:"BkTxCd>Domn>Fmly>SubFmlyCd"`
	Info      string       `xml:"AddtlNtryInf"`
	Batch     string       `xml:"NtryDtls>Btch>PmtInfId"`
	Details   []camtDetail `xml:"NtryDtls>TxDtls"`
}

// camtStatus is BOOK as text (up to version 7) or as a code (version 8+).
type camtStatus struct {
	Text string `xml:",chardata"`
	Code string `xml:"Cd"`
}

type camtDetail struct {
	Reference  string   `xml:"Refs>AcctSvcrRef"`
	EndToEnd   string   `xml:"Refs>EndToEndId"`
	Debtor     camtPty  `xml:"RltdPties>Dbtr"`
	DebtorIBAN string   `xml:"RltdPties>DbtrAcct>Id>IBAN"`
	Creditor   camtPty  `xml:"RltdPties>Cdtr"`
	CredIBAN   string   `xml:"RltdPties>CdtrAcct>Id>IBAN"`
	Lines      []string `xml:"RmtInf>Ustrd"`
	Structured string   `xml:"RmtInf>Strd>CdtrRefInf>Ref"`
	Info       string   `xml:"AddtlTxInf"`
}

// camtPty holds a party's name, directly (up to version 7) or under Pty.
type camtPty struct {
	Name    string `xml:"Nm"`
	PtyName string `xml:"Pty>Nm"`
}

func (p camtPty) name() string { return strings.TrimSpace(p.Name + p.PtyName) }

func (d camtDate) parse() (time.Time, bool) {
	if d.Date != "" {
		t, err := time.Parse(time.DateOnly, strings.TrimSpace(d.Date))
		return t, err == nil
	}
	if len(d.DateTime) >= 10 {
		t, err := time.Parse(time.DateOnly, d.DateTime[:10])
		return t, err == nil
	}
	return time.Time{}, false
}

func cents(s string) (int64, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f < 0 || f > 1e12 {
		return 0, fmt.Errorf("%w: amount %q", ErrInvalid, s)
	}
	return int64(math.Round(f * 100)), nil
}

func parseCAMT(data []byte) ([]Statement, error) {
	var doc camtDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	all := append(append(doc.Statements, doc.Reports...), doc.Notifications...)
	if len(all) == 0 {
		return nil, ErrUnknownFormat
	}
	out := make([]Statement, 0, len(all))
	for _, cs := range all {
		s := Statement{Account: strings.ReplaceAll(cs.IBAN+cs.Other, " ", ""), Currency: cs.Currency, Reference: strings.TrimSpace(cs.ID)}
		for _, b := range cs.Balances {
			if b.Code != "CLBD" {
				continue
			}
			amount, err := cents(b.Amount.Value)
			if err != nil {
				return nil, err
			}
			if b.Sign == "DBIT" {
				amount = -amount
			}
			s.Balance = &amount
			if day, ok := b.Date.parse(); ok {
				s.Date = &day
			}
			if s.Currency == "" {
				s.Currency = b.Amount.Currency
			}
		}
		for _, ce := range cs.Entries {
			if status := strings.TrimSpace(ce.Status.Text + ce.Status.Code); status != "" && status != "BOOK" {
				continue // Pending or informational entries are not booked yet.
			}
			e, err := camtEntryOf(ce)
			if err != nil {
				return nil, err
			}
			if s.Currency == "" {
				s.Currency = e.Currency
			}
			s.Entries = append(s.Entries, e)
		}
		out = append(out, s)
	}
	return out, nil
}

func camtEntryOf(ce camtEntry) (Entry, error) {
	amount, err := cents(ce.Amount.Value)
	if err != nil {
		return Entry{}, err
	}
	credit := ce.Sign != "DBIT"
	if !credit {
		amount = -amount
	}
	day, ok := ce.Booking.parse()
	if !ok {
		if day, ok = ce.Value.parse(); !ok {
			return Entry{}, fmt.Errorf("%w: entry without a date", ErrInvalid)
		}
	}
	e := Entry{Date: day, Amount: amount, Currency: ce.Amount.Currency, Code: strings.TrimSpace(ce.Code + ce.Domain), TransactionID: strings.TrimSpace(ce.Reference), BatchReference: strings.TrimSpace(ce.Batch)}
	if v, ok := ce.Value.parse(); ok {
		e.ValueDate = &v
	}
	var message []string
	if len(ce.Details) == 1 {
		d := ce.Details[0]
		if credit {
			e.ContraName, e.ContraAccount = d.Debtor.name(), d.DebtorIBAN
		} else {
			e.ContraName, e.ContraAccount = d.Creditor.name(), d.CredIBAN
		}
		message = append(message, d.Lines...)
		if d.Structured != "" {
			message = append(message, d.Structured)
		}
		if len(message) == 0 && d.Info != "" {
			message = append(message, d.Info)
		}
		if e.TransactionID == "" {
			e.TransactionID = strings.TrimSpace(d.Reference)
		}
		if e.TransactionID == "" && d.EndToEnd != "NOTPROVIDED" {
			e.TransactionID = strings.TrimSpace(d.EndToEnd)
		}
	} else if len(ce.Details) > 1 && e.BatchReference == "" {
		e.BatchReference = fmt.Sprintf("%d transactions", len(ce.Details))
	}
	if len(message) == 0 && ce.Info != "" {
		message = append(message, ce.Info)
	}
	e.Message = strings.Join(strings.Fields(strings.Join(message, " ")), " ")
	e.ContraAccount = strings.ReplaceAll(e.ContraAccount, " ", "")
	return e, nil
}
