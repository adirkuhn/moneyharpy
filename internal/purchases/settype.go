package purchases

import (
	"context"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Retype is an incoming document turned into another type under the same
// id: it is removed as From and stored as the one of Document, General or
// Journal that is set. The store moves its notes, attachments and events
// along, at once.
type Retype struct {
	AdministrationID resource.ID
	ID               resource.ID
	// From is the kind the document was: PurchaseInvoice, Receipt,
	// GeneralDocument or TypelessDocument.
	From     string
	Document *Document
	General  *General
	Journal  *journal.Document
	// Entries are what a booked purchase invoice or receipt books as its
	// new kind, in place of what it booked as From.
	Entries []journal.Entry
}

// To is the kind the document becomes.
func (r Retype) To() string {
	switch {
	case r.Document != nil:
		return r.Document.Kind
	case r.General != nil:
		return GeneralDocument
	}
	return journal.GeneralJournalDocument
}

// SetType turns an incoming document that books nothing into a purchase
// invoice, receipt, general document or journal entry (documentType as the
// Incoming page's type filter names it), keeping its id, its notes,
// attachments and history, and its contact, reference and date where the
// new type has them. As in Moneybird, a booked purchase invoice becomes a
// receipt and back, booked again as its new kind, but becomes nothing else;
// a paid one or a journal entry keeps its type. The type it has already
// changes nothing, but confirms what Scan and Recognise chose.
func (s *Service) SetType(ctx context.Context, admin, id, user resource.ID, documentType string) (Incoming, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Incoming{}, err
	}
	to, ok := incomingTypes[documentType]
	if !ok {
		return Incoming{}, invalid("document_type", "is not one of purchase_invoice, receipt, general_document, general_journal_document")
	}
	cur, err := s.store.IncomingDocument(ctx, admin, id, s.today(a))
	if err != nil {
		return Incoming{}, err
	}
	if cur.Kind == to {
		s.confirmed(ctx, admin, id, FieldDocumentType)
		return s.IncomingDocument(ctx, admin, id, user)
	}
	r, err := s.retype(ctx, a, cur, to)
	if err != nil {
		return Incoming{}, err
	}
	if err := s.checkDay(ctx, admin, user, cur.Date, "date"); err != nil {
		return Incoming{}, err
	}
	if err := s.rebook(ctx, a, user, &r); err != nil {
		return Incoming{}, err
	}
	if err := s.store.RetypeDocument(ctx, r); err != nil {
		return Incoming{}, wrap("change document type", err)
	}
	s.confirmed(ctx, admin, id, FieldDocumentType)
	return s.IncomingDocument(ctx, admin, id, user)
}

// rebook works out what a booked purchase invoice or receipt books as its
// new kind: a receipt books the other way round from a revenue invoice, and
// a purchase invoice needs its supplier.
func (s *Service) rebook(ctx context.Context, a administration.Listed, user resource.ID, r *Retype) error {
	if r.Document == nil || !r.Document.Booked() {
		return nil
	}
	d := r.Document
	if d.Kind == PurchaseInvoice && d.ContactID == 0 {
		return invalid("contact_id", "can't be blank")
	}
	rates, err := s.rates(ctx, a.ID, user, *d)
	if err != nil {
		return err
	}
	d.Totals = computeTotals(*d, rates)
	allocate(d, rates)
	if r.Entries, err = s.post(ctx, a.ID, user, *d, rates); err != nil {
		return err
	}
	return nil
}

// retype works out the document cur becomes as kind to.
func (s *Service) retype(ctx context.Context, a administration.Listed, cur Incoming, to string) (Retype, error) {
	r := Retype{AdministrationID: a.ID, ID: cur.ID, From: cur.Kind}
	now := s.now()
	// src holds what the source has that the new type keeps.
	src := Document{
		ID: cur.ID, AdministrationID: a.ID, ContactID: cur.ContactID, Reference: cur.Reference, Date: cur.Date, State: New,
		Origin: OriginAPI, Currency: a.Currency, CreatedAt: cur.CreatedAt, UpdatedAt: now,
	}
	switch cur.Kind {
	case journal.GeneralJournalDocument:
		return Retype{}, invalid("base", "A journal entry cannot change type")
	case PurchaseInvoice, Receipt:
		d, err := s.store.DocumentByID(ctx, a.ID, cur.ID)
		if err != nil {
			return Retype{}, err
		}
		name := "purchase invoice"
		if d.Kind == Receipt {
			name = "receipt"
		}
		if d.State == Paid || len(d.Payments) > 0 {
			return Retype{}, invalid("base", "A paid "+name+" or one with payments cannot change type")
		}
		if d.Booked() && to != PurchaseInvoice && to != Receipt {
			return Retype{}, invalid("base", `It is only possible to change to this type if the document is in the "New" state`)
		}
		src = d
		src.UpdatedAt = now
	case GeneralDocument:
		g, err := s.store.GeneralDocument(ctx, a.ID, cur.ID)
		if err != nil {
			return Retype{}, err
		}
		src.EntryNumber, src.DueDate = g.EntryNumber, g.DueDate
	case TypelessDocument:
		t, err := s.store.TypelessDocument(ctx, a.ID, cur.ID)
		if err != nil {
			return Retype{}, err
		}
		src.Origin = cmpString(t.Origin, OriginAPI)
	}
	// Purchase invoices, receipts and general documents share their entry
	// numbers: one of them keeps its number, a typeless document gets the
	// next one.
	if src.EntryNumber == 0 && to != journal.GeneralJournalDocument {
		n, err := s.store.NextEntryNumber(ctx, a.ID)
		if err != nil {
			return Retype{}, wrap("number document", err)
		}
		src.EntryNumber = n
	}
	switch to {
	case PurchaseInvoice, Receipt:
		d := src
		d.Kind = to
		if to == Receipt {
			d.RevenueInvoice = false
		}
		if cur.Kind != PurchaseInvoice && cur.Kind != Receipt {
			d.Lines, d.FiscalAllocations = nil, nil
		}
		d.Notes, d.Attachments, d.Payments = nil, nil, nil
		r.Document = &d
	case GeneralDocument:
		r.General = &General{
			ID: cur.ID, AdministrationID: a.ID, ContactID: src.ContactID, EntryNumber: src.EntryNumber, Reference: src.Reference, Date: src.Date,
			DueDate: src.DueDate, State: Saved, CreatedAt: src.CreatedAt, UpdatedAt: now,
		}
	default:
		r.Journal = &journal.Document{ID: cur.ID, AdministrationID: a.ID, Reference: src.Reference, Date: src.Date, CreatedAt: src.CreatedAt, UpdatedAt: now}
	}
	return r, nil
}
