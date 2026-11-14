package recognize

import (
	"strings"
	"time"
	"unicode"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

// agreeAcross checks the reading taken as best against all OCR readings of
// the same file (texts, and what the rules made of each, parsed). OCR
// misreads differently with each page mode, so a value most readings agree
// on is more likely right than the best reading's alone:
//
//   - The date is put to a vote over every occurrence in every reading,
//     between the best reading's date and the dates one digit apart from
//     it (its possible misreadings; other dates, such as a due date, are
//     other things). The most votes win, a tie goes to a labelled date;
//     the date is certain only with a clear majority (at least two votes,
//     and twice as many as any rival).
//   - The total is not replaced, as the VAT and lines belong to it, but
//     it is uncertain when more readings are sure of another total than
//     of this one. Totals a reading is not sure of do not vote: they are
//     often garbage.
//   - The supplier is combined from all readings (agreeSupplier).
//
// With a single reading the best one is returned as it is.
func agreeAcross(best Extraction, texts []string, parsed []Extraction, today time.Time) Extraction {
	if len(texts) < 2 {
		return best
	}
	if best.Date.Found {
		best.Date = voteDate(best.Date, texts, today)
	}
	if best.TotalInclTax.Found && best.TotalInclTax.Confidence == Certain && outvoted(best.TotalInclTax.V, parsed) {
		best.TotalInclTax.Confidence = Uncertain
	}
	best.Supplier, best.SupplierFound = agreeSupplier(best, parsed)
	return best
}

// agreeSupplier combines the supplier the readings found: one reading may
// garble the name while another cuts off the line with the VAT number.
//
//   - The VAT number, KVK number, IBAN and e-mail address are each the
//     valid value most readings read; a tie keeps the best reading's. A
//     value only one reading read counts when nothing valid conflicts.
//   - The name is put to a vote among the readings' names that are not
//     garbled: a name with a legal form (B.V., GmbH …) first, then the one
//     most readings read alike (similarNames, so a garbled spelling votes
//     for the clean one), a tie keeping the best reading's. Of the spellings
//     alike, the cleanest is taken (cleanerName).
//   - The supplier is certain when a valid VAT number or KVK number backs
//     it, when at least two readings read the name alike, or when the best
//     reading was already certain of the same name.
func agreeSupplier(best Extraction, parsed []Extraction) (Supplier, Confidence) {
	s := best.Supplier
	field := func(get func(Supplier) string) []string {
		out := make([]string, len(parsed))
		for i, e := range parsed {
			out[i] = get(e.Supplier)
		}
		return out
	}
	vats := field(func(x Supplier) string { return x.VATNumber })
	vat, vatVotes := voteID(s.VATNumber, vats, ValidVAT)
	s.VATNumber = vat
	s.KVK, _ = voteID(s.KVK, field(func(x Supplier) string { return x.KVK }), validKVK)
	s.IBAN, _ = voteID(s.IBAN, field(func(x Supplier) string { return x.IBAN }), ValidIBAN)
	s.Email, _ = voteID(s.Email, field(func(x Supplier) string { return x.Email }), validEmail)
	if s.Country == "" {
		s.Country = countryOf(s.VATNumber)
	}
	names := field(func(x Supplier) string { return x.Name })
	s.Name = voteName(best.Supplier.Name, names)
	support := 0
	for _, n := range names {
		if s.Name != "" && similarNames(n, s.Name) {
			support++
		}
	}

	vatSure := ValidVAT(s.VATNumber) && (strings.HasPrefix(s.VATNumber, "NL") || vatVotes >= 2)
	for i, e := range parsed {
		if e.SupplierFound == Certain && vats[i] == s.VATNumber && ValidVAT(s.VATNumber) {
			vatSure = true
		}
	}
	switch {
	case s == Supplier{}:
		return s, best.SupplierFound
	case vatSure, s.KVK != "", support >= 2,
		best.SupplierFound == Certain && (s.Name == "" || similarNames(s.Name, best.Supplier.Name)):
		return s, Certain
	}
	return s, Uncertain
}

// voteID picks the valid value most of values agree on; a tie keeps
// current, and so does having no valid value at all. It also returns the
// votes of the value picked.
func voteID(current string, values []string, valid func(string) bool) (string, int) {
	votes := map[string]int{}
	for _, v := range values {
		if v != "" && valid(v) {
			votes[v]++
		}
	}
	top, tie := current, false
	for v, n := range votes {
		switch {
		case v == top:
		case n > votes[top]:
			top, tie = v, false
		case n == votes[top]:
			tie = true
		}
	}
	if tie && votes[current] < votes[top] {
		// Valid values tie without the current one among them: it is not
		// clear which is right, so nothing changes.
		return current, votes[current]
	}
	if tie {
		top = current
	}
	return top, votes[top]
}

// voteName puts the supplier's name to the vote described at agreeSupplier;
// current is the best reading's name.
func voteName(current string, names []string) string {
	var candidates []string
	for _, n := range names {
		if n != "" && !garbledName(n) {
			candidates = append(candidates, n)
		}
	}
	if len(candidates) == 0 {
		return current
	}
	support := func(c string) int {
		n := 0
		for _, o := range candidates {
			if similarNames(o, c) {
				n++
			}
		}
		return n
	}
	rank := func(c string) (bool, int) { return legalForm.MatchString(c), support(c) }
	top := ""
	for _, c := range candidates {
		if top == "" {
			top = c
			continue
		}
		cl, cs := rank(c)
		tl, ts := rank(top)
		switch {
		case cl != tl:
			if cl {
				top = c
			}
		case cs > ts, cs == ts && current != "" && similarNames(c, current) && !similarNames(top, current):
			top = c
		}
	}
	// The cleanest spelling of the winning name.
	exact := map[string]int{}
	for _, c := range candidates {
		exact[c]++
	}
	name := top
	for _, c := range candidates {
		if similarNames(c, top) && cleanerName(c, name, exact, current) {
			name = c
		}
	}
	return name
}

// cleanerName tells whether spelling a of a name is cleaner than b: more
// of its characters are letters, more of its words read like words, it is
// read more often exactly, or it is the best reading's (current).
func cleanerName(a, b string, exact map[string]int, current string) bool {
	if la, lb := letterShare(baseName(a)), letterShare(baseName(b)); la != lb {
		return la > lb
	}
	if wa, wb := wordShare(baseName(a)), wordShare(baseName(b)); wa != wb {
		return wa > wb
	}
	if exact[a] != exact[b] {
		return exact[a] > exact[b]
	}
	return a == current && b != current
}

// baseName is a name without its legal form.
func baseName(s string) string {
	if m := legalForm.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}

// letterShare is the share of a name's printed characters that are letters.
func letterShare(s string) float64 {
	letters, all := 0, 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		all++
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if all == 0 {
		return 0
	}
	return float64(letters) / float64(all)
}

// wordShare is the share of a name's words that read like words: letters
// only, with a vowel. OCR garbles vowels into consonants ("Shcp" for
// "Shop") more than the other way round.
func wordShare(s string) float64 {
	words := strings.Fields(s)
	if len(words) == 0 {
		return 0
	}
	n := 0
	for _, w := range words {
		vowel, letters := false, true
		for _, r := range w {
			if !unicode.IsLetter(r) {
				letters = false
			}
			if strings.ContainsRune(vowels, unicode.ToLower(r)) {
				vowel = true
			}
		}
		if vowel && letters {
			n++
		}
	}
	return float64(n) / float64(len(words))
}

const vowels = "aeiouyàáâäãåèéêëìíîïòóôöõùúûüý"

// minNameSimilarity is how alike two spellings of a name must be, as one
// minus their edit distance over the longer one's length, to be taken for
// the same name: "Exarnple Shcp BV" is 0.81 like "Example Shop BV".
const minNameSimilarity = 0.8

// similarNames tells whether two names are spellings of the same name,
// ignoring case, dots and spacing.
func similarNames(a, b string) bool {
	a, b = nameKey(a), nameKey(b)
	if a == "" || b == "" {
		return false
	}
	longest := max(len([]rune(a)), len([]rune(b)))
	return 1-float64(editDistance(a, b))/float64(longest) >= minNameSimilarity
}

func nameKey(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(strings.ToLower(s), ".", "")), " ")
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

