package recognize

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/iban"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Self is the administration's own company: the buyer of the documents it
// reads, which is never their supplier. Empty fields are unknown.
type Self struct {
	Name      string
	Address1  string
	Zipcode   string
	VATNumber string
	KVK       string
	Email     string
	Website   string
	// IBANs are the accounts the company pays from.
	IBANs []string
}

// Own gives the details of an administration's own company.
type Own interface {
	Self(ctx context.Context, admin, user resource.ID) (Self, error)
}

// OwnCompany reads an administration's own company from its default
// identity, its website and its financial accounts.
type OwnCompany struct {
	Administrations interface {
		Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
		DefaultIdentity(ctx context.Context, admin, user resource.ID) (administration.Identity, error)
	}
	// Accounts may be nil; a user who may not see them adds no IBANs.
	Accounts interface {
		Accounts(ctx context.Context, admin, user resource.ID) ([]banking.Account, error)
	}
}

// Self returns the administration's own company details.
func (o OwnCompany) Self(ctx context.Context, admin, user resource.ID) (Self, error) {
	a, err := o.Administrations.Get(ctx, admin, user)
	if err != nil {
		return Self{}, err
	}
	id, err := o.Administrations.DefaultIdentity(ctx, admin, user)
	if err != nil {
		return Self{}, err
	}
	me := Self{
		Name: cmpOr(id.CompanyName, a.Name), Address1: id.Address1, Zipcode: id.Zipcode, VATNumber: id.TaxNumber,
		KVK: id.ChamberOfCommerce, Email: id.Email, Website: a.CompanyWebsiteURL,
	}
	if id.BankAccountNumber != "" {
		me.IBANs = append(me.IBANs, id.BankAccountNumber)
	}
	if o.Accounts != nil {
		if accounts, err := o.Accounts.Accounts(ctx, admin, user); err == nil {
			for _, acc := range accounts {
				if n := iban.Normalize(acc.Identifier); iban.Valid(n) {
					me.IBANs = append(me.IBANs, n)
				}
			}
		}
	}
	return me, nil
}

// self is the administration's own company, or nothing when it cannot be
// read: the document is then read as if for anyone.
func (s *Service) self(ctx context.Context, admin, user resource.ID) Self {
	if s.Own == nil {
		return Self{}
	}
	me, err := s.Own.Self(ctx, admin, user)
	if err != nil {
		s.Logger.WarnContext(ctx, "scan and recognise: own company details unavailable", "error", err)
		return Self{}
	}
	return me
}

// sameName tells whether name is the company's, also as OCR may misread
// it: "Exarnple Shop" is "Example Shop B.V.", but "ExampleShip" is not
// "Example Shop".
func (s Self) sameName(name string) bool {
	a, b := NormalizeName(s.Name), NormalizeName(name)
	if a == "" || b == "" {
		return false
	}
	if strings.ReplaceAll(a, " ", "") == strings.ReplaceAll(b, " ", "") {
		return true
	}
	return len(strings.Fields(a)) == len(strings.Fields(b)) && similarNames(a, b)
}

func (s Self) sameVAT(vat string) bool {
	n := NormalizeVAT(vat)
	return n != "" && n == NormalizeVAT(s.VATNumber)
}

func (s Self) sameKVK(kvk string) bool {
	n := NormalizeKVK(kvk)
	return n != "" && n == NormalizeKVK(s.KVK)
}

func (s Self) sameIBAN(account string) bool {
	n := iban.Normalize(account)
	if n == "" {
		return false
	}
	for _, own := range s.IBANs {
		if iban.Normalize(own) == n {
			return true
		}
	}
	return false
}

func (s Self) sameZipcode(zip string) bool {
	n := compactZip(zip)
	return n != "" && n == compactZip(s.Zipcode)
}

func (s Self) sameAddress(address string) bool {
	n := nameKey(address)
	return n != "" && n == nameKey(s.Address1)
}

// sameDomain tells whether an email address or website is at the
// company's own domain.
func (s Self) sameDomain(emailOrWebsite string) bool {
	d := Domain(emailOrWebsite)
	return d != "" && (d == Domain(s.Website) || d == Domain(s.Email))
}

func compactZip(s string) string {
	return strings.ToUpper(strings.Join(strings.Fields(s), ""))
}

// Is tells whether a supplier is the company itself: by its name, VAT or
// KVK number or bank account.
func (s Self) Is(sup Supplier) bool {
	return s.sameName(sup.Name) || s.sameVAT(sup.VATNumber) || s.sameKVK(sup.KVK) || s.sameIBAN(sup.IBAN)
}

