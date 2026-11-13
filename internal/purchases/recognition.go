package purchases

import (
	"context"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Recognition is Scan and Recognise, told about uploads, edits and removals
// so it can read new files, learn from what users save and forget removed
// documents.
type Recognition interface {
	// Queue has the document's attachments read in the background.
	Queue(ctx context.Context, admin, id, user resource.ID) error
	// Edited tells that a user changed or created a purchase invoice or
	// receipt; before is the zero Document for a new one.
	Edited(ctx context.Context, admin, user resource.ID, before, after Document)
	// Forget drops what was read of a removed document.
	Forget(ctx context.Context, admin, id resource.ID) error
}

// UseRecognition has uploads read by Scan and Recognise.
func (s *Service) UseRecognition(r Recognition) { s.recognition = r }

func (s *Service) queue(ctx context.Context, admin, id, user resource.ID) error {
	if s.recognition == nil {
		return nil
	}
	return wrap("queue recognition", s.recognition.Queue(ctx, admin, id, user))
}

func (s *Service) edited(ctx context.Context, admin, user resource.ID, before, after Document) {
	if s.recognition != nil {
		s.recognition.Edited(ctx, admin, user, before, after)
	}
}

func (s *Service) forget(ctx context.Context, admin, id resource.ID) error {
	if s.recognition == nil {
		return nil
	}
	return wrap("forget recognition", s.recognition.Forget(ctx, admin, id))
}

// Propose fills in an incoming document that books nothing with what Scan
// and Recognise read: a typeless document becomes a new purchase invoice or
// receipt of kind (PurchaseInvoice or Receipt) at once with the changes; a
// new purchase invoice or receipt keeps its kind. Either way it stays new,
// whatever it holds, until a user saves it. A booked document or one of
// another type is not changed (ErrNotEditable).
func (s *Service) Propose(ctx context.Context, admin, id, user resource.ID, kind string, c Changes) (Document, error) {
	if err := checkKind(kind); err != nil {
		return Document{}, err
	}
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Document{}, err
	}
	cur, err := s.store.IncomingDocument(ctx, admin, id, s.today(a))
	if err != nil {
		return Document{}, err
	}
	var d Document
	var r *Retype
	switch cur.Kind {
	case TypelessDocument:
		rt, err := s.retype(ctx, a, cur, kind)
		if err != nil {
			return Document{}, err
		}
		r, d = &rt, *rt.Document
	case PurchaseInvoice, Receipt:
		if d, err = s.store.Document(ctx, admin, cur.Kind, id); err != nil {
			return Document{}, err
		}
		if d.Booked() || len(d.Payments) > 0 {
			return Document{}, ErrNotEditable
		}
	default:
		return Document{}, ErrNotEditable
	}
	c.PaymentAccountID = nil
	if err := s.apply(ctx, a, user, &d, c); err != nil {
		return Document{}, err
	}
	rates, err := s.rates(ctx, admin, user, d)
	if err != nil {
		return Document{}, err
	}
	d.Totals = computeTotals(d, rates)
	allocate(&d, rates)
	d.State, d.UpdatedAt = New, s.now()
	if r != nil {
		r.Document = &d
		err = wrap("change document type", s.store.RetypeDocument(ctx, *r))
	} else {
		err = wrap("store document", s.store.SaveDocument(ctx, d, nil, false))
	}
	if err != nil {
		return Document{}, err
	}
	return s.Document(ctx, admin, d.Kind, id, user)
}
