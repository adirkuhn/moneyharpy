package purchases

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Why an incoming document may be a duplicate of another, as the API names
// it.
const (
	// SameFile: an attachment of each has the same contents.
	SameFile = "same_file"
	// SameReference: the same supplier and a reference alike.
	SameReference = "same_reference"
	// SameAmountAndDate: the same supplier and total, at most a week apart.
	SameAmountAndDate = "same_amount_and_date"
	// SameLines: the same supplier and mostly the same lines.
	SameLines = "same_lines"
)

// ComparedKinds are the kinds of document duplicate detection compares,
// whose attachments it needs the sums of.
var ComparedKinds = []string{PurchaseInvoice, Receipt, GeneralDocument, TypelessDocument}

// duplicateDays is how far apart documents of the same supplier and total
// may be dated to look like the same one.
const duplicateDays = 7

// DuplicateCandidate is what duplicate detection compares of an incoming
// document.
type DuplicateCandidate struct {
	ID   resource.ID
	Kind string
	// ContactID is zero without a supplier.
	ContactID    resource.ID
	Reference    string
	Date         time.Time
	TotalInclTax money.Amount
	// Hashes are the SHA-256 sums of its attachments' contents; an
	// attachment not summed yet has none.
	Hashes []string
	// Lines are those of a purchase invoice or receipt.
	Lines []DuplicateLine
}

// DuplicateLine is what duplicate detection compares of a line.
type DuplicateLine struct {
	Description string
	Price       money.Amount
	// Quantity is in thousandths.
	Quantity int64
}

// Duplicate is another document an incoming document may be a copy of,
// and why.
type Duplicate struct {
	ID           resource.ID
	Kind         string
	Reference    string
	Date         time.Time
	TotalInclTax money.Amount
	// Reasons are one or more of SameFile, SameReference,
	// SameAmountAndDate and SameLines, in that order.
	Reasons []string
}

// PossibleDuplicates finds, for each document with one of the ids, the
// other documents of pool it may be a copy of, oldest first. pool holds the
// documents themselves and those to compare them with; a document without
// any is left out of the answer.
func PossibleDuplicates(ids []resource.ID, pool []DuplicateCandidate) map[resource.ID][]Duplicate {
	byID := make(map[resource.ID]int, len(pool))
	for i, c := range pool {
		byID[c.ID] = i
	}
	out := map[resource.ID][]Duplicate{}
	for _, id := range ids {
		i, ok := byID[id]
		if !ok {
			continue
		}
		var found []Duplicate
		for j, other := range pool {
			if j == i {
				continue
			}
			if reasons := duplicateReasons(pool[i], other); len(reasons) > 0 {
				found = append(found, Duplicate{
					ID: other.ID, Kind: other.Kind, Reference: other.Reference, Date: other.Date, TotalInclTax: other.TotalInclTax, Reasons: reasons,
				})
			}
		}
		if len(found) == 0 {
			continue
		}
		slices.SortFunc(found, func(a, b Duplicate) int {
			return cmp.Or(a.Date.Compare(b.Date), cmp.Compare(a.ID, b.ID))
		})
		out[id] = found
	}
	return out
}

// duplicateReasons tells why a and b may be the same document; none when
// they look like different ones. It is symmetric.
func duplicateReasons(a, b DuplicateCandidate) []string {
	var reasons []string
	if sharesHash(a.Hashes, b.Hashes) {
		reasons = append(reasons, SameFile)
	}
	if a.ContactID == 0 || a.ContactID != b.ContactID {
		return reasons
	}
	if referencesAlike(a.Reference, b.Reference) {
		reasons = append(reasons, SameReference)
	}
	if a.TotalInclTax != 0 && a.TotalInclTax == b.TotalInclTax && daysApart(a.Date, b.Date) <= duplicateDays {
		reasons = append(reasons, SameAmountAndDate)
	}
	if linesAlike(a.Lines, b.Lines) {
		reasons = append(reasons, SameLines)
	}
	return reasons
}

func sharesHash(a, b []string) bool {
	for _, h := range a {
		if h != "" && slices.Contains(b, h) {
			return true
		}
	}
	return false
}

func daysApart(a, b time.Time) int {
	d := a.Sub(b)
	if d < 0 {
		d = -d
	}
	return int(d.Round(time.Hour).Hours() / 24)
}

// fileName is a reference that is only the name of an uploaded file, which
// an upload gets until its own reference is known.
var fileName = regexp.MustCompile(`(?i)^[^/\\]+\.(?:pdf|jpe?g|png|gif|tiff?|webp|heic|xml|eml|msg|txt)$`)

// IsFileName reports whether a reference is only the name of an uploaded
// file.
func IsFileName(reference string) bool { return fileName.MatchString(strings.TrimSpace(reference)) }

// referencesAlike tells whether two references of the same supplier name
// the same document: written alike once lowercased without spaces and
// punctuation; or, of four characters or more, one holding the other with
// its numbers whole; or, of six or more, at most two letters apart with the
// same numbers. A next invoice number is never alike, and a reference
// without a number, such as "Lunch", or a file name names no document.
func referencesAlike(a, b string) bool {
	if IsFileName(a) || IsFileName(b) {
		return false
	}
	na, nb := normalize(a), normalize(b)
	if !hasDigit(na) || !hasDigit(nb) {
		return false
	}
	if na == nb {
		return true
	}
	if len([]rune(na)) > len([]rune(nb)) {
		a, b, na, nb = b, a, nb, na
	}
	shorter, longer := numbers(a), numbers(b)
	if len([]rune(na)) >= 4 && strings.Contains(nb, na) && containsRun(longer, shorter) {
		return true
	}
	return len([]rune(na)) >= 6 && slices.Equal(shorter, longer) && editDistance(na, nb) <= 2
}

// normalize lowercases s and keeps only its letters and digits.
func normalize(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

func hasDigit(s string) bool { return strings.ContainsFunc(s, unicode.IsDigit) }

// numbers are the runs of digits in s, in order.
func numbers(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsDigit(r) })
}

// containsRun reports whether part appears in whole as consecutive items.
func containsRun(whole, part []string) bool {
	for i := 0; i+len(part) <= len(whole); i++ {
		if slices.Equal(whole[i:i+len(part)], part) {
			return true
		}
	}
	return false
}

// editDistance is the Levenshtein distance between a and b in runes.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// duplicateLineShare is the share of lines, in percent, two documents must
// have in common to look like the same one.
const duplicateLineShare = 80

// linesAlike tells whether at least 80% of the lines of the longer
// document match one of the other's on description (as normalize writes
// it), price and quantity; each line matches once.
func linesAlike(a, b []DuplicateLine) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	type key struct {
		description string
		price       money.Amount
		quantity    int64
	}
	left := map[key]int{}
	for _, l := range a {
		left[key{normalize(l.Description), l.Price, l.Quantity}]++
	}
	matched := 0
	for _, l := range b {
		k := key{normalize(l.Description), l.Price, l.Quantity}
		if left[k] > 0 {
			left[k]--
			matched++
		}
	}
	return matched*100 >= max(len(a), len(b))*duplicateLineShare
}
