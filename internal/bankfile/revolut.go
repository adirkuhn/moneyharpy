package bankfile

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"slices"
	"strings"
	"time"
)

// revolutHeader starts the CSV statement of Revolut Business, the only
// statement file Revolut gives.
var revolutHeader = []byte("Date started (UTC),Date completed (UTC),")

func isRevolut(data []byte) bool {
	first, _, _ := bytes.Cut(data, []byte("\n"))
	return bytes.HasPrefix(first, revolutHeader) && bytes.Contains(first, []byte("Total amount"))
}

// parseRevolut reads Revolut Business's CSV: one row per transaction, newest
// first, of every account the export covers. Only completed transactions
// are booked; Total amount is what a transaction, with its fee, did to the
// balance, and Balance the balance after it.
func parseRevolut(data []byte) ([]Statement, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	col := map[string]int{}
	for i, name := range rows[0] {
		col[strings.TrimSpace(name)] = i
	}
	for _, need := range []string{"Date completed (UTC)", "State", "Total amount", "Balance"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("%w: column %q is missing", ErrInvalid, need)
		}
	}
	var out []Statement
	index := map[string]int{}
	// Newest first: the first completed row of an account holds its closing balance.
	for _, row := range rows[1:] {
		get := func(name string) string {
			if i, ok := col[name]; ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		if get("State") != "COMPLETED" {
			continue // Pending, declined, reverted and failed transactions did not happen.
		}
		account := accountNumber(get("International account number"), false)
		i, ok := index[account]
		if !ok {
			out = append(out, Statement{Account: account, Currency: get("Payment currency")})
			i = len(out) - 1
			index[account] = i
		}
		s := &out[i]
		e, balance, err := revolutEntry(get)
		if err != nil {
			return nil, err
		}
		if s.Balance == nil || e.Date.After(*s.Date) {
			day := e.Date
			s.Balance, s.Date = &balance, &day
		}
		s.Entries = append(s.Entries, e)
	}
	for i := range out {
		// Oldest first, as the other formats; the same day keeps its order.
		slices.Reverse(out[i].Entries)
		slices.SortStableFunc(out[i].Entries, func(a, b Entry) int { return a.Date.Compare(b.Date) })
	}
	return out, nil
}

func revolutDate(s string) (time.Time, error) {
	if len(s) >= 10 {
		if t, err := time.Parse(time.DateOnly, s[:10]); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: date %q", ErrInvalid, s)
}

// signedCents reads an amount with its sign.
func signedCents(s string) (int64, error) {
	negative := strings.HasPrefix(s, "-")
	n, err := cents(strings.TrimPrefix(s, "-"))
	if negative {
		n = -n
	}
	return n, err
}

func revolutEntry(get func(string) string) (Entry, int64, error) {
	day, err := revolutDate(get("Date completed (UTC)"))
	if err != nil {
		return Entry{}, 0, err
	}
	amount, err := signedCents(get("Total amount"))
	if err != nil {
		return Entry{}, 0, err
	}
	balance, err := signedCents(get("Balance"))
	if err != nil {
		return Entry{}, 0, err
	}
	kind, description := get("Type"), get("Description")
	e := Entry{Date: day, Amount: amount, Currency: get("Payment currency"), Code: kind, TransactionID: get("ID"), Message: get("Reference")}
	if started, err := revolutDate(get("Date started (UTC)")); err == nil {
		e.ValueDate = &started
	}
	switch {
	case kind == "FEE":
		e.ContraName = "Revolut"
	case amount < 0:
		e.ContraName, e.ContraAccount = get("Beneficiary name"), get("Beneficiary IBAN")
	default:
		e.ContraName, e.ContraAccount = get("Sender name"), get("Sender account")
	}
	if e.ContraName == "" && kind == "CARD_PAYMENT" {
		e.ContraName = description // A card payment's description is the merchant.
	}
	if e.Message == "" {
		e.Message = description
	}
	// A payment in another currency says what it was in that currency.
	if orig := get("Orig currency"); orig != "" && orig != e.Currency && get("Orig amount") != "" {
		e.Message += " " + orig + " " + get("Orig amount")
	}
	e.Message = strings.Join(strings.Fields(e.Message), " ")
	e.ContraAccount = strings.ReplaceAll(e.ContraAccount, " ", "")
	return e, balance, nil
}