func validKVK(s string) bool { return len(s) == 8 && isDigits(s) }

func validEmail(s string) bool { return emailRe.FindString(s) == s }

// dateVotes counts the occurrences of the plausible dates in all texts, and
// tells which are labelled as the document date and which as another date.
func dateVotes(texts []string, today time.Time) (votes map[time.Time]int, labelled, other map[time.Time]bool) {
	votes, labelled, other = map[time.Time]int{}, map[time.Time]bool{}, map[time.Time]bool{}
	for _, text := range texts {
		lines := prepare(text)
		counts, _ := dateCounts(lines, today)
		for t, n := range counts {
			votes[t] += n
		}
		for _, d := range labelledDates(lines) {
			switch d.kind {
			case labelDate, labelGeneric:
				labelled[d.t] = true
			default:
				other[d.t] = true
			}
		}
	}
	return votes, labelled, other
}

// voteDate puts the date to the vote described at agreeAcross.
func voteDate(date Value[time.Time], texts []string, today time.Time) Value[time.Time] {
	votes, labelled, other := dateVotes(texts, today)
	candidates := []time.Time{date.V}
	for t := range votes {
		if !other[t] && oneDigitApart(t, date.V) {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 1 {
		return date
	}
	top, tie := date.V, false
	for _, t := range candidates[1:] {
		switch {
		case votes[t] > votes[top], votes[t] == votes[top] && labelled[t] && !labelled[top]:
			top, tie = t, false
		case votes[t] == votes[top] && labelled[t] == labelled[top]:
			tie = true
		}
	}
	rival := 0
	for _, t := range candidates {
		if !t.Equal(top) {
			rival = max(rival, votes[t])
		}
	}
	if !tie && votes[top] >= 2 && votes[top] >= 2*rival {
		return sure(top)
	}
	return unsure(top)
}

// outvoted tells whether more readings are sure of a total other than
// total than of total itself.
func outvoted(total money.Amount, parsed []Extraction) bool {
	votes := map[money.Amount]int{}
	for _, e := range parsed {
		if e.TotalInclTax.Found && e.TotalInclTax.Confidence == Certain {
			votes[e.TotalInclTax.V]++
		}
	}
	for v, n := range votes {
		if v != total && n > votes[total] {
			return true
		}
	}
	return false
}
