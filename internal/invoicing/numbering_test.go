package invoicing

import (
	"errors"
	"testing"
	"time"
)

func TestRenderNumber(t *testing.T) {
	at := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC) // Tuesday of ISO week 46.
	tests := []struct {
		template, sequence, language, want string
	}{
		{"{year}-{id}", "1462", "en", "2026-1462"},
		{"{year_two_digits}{month}{day}-{id}", "0027", "en", "261110-0027"},
		{"Q{quarter}/W{week}/{id}", "7", "en", "Q4/W46/7"},
		{"{month_name}-{id}", "1", "en", "November-1"},
		{"{month_name}-{id}", "1", "nl", "november-1"},
		{"{customer_id}-{id}", "5", "en", "1-5"},
	}
	for _, tt := range tests {
		if got := RenderNumber(tt.template, tt.sequence, at, tt.language, "1"); got != tt.want {
			t.Errorf("RenderNumber(%q, %q) = %q, want %q", tt.template, tt.sequence, got, tt.want)
		}
	}
}

func TestValidateNumbering(t *testing.T) {
	for _, tt := range []struct {
		template, sequence string
		ok                 bool
	}{
		{"{year}-{id}", "0001", true},
		{"INV{id}", "12", true},
		{"{year}", "1", false},         // No {id}: numbers would repeat.
		{"{id}-{nope}", "1", false},    // Unknown tag.
		{"{id}\n", "1", false},         // Line break.
		{"{id}", "", false},            // No next number.
		{"{id}", "12a", false},         // Not digits.
		{"{id}", "0", false},           // Must be positive.
		{"{id}", "12345678901", false}, // Too long.
		{"{id}" + string(make([]byte, 60)), "1", false},
	} {
		err := errors.Join(validateTemplate(tt.template), validateSequence(tt.sequence))
		if (err == nil) != tt.ok {
			t.Errorf("validate(%q, %q) error = %v, want ok %v", tt.template, tt.sequence, err, tt.ok)
		}
	}
}
