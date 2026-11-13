package recognize

import (
	"net/url"
	"strings"
	"unicode"

	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/iban"
)

// MatchedBy tells which key matched a supplier to a contact.
type MatchedBy string

const (
	ByIBAN        MatchedBy = "iban"
	ByVATNumber   MatchedBy = "vat_number"
	ByKVK         MatchedBy = "kvk"
	ByEmailDomain MatchedBy = "email_domain"
	ByName        MatchedBy = "name"
)

// Confidence of a match: a bank account, VAT or KVK number identifies a
// supplier; an email domain or a name only suggests it.
func (b MatchedBy) Confidence() Confidence {
	switch b {
	case ByIBAN, ByVATNumber, ByKVK:
		return Certain
	}
	return Uncertain
}

// freeMail are email domains anyone can have an address at, which say
// nothing about the sender.
var freeMail = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "outlook.com": true, "hotmail.com": true, "hotmail.nl": true, "live.com": true,
	"live.nl": true, "msn.com": true, "yahoo.com": true, "yahoo.nl": true, "icloud.com": true, "me.com": true, "mac.com": true,
	"aol.com": true, "proton.me": true, "protonmail.com": true, "gmx.com": true, "gmx.net": true, "gmx.de": true, "ziggo.nl": true,
	"kpnmail.nl": true, "planet.nl": true, "home.nl": true, "xs4all.nl": true, "telfort.nl": true, "hetnet.nl": true,
	"zonnet.nl": true, "upcmail.nl": true, "web.de": true, "mail.com": true,
}

// legalForms are left out of company names, so "Example Shop B.V." and
// "Example Shop" are the same supplier.
var legalForms = map[string]bool{
	"bv": true, "nv": true, "vof": true, "cv": true, "ltd": true, "limited": true, "gmbh": true, "inc": true, "llc": true,
	"plc": true, "sa": true, "sarl": true, "srl": true, "ag": true, "co": true, "corp": true, "bvba": true, "ug": true,
	"eenmanszaak": true, "the": true,
}

// NormalizeName reduces a company name to compare it: lower case words
// without punctuation and legal forms.
func NormalizeName(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), ".", "")
	words := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	out := words[:0]
	for _, w := range words {
		if !legalForms[w] {
			out = append(out, w)
		}
	}
	return strings.Join(out, " ")
}

// NormalizeVAT reduces a VAT number to letters and digits in upper case.
func NormalizeVAT(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToUpper(r)
		}
		return -1
	}, s)
}

// NormalizeKVK keeps the digits of a KVK number.
func NormalizeKVK(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// Domain returns the domain of an email address or website that says who
// sent a document: none for free mail.
func Domain(emailOrWebsite string) string {
	s := strings.ToLower(strings.TrimSpace(emailOrWebsite))
	if _, d, ok := strings.Cut(s, "@"); ok {
		s = d
	} else if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil {
			s = u.Hostname()
		}
	} else {
		s, _, _ = strings.Cut(s, "/")
	}
	s = strings.TrimPrefix(strings.Trim(s, ". "), "www.")
	if !strings.Contains(s, ".") || freeMail[s] {
		return ""
	}
	return s
}

// SearchTerms are what to search contacts for to find the supplier: its
// IBAN, VAT and KVK numbers, email domain and the first word of its name.
func (s Supplier) SearchTerms() []string {
	var out []string
	add := func(v string) {
		if v != "" {
			out = append(out, v)
		}
	}
	add(iban.Normalize(s.IBAN))
	add(NormalizeVAT(s.VATNumber))
	if kvk := NormalizeKVK(s.KVK); len(kvk) == 8 {
		add(kvk)
	}
	if d := Domain(s.Email); d != "" {
		add("@" + d)
	} else if d := Domain(s.Website); d != "" {
		add("@" + d)
	}
	if words := strings.Fields(NormalizeName(s.Name)); len(words) > 0 {
		add(words[0])
	}
	return out
}

// Match finds the contact that is the supplier among candidates: by IBAN,
// then VAT number, KVK number, email domain and at last company name.
func Match(s Supplier, candidates []contacts.Contact) (contacts.Contact, MatchedBy, bool) {
	keys := []struct {
		by    MatchedBy
		want  string
		match func(c contacts.Contact, want string) bool
	}{
		{ByIBAN, iban.Normalize(s.IBAN), func(c contacts.Contact, want string) bool {
			return iban.Normalize(c.SepaIBAN) == want || iban.Normalize(c.BankAccount) == want
		}},
		{ByVATNumber, NormalizeVAT(s.VATNumber), func(c contacts.Contact, want string) bool { return NormalizeVAT(c.TaxNumber) == want }},
		{ByKVK, NormalizeKVK(s.KVK), func(c contacts.Contact, want string) bool { return NormalizeKVK(c.ChamberOfCommerce) == want }},
		{ByEmailDomain, cmpOr(Domain(s.Email), Domain(s.Website)), func(c contacts.Contact, want string) bool {
			for _, e := range contactEmails(c) {
				if Domain(e) == want {
					return true
				}
			}
			return false
		}},
		{ByName, NormalizeName(s.Name), func(c contacts.Contact, want string) bool { return NormalizeName(c.Name()) == want }},
	}
	for _, k := range keys {
		if k.want == "" {
			continue
		}
		for _, c := range candidates {
			if k.match(c, k.want) {
				return c, k.by, true
			}
		}
	}
	return contacts.Contact{}, "", false
}

func contactEmails(c contacts.Contact) []string {
	all := []string{c.SendInvoicesToEmail, c.SendEstimatesToEmail}
	for _, p := range c.People {
		all = append(all, p.Email)
	}
	var out []string
	for _, a := range all {
		out = append(out, strings.FieldsFunc(a, func(r rune) bool { return r == ',' || r == ';' || r == ' ' })...)
	}
	return out
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
