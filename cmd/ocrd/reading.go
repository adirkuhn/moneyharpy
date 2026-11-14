package main

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// DefaultPageModes are the page modes a photo or short scan is read with:
// Tesseract's automatic layout first, then one column of lines and one
// block of text, which read till receipts better.
var DefaultPageModes = []string{"3", "6", "4"}

var (
	readingAmount = regexp.MustCompile(`(?:^|[\s€(])\d{1,6}[.,]\d{2}(?:$|[\s)])`)
	readingDate   = regexp.MustCompile(`\b\d{1,2}[-/.]\d{1,2}[-/.](?:\d{4}|\d{2})\b`)
)

// readingScore tells how well a reading looks read: amounts and dates in
// their usual print, words of letters, less the tokens of noise OCR makes
// of specks and bad segmentation ("25,9%", "“Lon").
func readingScore(text string) int {
	score := 2*len(readingAmount.FindAllStringIndex(text, -1)) + 2*len(readingDate.FindAllStringIndex(text, -1))
	for _, tok := range strings.Fields(text) {
		switch {
		case noisy(tok):
			score--
		case isWord(tok):
			score++
		}
	}
	return score
}

// isWord tells whether tok is a word of at least three letters, perhaps
// followed by punctuation.
func isWord(tok string) bool {
	tok = strings.TrimRight(tok, ".,:;!?")
	n := 0
	for _, r := range tok {
		if !unicode.IsLetter(r) {
			return false
		}
		n++
	}
	return n >= 3
}

// noisy tells whether tok holds marks OCR makes of specks, or is mostly
// marks.
func noisy(tok string) bool {
	if strings.ContainsAny(tok, "“”„‘’|~^{}<>«»¦§¬") {
		return true
	}
	marks, all := 0, 0
	for _, r := range tok {
		all++
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune(".,:-/€%()&+'", r) {
			marks++
		}
	}
	return all >= 2 && 2*marks > all
}

// bestFirst orders readings by score, keeping their order between equals:
// the order of the page modes, then RapidOCR.
func bestFirst(readings []Reading) []Reading {
	scores := make([]int, len(readings))
	order := make([]int, len(readings))
	for i, r := range readings {
		scores[i], order[i] = readingScore(r.Text), i
	}
	slices.SortStableFunc(order, func(a, b int) int { return scores[b] - scores[a] })
	out := make([]Reading, len(readings))
	for i, o := range order {
		out[i] = readings[o]
	}
	return out
}
