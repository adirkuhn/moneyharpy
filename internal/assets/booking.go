package assets

import (
	"time"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Accounts are the categories an asset books on besides its own.
type Accounts struct {
	// Depreciation is the depreciation costs category.
	Depreciation resource.ID
	// Accumulated is the asset category's accumulated depreciation.
	Accumulated resource.ID
	// BookResult takes the result of divestments.
	BookResult resource.ID
	// Reserve is the reinvestment reserve.
	Reserve resource.ID
}

// Needs tells which accounts the asset's entries use.
type Needs struct{ Depreciation, BookResult, Reserve bool }

// needs reports which accounts Entries will use.
func (a Asset) needs() Needs {
	var n Needs
	for _, c := range a.ValueChanges {
		if c.ExternallyBooked || c.Amount == 0 {
			continue
		}
		n.Depreciation = true
		if c.Type == Divestment {
			n.BookResult = true
		}
	}
	if a.Disposal != nil && a.Disposal.ReserveSale != nil {
		n.BookResult, n.Reserve = true, true
	}
	if a.ReservePurchase != nil {
		n.Reserve = true
	}
	return n
}

// secondRow sets a bit generated ids do not reach before 2043 (bank
// transactions use bit 62 the same way), so each booking's second row has
// an id of its own that stays the same when the asset is booked again.
const secondRow = resource.ID(1) << 61

// Entries returns what the asset books, in the journal's sign: debits
// positive. Every booking's rows have ids derived from what it books, so
// booking the same asset again gives the same entries.
//
//   - A value change, unless externally booked: its amount on the
//     accumulated depreciation, against depreciation costs (the book result
//     for a divestment).
//   - A disposal, unless externally booked: the accumulated depreciation of
//     the booked value changes cleared against the asset's category.
//   - A reinvestment reserve purchase, on the purchase date: the reserve
//     against the asset's category, lowering its cost.
//   - A reinvestment reserve sale, on the disposal date: the book result
//     against the reserve.
func (a Asset) Entries(acc Accounts) []journal.Entry {
	var out []journal.Entry
	pair := func(id resource.ID, day time.Time, debit, credit resource.ID, amount money.Amount, description string) {
		if amount == 0 {
			return
		}
		row := func(rid, ledger resource.ID, amt money.Amount, order int) journal.Entry {
			return journal.Entry{
				ID: rid, AdministrationID: a.AdministrationID, DocumentType: DocumentType, DocumentID: a.ID, Date: day,
				LedgerAccountID: ledger, Amount: amt, Description: description, RowOrder: order,
			}
		}
		out = append(out, row(id, debit, amount, len(out)), row(id|secondRow, credit, -amount, len(out)+1))
	}
	var booked money.Amount
	for _, c := range a.ValueChanges {
		if c.ExternallyBooked {
			continue
		}
		costs := acc.Depreciation
		if c.Type == Divestment {
			costs = acc.BookResult
		}
		description := a.Name
		if c.Description != nil && *c.Description != "" {
			description = a.Name + ": " + *c.Description
		}
		// A decrease credits the accumulated depreciation and debits the
		// costs; amount is the cost side.
		pair(c.ID, c.Date, costs, acc.Accumulated, -c.Amount, description)
		booked += c.Amount
	}
	if r := a.ReservePurchase; r != nil {
		pair(r.ID, a.PurchaseDate, acc.Reserve, a.LedgerAccountID, r.Amount, a.Name)
	}
	if d := a.Disposal; d != nil {
		if !d.ExternallyBooked {
			pair(d.ID, d.Date, acc.Accumulated, a.LedgerAccountID, -booked, a.Name)
		}
		if s := d.ReserveSale; s != nil {
			pair(s.ID, d.Date, acc.BookResult, acc.Reserve, s.Amount, a.Name)
		}
	}
	return out
}
