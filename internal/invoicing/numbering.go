package invoicing

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// NumberTags are the placeholders a number template may contain, after
// Moneybird. {id} is the sequence number and is required.
var NumberTags = []string{"id", "year", "year_two_digits", "quarter", "month", "month_name", "week", "day", "customer_id"}

const maxTemplateLength = 50

var (
	tagPattern      = regexp.MustCompile(`\{([a-z_]+)\}`)
	sequencePattern = regexp.MustCompile(`^[0-9]{1,10}$`)
	dutchMonths     = []string{"januari", "februari", "maart", "april", "mei", "juni", "juli", "augustus", "september", "oktober", "november", "december"}
)

// RenderNumber fills in a number template for a document dated at. The
// sequence keeps its leading zeros.
func RenderNumber(template, sequence string, at time.Time, language, customerID string) string {
	_, week := at.ISOWeek()
	month := at.Month().String()
	if strings.HasPrefix(language, "nl") {
		month = dutchMonths[at.Month()-1]
	}
	values := map[string]string{
		"id":              sequence,
		"year":            strconv.Itoa(at.Year()),
		"year_two_digits": fmt.Sprintf("%02d", at.Year()%100),
		"quarter":         strconv.Itoa((int(at.Month())-1)/3 + 1),
		"month":           fmt.Sprintf("%02d", int(at.Month())),
		"month_name":      month,
		"week":            fmt.Sprintf("%02d", week),
		"day":             fmt.Sprintf("%02d", at.Day()),
		"customer_id":     customerID,
	}
	return tagPattern.ReplaceAllStringFunc(template, func(tag string) string {
		if v, ok := values[tag[1:len(tag)-1]]; ok {
			return v
		}
		return tag
	})
}

// validateTemplate reports why a number template cannot be used, or nil.
func validateTemplate(template string) error {
	switch {
	case template == "" || len(template) > maxTemplateLength || strings.ContainsAny(template, "\r\n\t"):
		return errors.New("must be a single line of at most 50 characters")
	case !strings.Contains(template, "{id}"):
		return errors.New("must contain {id}")
	}
	for _, m := range tagPattern.FindAllStringSubmatch(template, -1) {
		if !slices.Contains(NumberTags, m[1]) {
			return fmt.Errorf("contains the unknown tag {%s}", m[1])
		}
	}
	return nil
}

// validateSequence reports why a next sequence number cannot be used, or nil.
func validateSequence(sequence string) error {
	if !sequencePattern.MatchString(sequence) || strings.Trim(sequence, "0") == "" {
		return errors.New("must be a positive number of at most 10 digits")
	}
	return nil
}
