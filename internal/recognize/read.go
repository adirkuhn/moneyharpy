package recognize

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/ocr"
)

// extract reads the files in Moneybird's order: an e-invoice, exact, wins;
// else the text of each file (its text layer, or OCR) is read by rules, and
// when that is not enough the AI model, if turned on, reads the text and
// pages too. Without anything readable it returns errUnreadable, or the
// OCR service's error when it was needed but failed.
func (s *Service) extract(ctx context.Context, files []ocr.File) (Extraction, error) {
	var best *Extraction
	var texts []string
	var pages []ocr.File
	var ocrErr error
	today := s.Now().UTC().Truncate(24 * time.Hour)
	// XML files go first: an e-invoice makes reading the others needless.
	files = slices.Clone(files)
	slices.SortStableFunc(files, func(a, b ocr.File) int {
		switch {
		case isXML(a) && !isXML(b):
			return -1
		case isXML(b) && !isXML(a):
			return 1
		}
		return 0
	})
	for _, f := range files {
		if isXML(f) {
			if e, ok := s.eInvoice(f.Data); ok {
				return e, nil
			}
			continue
		}
		if s.OCR == nil {
			ocrErr = ocr.ErrUnavailable
			continue
		}
		t, err := s.OCR.Read(ctx, f)
		if errors.Is(err, ocr.ErrUnsupported) {
			continue
		}
		if err != nil {
			ocrErr = err
			continue
		}
		for _, embedded := range t.Embedded {
			if isXML(embedded) {
				if e, ok := s.eInvoice(embedded.Data); ok {
					return e, nil
				}
			}
		}
		pages = append(pages, f)
		if strings.TrimSpace(t.Text) == "" || s.Parsers.Text == nil {
			continue
		}
		texts = append(texts, t.Text)
		e := s.Parsers.Text(t.Text, today)
		e.Source = SourceText
		if t.Method == "ocr" {
			e.Source = SourceOCR
		}
		if best == nil || score(e) > score(*best) {
			best = &e
		}
	}
	if (best == nil || !best.Good()) && len(pages) > 0 {
		if ai, ok := s.askModel(ctx, strings.Join(texts, "\f"), pages); ok {
			if best != nil {
				ai = merge(ai, *best)
			}
			best = &ai
		}
	}
	if best == nil || score(*best) == 0 {
		if ocrErr != nil {
			return Extraction{}, ocrErr
		}
		return Extraction{}, errUnreadable
	}
	return *best, nil
}

func (s *Service) eInvoice(data []byte) (Extraction, bool) {
	if s.Parsers.EInvoice == nil {
		return Extraction{}, false
	}
	e, err := s.Parsers.EInvoice(data)
	if err != nil {
		return Extraction{}, false
	}
	e.Source = SourceEInvoice
	return e, true
}

// askModel has the AI model read the text and up to maxPages pages; it
// tells false when the model is off or failed, and the rules' reading is
// used instead.
func (s *Service) askModel(ctx context.Context, text string, files []ocr.File) (Extraction, bool) {
	m, err := s.model(ctx)
	if err != nil {
		s.Logger.WarnContext(ctx, "scan and recognise: AI model unavailable", "error", err)
		return Extraction{}, false
	}
	if m == nil {
		return Extraction{}, false
	}
	var images [][]byte
	for _, f := range files {
		for page := 1; len(images) < maxPages; page++ {
			p, err := s.OCR.Render(ctx, f, page)
			if err != nil {
				break
			}
			images = append(images, p.PNG)
			if page >= p.Count {
				break
			}
		}
	}
	e, err := m.Extract(ctx, text, images)
	if err != nil {
		s.Logger.WarnContext(ctx, "scan and recognise: AI model failed", "error", err)
		return Extraction{}, false
	}
	e.Source = SourceAI
	return e, true
}

func isXML(f ocr.File) bool {
	return strings.Contains(strings.ToLower(f.ContentType), "xml") || strings.HasSuffix(strings.ToLower(f.Name), ".xml")
}

// score counts what an extraction found, to pick the best reading.
func score(e Extraction) int {
	n := 0
	for _, found := range []bool{
		e.TotalInclTax.Found, e.TotalExclTax.Found, e.Date.Found, e.DueDate.Found, e.Reference.Found, e.Currency.Found,
		e.Supplier.Name != "", e.Supplier.IBAN != "", e.Supplier.VATNumber != "", len(e.Lines) > 0, len(e.VAT) > 0,
	} {
		if found {
			n++
		}
	}
	if e.Good() {
		n += 100
	}
	return n
}

// merge completes primary with what only secondary found.
func merge(primary, secondary Extraction) Extraction {
	e := primary
	e.Kind = or(e.Kind, secondary.Kind)
	e.Reference = or(e.Reference, secondary.Reference)
	e.Date = or(e.Date, secondary.Date)
	e.DueDate = or(e.DueDate, secondary.DueDate)
	e.Currency = or(e.Currency, secondary.Currency)
	e.PricesInclTax = or(e.PricesInclTax, secondary.PricesInclTax)
	e.TotalInclTax = or(e.TotalInclTax, secondary.TotalInclTax)
	e.TotalExclTax = or(e.TotalExclTax, secondary.TotalExclTax)
	e.PaymentReference = or(e.PaymentReference, secondary.PaymentReference)
	if len(e.VAT) == 0 {
		e.VAT = secondary.VAT
	}
	if len(e.Lines) == 0 {
		e.Lines = secondary.Lines
	}
	p, q := &e.Supplier, secondary.Supplier
	p.Name = cmpOr(p.Name, q.Name)
	p.VATNumber = cmpOr(p.VATNumber, q.VATNumber)
	p.KVK = cmpOr(p.KVK, q.KVK)
	p.IBAN = cmpOr(p.IBAN, q.IBAN)
	p.Email = cmpOr(p.Email, q.Email)
	p.Website = cmpOr(p.Website, q.Website)
	p.Address1 = cmpOr(p.Address1, q.Address1)
	p.Zipcode = cmpOr(p.Zipcode, q.Zipcode)
	p.City = cmpOr(p.City, q.City)
	p.Country = cmpOr(p.Country, q.Country)
	p.Phone = cmpOr(p.Phone, q.Phone)
	if e.SupplierFound == "" {
		e.SupplierFound = secondary.SupplierFound
	}
	return e
}

func or[T any](a, b Value[T]) Value[T] {
	if a.Found {
		return a
	}
	return b
}
