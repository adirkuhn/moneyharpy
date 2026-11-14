package recognize

import (
	"context"
	"errors"
	"fmt"

	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// Remember fills in a new purchase invoice or receipt whose contact a user
// sets or changes with what is remembered of that contact's earlier
// documents: the categories and VAT rates of its lines, its currency and
// whether its prices include VAT. before is the document as it was, the
// zero Document for one being created; d holds the user's changes and is
// not saved yet.
//
// A field is filled in when it is empty (a line without a category), or
// when it still holds what Scan and Recognise filled in other than from the
// attachments, such as what it remembered of the contact read wrongly.
// What the user set, now or before, stays. The changes are for d; the
// fields they fill in are to mark, through Edited, once d is saved.
func (s *Service) Remember(ctx context.Context, admin, user resource.ID, before, d purchases.Document) (purchases.Changes, []purchases.Remembered) {
	c, filled, err := s.remember(ctx, admin, user, before, d)
	if err != nil {
		s.Logger.ErrorContext(ctx, "scan and recognise: fill in from the contact's earlier documents", "document", d.ID, "error", err)
		return purchases.Changes{}, nil
	}
	return c, filled
}

func (s *Service) remember(ctx context.Context, admin, user resource.ID, before, d purchases.Document) (purchases.Changes, []purchases.Remembered, error) {
	var c purchases.Changes
	if d.Booked() || d.ContactID == 0 || d.ContactID == before.ContactID {
		return c, nil, nil
	}
	marks := map[string]Field{}
	r, err := s.Store.Recognition(ctx, admin, d.ID)
	switch {
	case err == nil:
		for _, f := range r.Fields {
			marks[f.Name] = f
		}
	case !errors.Is(err, ErrNotFound):
		return c, nil, err
	}
	changed := map[string]bool{}
	if before.ID != 0 {
		for _, name := range ChangedFields(before, d) {
			changed[name] = true
		}
	}
	m, err := s.memory(ctx, admin, user, d.ContactID, 0)
	if err != nil || m.Empty() {
		return c, nil, err
	}
	uncategorized, err := s.Ledger.SystemLedgerAccount(ctx, admin, user, uncategorizedExpenses)
	if err != nil {
		return c, nil, err
	}

	replaceable := func(name string, empty bool) bool {
		if empty {
			return true
		}
		f, ok := marks[name]
		return ok && !changed[name] && ReasonOf(f) != ReasonAttachment
	}
	var filled []purchases.Remembered
	fill := func(name string, conf Confidence, source resource.ID) {
		filled = append(filled, purchases.Remembered{Field: name, Certain: conf == Certain, SourceDocumentID: source})
	}

	memory := m.Recall(KindOf(d.Kind))
	if cur := memory.Currency(); cur.Known() && replaceable(FieldCurrency, false) {
		v := cur.V
		c.Currency = &v
		fill(FieldCurrency, cur.Confidence(), cur.SourceDocumentID)
	}
	if incl := memory.PricesInclTax(); incl.Known() && replaceable(FieldPricesAreInclTax, false) {
		v := incl.V
		c.PricesAreInclTax = &v
		fill(FieldPricesAreInclTax, incl.Confidence(), incl.SourceDocumentID)
	}
	for i, l := range d.Lines {
		category, ok := memory.Category(l.Description)
		if !ok {
			continue
		}
		id := l.ID
		in := sales.DetailInput{ID: &id}
		field := func(name string) string { return fmt.Sprintf("details.%d.%s", i, name) }
		if v := category.LedgerAccountID; v != 0 && replaceable(field("ledger_account_id"), l.LedgerAccountID == nil || *l.LedgerAccountID == uncategorized) {
			in.LedgerAccountID = &v
			fill(field("ledger_account_id"), category.Confidence(), category.SourceDocumentID)
		}
		if v := category.TaxRateID; v != 0 && replaceable(field("tax_rate_id"), l.TaxRateID == 0) {
			in.TaxRateID = &v
			fill(field("tax_rate_id"), category.Confidence(), category.SourceDocumentID)
		}
		if in.LedgerAccountID != nil || in.TaxRateID != nil {
			c.Details = append(c.Details, in)
		}
	}
	return c, filled, nil
}
