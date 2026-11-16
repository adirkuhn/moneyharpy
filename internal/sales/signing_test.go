package sales

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// pngSignature stands in for a drawn signature: only its PNG header counts.
var pngSignature = []byte("\x89PNG\r\n\x1a\nsignature")

func signingOf(t *testing.T, h harness, id resource.ID) Signing {
	t.Helper()
	return h.store.signings[id]
}

func TestSendingToSignOnlineGivesTheContactALink(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := estimate(t, h, EstimateChanges{})
	if !e.SignOnline {
		t.Fatal("a new estimate is not signed online by default, as on Moneybird")
	}
	if _, err := h.svc.SendEstimate(ctx, adminID, e.ID, owner, EstimateSendInput{}); err != nil {
		t.Fatal(err)
	}
	g := signingOf(t, h, e.ID)
	if !regexp.MustCompile(`^\d{12}$`).MatchString(g.ViewCode) || g.ViewCodeExpiresAt == nil || !g.ViewCodeExpiresAt.Equal(now.Add(viewCodeLife)) || g.SentByUserID == nil || *g.SentByUserID != owner {
		t.Fatalf("signing = %+v", g)
	}
	link := "https://books.example.com/online/estimates/" + e.ID.String() + "?code=" + g.ViewCode
	m := h.mailer.sent[0]
	if !strings.Contains(m.Body, link) || m.Look == nil || m.Look.Button == nil || m.Look.Button.URL != link {
		t.Errorf("mail = %q, look = %+v", m.Body, m.Look)
	}
	got, err := h.svc.Estimate(ctx, adminID, e.ID, owner)
	if err != nil || got.Signing == nil || got.Signing.ViewCode != g.ViewCode {
		t.Errorf("estimate signing = %+v, %v", got.Signing, err)
	}

	// Sending again makes a new code; the old one no longer opens it.
	if _, err := h.svc.SendEstimate(ctx, adminID, e.ID, owner, EstimateSendInput{}); err != nil {
		t.Fatal(err)
	}
	again := signingOf(t, h, e.ID)
	if again.ViewCode == g.ViewCode {
		t.Error("resending kept the access code")
	}
	if _, err := h.svc.OnlineEstimate(ctx, e.ID, g.ViewCode); !errors.Is(err, ErrEstimateNotFound) {
		t.Errorf("old code = %v", err)
	}

	// Not to be signed online: no link.
	plain := estimate(t, h, EstimateChanges{})
	if _, err := h.svc.SendEstimate(ctx, adminID, plain.ID, owner, EstimateSendInput{SignOnline: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if last := h.mailer.sent[len(h.mailer.sent)-1]; strings.Contains(last.Body, "/online/estimates/") || last.Look.Button != nil || signingOf(t, h, plain.ID).ViewCode != "" {
		t.Errorf("unsigned mail = %q", last.Body)
	}
}

func TestSenderSignsBeforeSending(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := estimate(t, h, EstimateChanges{})
	if _, err := h.svc.SendEstimate(ctx, adminID, e.ID, owner, EstimateSendInput{Sender: true}); fieldsOf(t, err)["signature_output"] == nil {
		t.Errorf("signing myself without a signature: %v", err)
	}
	if _, err := h.svc.SendEstimate(ctx, adminID, e.ID, owner, EstimateSendInput{Sender: true, SenderSignature: []byte("not an image")}); fieldsOf(t, err)["signature_output"] == nil {
		t.Errorf("a signature that is no PNG: %v", err)
	}
	if _, err := h.svc.SendEstimate(ctx, adminID, e.ID, owner, EstimateSendInput{Sender: true, SenderSignature: pngSignature}); err != nil {
		t.Fatal(err)
	}
	g := signingOf(t, h, e.ID)
	if string(g.SenderSignature) != string(pngSignature) || g.SenderSignedAt == nil || g.SenderUserID == nil || *g.SenderUserID != owner {
		t.Errorf("sender signature = %+v", g)
	}
}

func TestContactSignsOnline(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := estimate(t, h, EstimateChanges{})
	if _, err := h.svc.SendEstimate(ctx, adminID, e.ID, owner, EstimateSendInput{Sender: true, SenderSignature: pngSignature}); err != nil {
		t.Fatal(err)
	}
	code := signingOf(t, h, e.ID).ViewCode
	if _, err := h.svc.OnlineEstimate(ctx, e.ID, "000000000000"); !errors.Is(err, ErrEstimateNotFound) {
		t.Errorf("wrong code = %v", err)
	}
	online, err := h.svc.OnlineEstimate(ctx, e.ID, code)
	if err != nil || online.Estimate.ID != e.ID || !online.Signable || online.Sender().CompanyName != "Harpy B.V." || online.ContactName() != "Example Shop B.V." {
		t.Fatalf("online = %+v, %v", online, err)
	}

	in := SignInput{Name: "Jan Voorbeeld", Email: "jan@example.com", Signature: pngSignature, AuditTrail: `[[1,2]]`, IP: "203.0.113.7", UserAgent: "Test browser"}
	for field, bad := range map[string]SignInput{
		"name": {Email: in.Email, Signature: in.Signature}, "email": {Name: in.Name, Email: "jan", Signature: in.Signature},
		"signature_output": {Name: in.Name, Email: in.Email},
	} {
		if _, err := h.svc.SignEstimateOnline(ctx, e.ID, code, bad); fieldsOf(t, err)[field] == nil {
			t.Errorf("without a good %s: %v", field, err)
		}
	}
	mailed := len(h.mailer.sent)
	signed, err := h.svc.SignEstimateOnline(ctx, e.ID, code, in)
	if err != nil {
		t.Fatal(err)
	}
	if signed.State != Accepted || signed.AcceptedAt == nil || signed.Signing == nil || !signed.Signing.Signed() || signed.Signing.SignerIP != "203.0.113.7" {
		t.Fatalf("signed = %+v", signed)
	}
	var got []string
	for _, ev := range signed.Events {
		got = append(got, ev.Action)
	}
	if !slices.Contains(got, "estimate_signed_online") {
		t.Errorf("history = %v", got)
	}
	// Both the contact and the sender get the signed quote.
	sent := h.mailer.sent[mailed:]
	var to []string
	for _, m := range sent {
		to = append(to, m.To...)
		if len(m.Files) != 1 || !strings.HasPrefix(m.Files[0].Name, "quote-") || !strings.Contains(string(m.Files[0].Data), "signed") {
			t.Errorf("files = %+v", m.Files)
		}
	}
	slices.Sort(to)
	if !slices.Equal(to, []string{"books@example.com", "jan@example.com"}) {
		t.Errorf("signed quote mailed to %v", to)
	}

	if _, err := h.svc.SignEstimateOnline(ctx, e.ID, code, in); err == nil {
		t.Error("signed twice")
	}
	if online, _ := h.svc.OnlineEstimate(ctx, e.ID, code); online.Signable {
		t.Error("still signable once signed")
	}
	pdf, name, err := h.svc.SignedEstimatePDF(ctx, adminID, e.ID, owner)
	if err != nil || !strings.Contains(string(pdf), "signed") || !strings.HasSuffix(name, ".pdf") {
		t.Errorf("signed pdf = %q %s %v", pdf, name, err)
	}
}

func TestOnlyOpenEstimatesAreSignedOnline(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := sentEstimate(t, h)
	code := signingOf(t, h, e.ID).ViewCode
	if _, err := h.svc.ChangeEstimateState(ctx, adminID, e.ID, owner, Rejected); err != nil {
		t.Fatal(err)
	}
	in := SignInput{Name: "Jan Voorbeeld", Email: "jan@example.com", Signature: pngSignature}
	if _, err := h.svc.SignEstimateOnline(ctx, e.ID, code, in); fieldsOf(t, err)["base"] == nil {
		t.Errorf("signed a rejected quote: %v", err)
	}

	// An expired code opens nothing.
	other := sentEstimate(t, h)
	*h.clock = now.Add(viewCodeLife + time.Hour)
	if _, err := h.svc.OnlineEstimate(ctx, other.ID, signingOf(t, h, other.ID).ViewCode); !errors.Is(err, ErrEstimateNotFound) {
		t.Errorf("expired code = %v", err)
	}
}
