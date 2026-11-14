package recognize

import (
	"time"

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
	return best
}

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