// isContact tells whether a contact is the company itself.
func (s Self) isContact(c contacts.Contact) bool {
	return s.sameName(c.Name()) || s.sameName(c.CompanyName) || s.sameVAT(c.TaxNumber) || s.sameKVK(c.ChamberOfCommerce) ||
		s.sameIBAN(c.SepaIBAN) || s.sameIBAN(c.BankAccount)
}

// Without leaves out of a supplier what is the company's own.
func (s Self) Without(sup Supplier) Supplier {
	if s.sameName(sup.Name) {
		sup.Name = ""
	}
	if s.sameVAT(sup.VATNumber) {
		sup.VATNumber = ""
	}
	if s.sameKVK(sup.KVK) {
		sup.KVK = ""
	}
	if s.sameIBAN(sup.IBAN) {
		sup.IBAN = ""
	}
	if s.sameZipcode(sup.Zipcode) || s.sameAddress(sup.Address1) {
		sup.Address1, sup.Zipcode, sup.City = "", "", ""
	}
	if s.sameDomain(sup.Email) {
		sup.Email = ""
	}
	if s.sameDomain(sup.Website) {
		sup.Website = ""
	}
	return sup
}

// ownBlockLines is how many lines under the company's name its block of
// details (attention line, street, postcode) may run.
const ownBlockLines = 4

// scrub blanks what the lines print of the company itself: its name, its
// address and website, and the block under its name in the same column,
// so that only the supplier's details are left to read. Other columns of
// the lines stay where they are.
func (s Self) scrub(lines []string) []string {
	out := make([]string, len(lines))
	copy(out, lines)
	for i := range out {
		// Blanking keeps the columns, not the byte offsets: find the
		// segments again after each.
		for more := true; more; {
			more = s.scrubOne(out, i)
		}
	}
	return out
}

// scrubOne blanks the first segment of line i that is the company's own,
// and tells whether there was one.
func (s Self) scrubOne(lines []string, i int) bool {
	for _, sp := range segmentSpans(lines[i]) {
		seg := lines[i][sp[0]:sp[1]]
		switch {
		case s.sameName(cleanName(seg)):
			col := utf8.RuneCountInString(lines[i][:sp[0]])
			lines[i] = blank(lines[i], sp)
			s.scrubBlock(lines, i, col)
			return true
		case s.ownDetail(seg):
			lines[i] = blank(lines[i], sp)
			return true
		}
	}
	return false
}

// ownDetail tells whether a segment is the company's address, postcode or
// website.
func (s Self) ownDetail(seg string) bool {
	if s.sameAddress(seg) {
		return true
	}
	if zip, _, _ := postcode(seg, ""); zip != "" && s.sameZipcode(zip) {
		return true
	}
	if m := nlZip.FindStringSubmatch(seg); m != nil && s.sameZipcode(m[1]+m[2]) {
		return true
	}
	if strings.ContainsAny(seg, " ") {
		return false
	}
	return (strings.Contains(seg, ".") || strings.Contains(seg, "@")) && s.sameDomain(seg)
}

// scrubBlock blanks the segments starting in column col of the lines under
// line i, up to a blank line.
func (s Self) scrubBlock(lines []string, i, col int) {
	for j := i + 1; j < len(lines) && j <= i+ownBlockLines; j++ {
		if strings.TrimSpace(lines[j]) == "" {
			return
		}
		for _, sp := range segmentSpans(lines[j]) {
			if c := utf8.RuneCountInString(lines[j][:sp[0]]); absInt(c-col) <= 2 {
				lines[j] = blank(lines[j], sp)
				break
			}
		}
	}
}

// segmentSpans are the byte offsets of the parts of a line printed with
// "|", "·" or wide gaps, as segments splits them.
func segmentSpans(l string) [][2]int {
	var out [][2]int
	start := 0
	add := func(from, to int) {
		for from < to && l[from] == ' ' {
			from++
		}
		for to > from && l[to-1] == ' ' {
			to--
		}
		if from < to {
			out = append(out, [2]int{from, to})
		}
	}
	for _, sep := range segmentSep.FindAllStringIndex(l, -1) {
		add(start, sep[0])
		start = sep[1]
	}
	add(start, len(l))
	return out
}

// blank replaces a span of a line with as many spaces as it has
// characters, keeping the columns after it in place.
func blank(l string, sp [2]int) string {
	return l[:sp[0]] + strings.Repeat(" ", utf8.RuneCountInString(l[sp[0]:sp[1]])) + l[sp[1]:]
}
