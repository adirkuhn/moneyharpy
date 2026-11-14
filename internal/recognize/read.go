package recognize

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/iban"
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
		if s.Parsers.Text == nil {
			continue
		}
		// OCR reads a photo in several ways; the rules take the reading
		// they understand best.
		var fileBest *Extraction
		var readings []string
		var parsed []Extraction
		fileText := ""
		for _, text := range append([]string{t.Text}, t.Alternatives...) {
			if strings.TrimSpace(text) == "" {
				continue
			}
			e := s.Parsers.Text(text, today)
			e.Source = SourceText
			if t.Method == "ocr" {
				e.Source = SourceOCR
			}
			readings, parsed = append(readings, text), append(parsed, e)
			if fileBest == nil || score(e) > score(*fileBest) {
				fileBest, fileText = &e, text
			}
		}
		if fileBest == nil {
			continue
		}
		// The readings vote on what they all print, such as the date.
		agreed := agreeAcross(*fileBest, readings, parsed, today)
		fileBest = &agreed
		texts = append(texts, fileText)
		if best == nil || score(*fileBest) > score(*best) {
			best = fileBest
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

// aiPageSide is the longest side of a page image the AI model gets, in
// pixels: a tall till receipt photo stays legible, where a preview (1600)
// shrinks its text too much.
const aiPageSide = 2048

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
			p, err := s.OCR.RenderAt(ctx, f, page, aiPageSide)
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

// merge completes the AI model's reading with what the rules are sure of.
// A value the rules are not sure of is never used: on a poor photo it is
// often OCR garbage. A value the rules are sure of (checked against another,
// such as a total against the VAT table) wins over the model's, which is
// never sure. Lines and VAT come from the rules only when their total is
// sure and the model's total, if any, is the same.
func merge(ai, rules Extraction) Extraction {
	e := ai
	e.Kind = prefer(e.Kind, rules.Kind)
	e.Reference = prefer(e.Reference, rules.Reference)
	e.Date = prefer(e.Date, rules.Date)
	e.DueDate = prefer(e.DueDate, rules.DueDate)
	e.Currency = prefer(e.Currency, rules.Currency)
	e.PricesInclTax = prefer(e.PricesInclTax, rules.PricesInclTax)
	e.TotalInclTax = prefer(e.TotalInclTax, rules.TotalInclTax)
	e.TotalExclTax = prefer(e.TotalExclTax, rules.TotalExclTax)
	e.PaymentReference = prefer(e.PaymentReference, rules.PaymentReference)
	total := rules.TotalInclTax
	sameTotal := total.Found && total.Confidence == Certain && (!ai.TotalInclTax.Found || ai.TotalInclTax.V == total.V)
	if len(e.VAT) == 0 && sameTotal {
		e.VAT = rules.VAT
	}
	if len(e.Lines) == 0 && sameTotal {
		e.Lines = rules.Lines
	}
	if rules.SupplierFound != Certain {
		return e
	}
	p, q := &e.Supplier, rules.Supplier
	if !garbledName(q.Name) {
		p.Name = cmpOr(p.Name, q.Name)
	}
	if ValidVAT(q.VATNumber) {
		p.VATNumber = cmpOr(p.VATNumber, q.VATNumber)
	}
	if iban.Valid(iban.Normalize(q.IBAN)) {
		p.IBAN = cmpOr(p.IBAN, q.IBAN)
	}
	p.KVK = cmpOr(p.KVK, q.KVK)
	p.Email = cmpOr(p.Email, q.Email)
	p.Website = cmpOr(p.Website, q.Website)
	p.Address1 = cmpOr(p.Address1, q.Address1)
	p.Zipcode = cmpOr(p.Zipcode, q.Zipcode)
	p.City = cmpOr(p.City, q.City)
	p.Country = cmpOr(p.Country, q.Country)
	p.Phone = cmpOr(p.Phone, q.Phone)
	if e.SupplierFound == "" && e.Supplier != (Supplier{}) {
		e.SupplierFound = rules.SupplierFound
	}
	return e
}

// prefer takes the model's value unless only the rules' value is sure; a
// rules' value that is not sure is never taken.
func prefer[T any](model, rules Value[T]) Value[T] {
	switch {
	case rules.Found && rules.Confidence == Certain && (!model.Found || model.Confidence != Certain):
		return rules
	case model.Found:
		return model
	}
	return Value[T]{}
}
