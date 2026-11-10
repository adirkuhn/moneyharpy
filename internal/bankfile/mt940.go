package bankfile

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// statementLine is a :61: field: value date, optional entry date, debit or
// credit (R reverses), optional funds code, amount, transaction type and
// references.
var statementLine = regexp.MustCompile(`^(\d{6})(\d{4})?(RC|RD|C|D)[A-Z]?([\d,]+)([A-Z][A-Z0-9]{3})?([^/\n]*)(?://([^\n]*))?`)

// balanceLine is a :60F:/:62F: field: sign, date, currency and amount.
var balanceLine = regexp.MustCompile(`^([CD])(\d{6})([A-Z]{3})([\d,]+)`)

type mtField struct{ tag, value string }

// mtFields splits a message into its :tag: fields, keeping continuation
// lines with their field.
func mtFields(data string) []mtField {
	var out []mtField
	for _, line := range strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, ":") {
			if tag, value, ok := strings.Cut(line[1:], ":"); ok && len(tag) <= 4 {
				out = append(out, mtField{tag, value})
				continue
			}
		}
		if line == "-" || strings.HasPrefix(line, "-}") || strings.HasPrefix(line, "{") {
			continue
		}
		if len(out) > 0 {
			out[len(out)-1].value += "\n" + line
		}
	}
	return out
}

func mtDate(s string) (time.Time, error) {
	t, err := time.Parse("060102", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: date %q", ErrInvalid, s)
	}
	return t, nil
}

func parseMT940(data []byte) ([]Statement, error) {
	var out []Statement
	var cur *Statement
	var last *Entry
	for _, f := range mtFields(string(data)) {
		value := strings.TrimSpace(f.value)
		switch f.tag {
		case "20":
			out = append(out, Statement{Reference: value})
			cur, last = &out[len(out)-1], nil
		case "25":
			if cur != nil {
				account := value
				if i := strings.LastIndex(account, "/"); i >= 0 {
					account = account[i+1:]
				}
				cur.Account = strings.Fields(account + " ")[0]
			}
		case "28C":
			if cur != nil && cur.Reference == "" {
				cur.Reference = value
			}
		case "60F", "60M":
			if m := balanceLine.FindStringSubmatch(value); m != nil && cur != nil {
				cur.Currency = m[3]
			}
		case "62F":
			m := balanceLine.FindStringSubmatch(value)
			if m == nil || cur == nil {
				continue
			}
			day, err := mtDate(m[2])
			if err != nil {
				return nil, err
			}
			amount, err := cents(strings.ReplaceAll(m[4], ",", "."))
			if err != nil {
				return nil, err
			}
			if m[1] == "D" {
				amount = -amount
			}
			cur.Date, cur.Balance, cur.Currency = &day, &amount, m[3]
		case "61":
			if cur == nil {
				return nil, fmt.Errorf("%w: transaction before the statement reference field", ErrInvalid)
			}
			e, err := mtEntry(value)
			if err != nil {
				return nil, err
			}
			e.Currency = cur.Currency
			cur.Entries = append(cur.Entries, e)
			last = &cur.Entries[len(cur.Entries)-1]
		case "86":
			if last != nil {
				mtInfo(last, value)
			}
		}
	}
	if len(out) == 0 {
		return nil, ErrUnknownFormat
	}
	return out, nil
}

func mtEntry(value string) (Entry, error) {
	m := statementLine.FindStringSubmatch(value)
	if m == nil {
		return Entry{}, fmt.Errorf("%w: transaction %q", ErrInvalid, value)
	}
	valueDate, err := mtDate(m[1])
	if err != nil {
		return Entry{}, err
	}
	day := valueDate
	if m[2] != "" {
		// The entry date has no year; take the one closest to the value date.
		day = time.Date(valueDate.Year(), time.Month(atoi(m[2][:2])), atoi(m[2][2:]), 0, 0, 0, 0, time.UTC)
		if day.Sub(valueDate) > 180*24*time.Hour {
			day = day.AddDate(-1, 0, 0)
		} else if valueDate.Sub(day) > 180*24*time.Hour {
			day = day.AddDate(1, 0, 0)
		}
	}
	amount, err := cents(strings.ReplaceAll(m[4], ",", "."))
	if err != nil {
		return Entry{}, err
	}
	if m[3] == "D" || m[3] == "RC" {
		amount = -amount
	}
	e := Entry{Date: day, ValueDate: &valueDate, Amount: amount, Code: m[5]}
	if ref := strings.TrimSpace(m[7]); ref != "" {
		e.TransactionID = ref
	} else if ref := strings.TrimSpace(m[6]); ref != "" && ref != "NONREF" {
		e.TransactionID = ref
	}
	return e, nil
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

// mtInfo reads the :86: field: structured as /CODE/value/ pairs in Dutch
// and many other banks' files, free text otherwise.
func mtInfo(e *Entry, value string) {
	text := strings.ReplaceAll(value, "\n", "")
	if !strings.HasPrefix(text, "/") {
		e.Message = strings.Join(strings.Fields(value), " ")
		return
	}
	known := map[string]bool{"TRTP": true, "IBAN": true, "BIC": true, "NAME": true, "REMI": true, "EREF": true, "MARF": true, "CSID": true, "CNTP": true, "PREF": true, "ORDP": true, "BENM": true, "ID": true, "ADDR": true, "USTD": true, "STRD": true, "CUR": true, "ISDT": true, "RTRN": true, "ULTC": true, "ULTD": true, "PURP": true}
	parts := strings.Split(text[1:], "/")
	fields := map[string]string{}
	key := ""
	for _, p := range parts {
		if known[p] && (key == "" || fields[key] != "" || key == "USTD" || key == "REMI" || key == "CNTP") {
			key = p
			if _, seen := fields[key]; !seen {
				fields[key] = ""
			}
			continue
		}
		if key == "" {
			continue
		}
		if fields[key] != "" {
			fields[key] += "/"
		}
		fields[key] += p
	}
	if cntp, ok := fields["CNTP"]; ok {
		// /CNTP/IBAN/BIC/NAME/CITY/
		c := strings.Split(cntp, "/")
		if len(c) > 0 {
			e.ContraAccount = c[0]
		}
		if len(c) > 2 {
			e.ContraName = c[2]
		}
	}
	if v := fields["IBAN"]; v != "" {
		e.ContraAccount = v
	}
	if v := fields["NAME"]; v != "" {
		e.ContraName = v
	}
	remi := fields["REMI"]
	if v := fields["USTD"]; v != "" {
		remi = v
	}
	remi = strings.TrimPrefix(strings.TrimPrefix(remi, "USTD//"), "STRD/CUR/")
	e.Message = strings.Join(strings.Fields(strings.Trim(remi, "/")), " ")
	if e.Message == "" {
		e.Message = strings.Join(strings.Fields(value), " ")
	}
	if e.TransactionID == "" {
		if v := fields["EREF"]; v != "" && v != "NOTPROVIDED" {
			e.TransactionID = v
		}
	}
	e.ContraName = strings.TrimSpace(e.ContraName)
	e.ContraAccount = strings.TrimSpace(e.ContraAccount)
}
