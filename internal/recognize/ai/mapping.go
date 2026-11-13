package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/iban"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/recognize"
)

// answer is the model's JSON, as described by extractionSchema. Every field is
// a pointer: null and missing both mean "not on the document".
type answer struct {
	Type     *string `json:"type"`
	Supplier struct {
		Name      *string `json:"name"`
		VATNumber *string `json:"vat_number"`
		KVK       *string `json:"kvk"`
		IBAN      *string `json:"iban"`
		Email     *string `json:"email"`
		Website   *string `json:"website"`
		Address1  *string `json:"address1"`
		Zipcode   *string `json:"zipcode"`
		City      *string `json:"city"`
		Country   *string `json:"country"`
		Phone     *string `json:"phone"`
	} `json:"supplier"`
	Reference     *string `json:"reference"`
	Date          *string `json:"date"`
	DueDate       *string `json:"due_date"`
	Currency      *string `json:"currency"`
	PricesInclTax *bool   `json:"prices_incl_tax"`
	TotalInclTax  *string `json:"total_incl_tax"`
	TotalExclTax  *string `json:"total_excl_tax"`
	VAT           []struct {
		Rate    *string `json:"rate"`
		Taxable *string `json:"taxable"`
		Tax     *string `json:"tax"`
	} `json:"vat"`
	Lines []struct {
		Description *string `json:"description"`
		Quantity    *string `json:"quantity"`
		Price       *string `json:"price"`
		Total       *string `json:"total"`
		TaxRate     *string `json:"tax_rate"`
	} `json:"lines"`
	PaymentReference *string `json:"payment_reference"`
}

var (
	kvkShape      = regexp.MustCompile(`^[0-9]{8}$`)
	countryShape  = regexp.MustCompile(`^[A-Z]{2}$`)
	currencyShape = regexp.MustCompile(`^[A-Z]{3}$`)
	vatShape      = regexp.MustCompile(`^[A-Z]{2}[0-9A-Z]{2,13}$`)
	currencyAffix = regexp.MustCompile(`^(?:[A-Za-z]{3}|[€$£])\s*|\s*(?:[A-Za-z]{3}|[€$£])$`)
	numberShape   = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)
)

// toExtraction maps the model's answer into an Extraction. Values are
// uncertain, except those the code can verify: a supplier with a valid IBAN
// and totals that add up (excl. + VAT = incl.). Values that are not what the
// field holds are dropped rather than passed on.
func toExtraction(raw []byte) (recognize.Extraction, error) {
	var a answer
	if err := json.Unmarshal(raw, &a); err != nil {
		return recognize.Extraction{}, fmt.Errorf("model answer is not the expected JSON: %w", err)
	}
	e := recognize.Extraction{Source: recognize.SourceAI}

	if s := str(a.Type); s == string(recognize.KindPurchaseInvoice) || s == string(recognize.KindReceipt) {
		e.Kind = uncertain(recognize.Kind(s))
	}
	e.Supplier, e.SupplierFound = supplier(a)
	if s := str(a.Reference); s != "" {
		e.Reference = uncertain(s)
	}
	if d, ok := parseDate(str(a.Date)); ok {
		e.Date = uncertain(d)
	}
	if d, ok := parseDate(str(a.DueDate)); ok {
		e.DueDate = uncertain(d)
	}
	if c := strings.ToUpper(str(a.Currency)); currencyShape.MatchString(c) {
		e.Currency = uncertain(c)
	}
	if a.PricesInclTax != nil {
		e.PricesInclTax = uncertain(*a.PricesInclTax)
	}
	if v, ok := parseAmount(str(a.TotalInclTax)); ok {
		e.TotalInclTax = uncertain(v)
	}
	if v, ok := parseAmount(str(a.TotalExclTax)); ok {
		e.TotalExclTax = uncertain(v)
	}
	for _, v := range a.VAT {
		rate, ok := parseRate(str(v.Rate))
		if !ok {
			continue
		}
		taxable, _ := parseAmount(str(v.Taxable))
		tax, _ := parseAmount(str(v.Tax))
		e.VAT = append(e.VAT, recognize.VAT{Rate: rate, Taxable: taxable, Tax: tax})
	}
	for _, l := range a.Lines {
		line := recognize.Line{Description: str(l.Description)}
		if q, ok := parseQuantity(str(l.Quantity)); ok {
			line.Quantity = uncertain(q)
		}
		if v, ok := parseAmount(str(l.Price)); ok {
			line.Price = uncertain(v)
		}
		if v, ok := parseAmount(str(l.Total)); ok {
			line.Total = uncertain(v)
		}
		if r, ok := parseRate(str(l.TaxRate)); ok {
			line.TaxRate = uncertain(r)
		}
		if line.Description == "" && !line.Quantity.Found && !line.Price.Found && !line.Total.Found {
			continue
		}
		e.Lines = append(e.Lines, line)
	}
	if s := strings.ToUpper(strings.ReplaceAll(str(a.PaymentReference), " ", "")); s != "" {
		e.PaymentReference = uncertain(s)
	}

	if totalsAddUp(e) {
		e.TotalInclTax.Confidence = recognize.Certain
		e.TotalExclTax.Confidence = recognize.Certain
	}
	return e, nil
}

