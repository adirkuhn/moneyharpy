package sales

import (
	"testing"
	"time"
)

func TestEstimateTextSpans(t *testing.T) {
	d := tagDocument()
	d.Estimate = &Estimate{}
	text := "Dear {recipient.name},\nhere is quote *{document.estimate_id}* of {document.estimate_date}."
	spans := EstimateTextSpans(d, text, time.Now())
	if len(spans) != 3 || spans[0].Text != "Dear Example Shop B.V.,\nhere is quote " || spans[1] != (Span{Text: "2026-0001", Bold: true}) || spans[2].Text != " of 11-11-2026." {
		t.Errorf("sent = %+v", spans)
	}
	d.Invoice.InvoiceID, d.Invoice.InvoiceDate = "", nil
	today := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	if got := EstimateTextSpans(d, "{document.estimate_id} {document.estimate_date}", today); len(got) != 1 || got[0].Text != "Concept #3 01-12-2026" {
		t.Errorf("draft in Dutch = %+v", got)
	}
	d.Invoice.Language = "en"
	if got := EstimateTextSpans(d, "{document.estimate_id}", today); got[0].Text != "Draft #3" {
		t.Errorf("draft in English = %+v", got)
	}
	if got := EstimateTextSpans(d, "", today); len(got) != 0 {
		t.Errorf("empty = %+v", got)
	}
}