func supplier(a answer) (recognize.Supplier, recognize.Confidence) {
	s := recognize.Supplier{
		Name:     str(a.Supplier.Name),
		Email:    str(a.Supplier.Email),
		Website:  str(a.Supplier.Website),
		Address1: str(a.Supplier.Address1),
		Zipcode:  str(a.Supplier.Zipcode),
		City:     str(a.Supplier.City),
		Phone:    str(a.Supplier.Phone),
	}
	if v := strings.ToUpper(stripAll(str(a.Supplier.VATNumber), " .-")); vatShape.MatchString(v) {
		s.VATNumber = v
	}
	if k := stripAll(str(a.Supplier.KVK), " .-"); kvkShape.MatchString(k) {
		s.KVK = k
	}
	if c := strings.ToUpper(str(a.Supplier.Country)); countryShape.MatchString(c) {
		s.Country = c
	}
	if i := iban.Normalize(str(a.Supplier.IBAN)); iban.Valid(i) {
		s.IBAN = i
	}
	switch {
	case s.IBAN != "":
		return s, recognize.Certain
	case s == recognize.Supplier{}:
		return s, ""
	default:
		return s, recognize.Uncertain
	}
}

// totalsAddUp checks total excl. VAT plus the VAT per rate is total incl. VAT.
func totalsAddUp(e recognize.Extraction) bool {
	if !e.TotalInclTax.Found || !e.TotalExclTax.Found || len(e.VAT) == 0 {
		return false
	}
	sum := e.TotalExclTax.V
	for _, v := range e.VAT {
		sum += v.Tax
	}
	return sum == e.TotalInclTax.V
}

func uncertain[T any](v T) recognize.Value[T] {
	return recognize.Value[T]{V: v, Found: true, Confidence: recognize.Uncertain}
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func stripAll(s, chars string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(chars, r) {
			return -1
		}
		return r
	}, s)
}

// parseDate reads YYYY-MM-DD, also when the model added a time.
func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if len(s) > 10 && s[10] == 'T' {
		s = s[:10]
	}
	d, err := time.Parse(time.DateOnly, s)
	return d, err == nil
}

// parseAmount reads an amount as models still write them despite the prompt:
// with a currency, a decimal comma or thousands separators.
func parseAmount(s string) (money.Amount, bool) {
	s = currencyAffix.ReplaceAllString(strings.TrimSpace(s), "")
	s = strings.ReplaceAll(s, " ", "")
	if strings.Trim(s, "0123456789.,-") != "" {
		return 0, false
	}
	dots, commas := strings.Count(s, "."), strings.Count(s, ",")
	var thousands, decimal string
	switch {
	case dots > 0 && commas > 0:
		thousands, decimal = ",", "."
		if strings.LastIndex(s, ",") > strings.LastIndex(s, ".") {
			thousands, decimal = ".", ","
		}
		if strings.Count(s, decimal) > 1 {
			return 0, false
		}
	case dots+commas > 1:
		thousands = "." // only one kind of separator, used more than once
		if commas > 0 {
			thousands = ","
		}
	case dots+commas == 1:
		sep := "."
		if commas > 0 {
			sep = ","
		}
		if _, frac, _ := strings.Cut(s, sep); len(frac) == 3 {
			thousands = sep
		} else {
			decimal = sep
		}
	}
	whole, frac := s, ""
	if decimal != "" {
		whole, frac, _ = strings.Cut(s, decimal)
	}
	if thousands != "" {
		groups := strings.Split(whole, thousands)
		for _, g := range groups[1:] {
			if len(g) != 3 {
				return 0, false
			}
		}
		whole = strings.Join(groups, "")
	}
	if frac != "" {
		whole += "." + frac
	}
	a, err := money.ParseAmount(whole)
	return a, err == nil
}

// parseRate reads a VAT percentage such as "21", "21%" or "5,5".
func parseRate(s string) (string, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%")), ",", ".")
	if !numberShape.MatchString(s) {
		return "", false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 || f > 100 {
		return "", false
	}
	return strconv.FormatFloat(f, 'f', -1, 64), true
}

// parseQuantity reads a quantity, normalising a decimal comma to a dot.
func parseQuantity(s string) (string, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	if !numberShape.MatchString(s) {
		return "", false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return "", false
	}
	return strconv.FormatFloat(f, 'f', -1, 64), true
}
