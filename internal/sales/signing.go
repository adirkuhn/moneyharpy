package sales

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// viewCodeLife is how long an estimate's online code opens it, about the
// three months of Moneybird's public_view_code_expires_at.
const viewCodeLife = 90 * 24 * time.Hour

// maxSignature caps a drawn signature image.
const maxSignature = 256 << 10

// Signing is how an estimate is accepted online, after Moneybird's: the
// code that opens it for the contact, the sender's own signature, and the
// contact's signature with what is kept as evidence of it.
type Signing struct {
	AdministrationID resource.ID
	EstimateID       resource.ID
	// ViewCode opens the estimate online until ViewCodeExpiresAt; a new
	// sending replaces it.
	ViewCode          string
	ViewCodeExpiresAt *time.Time
	// SentByUserID sent it to be signed; the contact's online actions are
	// done in their name.
	SentByUserID *resource.ID
	// SenderSignature is the sender's own signature, a PNG, shown on the
	// quote only once the contact signed too.
	SenderSignature []byte
	SenderSignedAt  *time.Time
	SenderUserID    *resource.ID
	// The contact's signature, a PNG, with the name, email address, network
	// address and browser it was given with and the strokes drawn.
	SignerName      string
	SignerEmail     string
	SignerIP        string
	SignerUserAgent string
	Signature       []byte
	AuditTrail      string
	SignedAt        *time.Time
	UpdatedAt       time.Time
}

// Signed reports whether the contact signed.
func (g Signing) Signed() bool { return g.SignedAt != nil }

// opens reports whether code opens the estimate at now.
func (g Signing) opens(code string, now time.Time) bool {
	return g.ViewCode != "" && g.ViewCodeExpiresAt != nil && now.Before(*g.ViewCodeExpiresAt) &&
		subtle.ConstantTimeCompare([]byte(g.ViewCode), []byte(strings.TrimSpace(code))) == 1
}

// SigningStore keeps the online signing of estimates.
type SigningStore interface {
	// EstimateSigning returns an estimate's signing; ok is false when it was
	// never sent to be signed. admin 0 finds it by the estimate alone.
	EstimateSigning(ctx context.Context, admin, estimate resource.ID) (g Signing, ok bool, err error)
	SaveSigning(ctx context.Context, g Signing) error
}

// isPNG reports whether data is a PNG image of an acceptable size.
func isPNG(data []byte) bool {
	return len(data) <= maxSignature && bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n"))
}

// newViewCode makes a random 12-digit code, as Moneybird's are digits.
func newViewCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%012d", n), nil
}

// OnlineURL is where the contact opens the estimate with its code.
func (s *Service) OnlineURL(g Signing) string {
	return s.baseURL + "/online/estimates/" + g.EstimateID.String() + "?code=" + g.ViewCode
}

// signingFor prepares the signing of an estimate being sent: a fresh code
// when it is to be signed online, and the sender's signature when they
// signed. It is not stored yet.
func (s *Service) signingFor(ctx context.Context, e Estimate, user resource.ID, in EstimateSendInput) (Signing, bool, error) {
	g, _, err := s.store.EstimateSigning(ctx, e.AdministrationID, e.ID)
	if err != nil {
		return Signing{}, false, wrap("load estimate signing", err)
	}
	if !e.SignOnline || g.Signed() {
		return g, false, nil
	}
	if in.Sender && !isPNG(in.SenderSignature) {
		return Signing{}, false, invalid("signature_output", "must be a drawn signature")
	}
	code, err := newViewCode()
	if err != nil {
		return Signing{}, false, err
	}
	now := s.now()
	expires := now.Add(viewCodeLife)
	g.AdministrationID, g.EstimateID, g.ViewCode, g.ViewCodeExpiresAt, g.SentByUserID, g.UpdatedAt = e.AdministrationID, e.ID, code, &expires, &user, now
	if in.Sender {
		g.SenderSignature, g.SenderSignedAt, g.SenderUserID = in.SenderSignature, &now, &user
	}
	return g, true, nil
}

// OnlineEstimate is what the contact sees of an estimate online: its
// document, as its PDF shows it.
type OnlineEstimate struct {
	Estimate Estimate
	Document Document
	// Signable tells whether the contact can still sign it.
	Signable bool
}

// Sender is who the estimate comes from.
func (o OnlineEstimate) Sender() administration.Identity { return o.Document.Identity }

// ContactName is who the estimate is for.
func (o OnlineEstimate) ContactName() string { return o.Document.Contact.Name() }

// openOnline finds the estimate the code opens, with its signing and the
// user the contact acts as. Any problem reads as not found, so codes
// cannot be probed.
func (s *Service) openOnline(ctx context.Context, id resource.ID, code string) (Estimate, Signing, resource.ID, error) {
	g, ok, err := s.store.EstimateSigning(ctx, 0, id)
	if err != nil {
		return Estimate{}, Signing{}, 0, wrap("load estimate signing", err)
	}
	if !ok || !g.opens(code, s.now()) || g.SentByUserID == nil {
		return Estimate{}, Signing{}, 0, ErrEstimateNotFound
	}
	e, err := s.Estimate(ctx, g.AdministrationID, id, *g.SentByUserID)
	if err != nil {
		return Estimate{}, Signing{}, 0, ErrEstimateNotFound
	}
	return e, g, *g.SentByUserID, nil
}

// signable reports whether the contact can sign the estimate: once, while
// it is open or late, as Moneybird lets a quote past its due date be signed.
func signable(e Estimate, g Signing) bool {
	return !g.Signed() && slices.Contains([]string{Open, Late}, e.State)
}

// OnlineEstimate returns the estimate the code opens, for the contact.
func (s *Service) OnlineEstimate(ctx context.Context, id resource.ID, code string) (OnlineEstimate, error) {
	e, g, user, err := s.openOnline(ctx, id, code)
	if err != nil {
		return OnlineEstimate{}, err
	}
	a, err := s.admins.Get(ctx, e.AdministrationID, user)
	if err != nil {
		return OnlineEstimate{}, err
	}
	doc, err := s.estimateDocument(ctx, a, user, e)
	if err != nil {
		return OnlineEstimate{}, err
	}
	return OnlineEstimate{Estimate: e, Document: doc, Signable: signable(e, g)}, nil
}

// OnlineEstimatePDF is the estimate's PDF for the contact: signed once it is.
func (s *Service) OnlineEstimatePDF(ctx context.Context, id resource.ID, code string) ([]byte, string, error) {
	e, _, user, err := s.openOnline(ctx, id, code)
	if err != nil {
		return nil, "", err
	}
	if e.Signing != nil && e.Signing.Signed() {
		return s.SignedEstimatePDF(ctx, e.AdministrationID, id, user)
	}
	return s.EstimatePDF(ctx, e.AdministrationID, id, user, false)
}

// SignInput is the contact's online acceptance: their name and email
// address, the signature drawn, its strokes, and where it came from.
type SignInput struct {
	Name, Email string
	Signature   []byte
	AuditTrail  string
	IP          string
	UserAgent   string
}

// SignEstimateOnline accepts the estimate with the contact's signature: it
// becomes accepted, and the contact and the sender both get the signed
// quote. As on Moneybird, it cannot be rejected online.
func (s *Service) SignEstimateOnline(ctx context.Context, id resource.ID, code string, in SignInput) (Estimate, error) {
	e, g, user, err := s.openOnline(ctx, id, code)
	if err != nil {
		return Estimate{}, err
	}
	var verr resource.ValidationError
	name, email := strings.TrimSpace(in.Name), strings.TrimSpace(in.Email)
	if name == "" || len(name) > 255 {
		verr.Add("name", "can't be blank")
	}
	if _, err := mail.ParseAddress(email); err != nil || len(email) > 255 {
		verr.Add("email", "is not a valid email address")
	}
	if !isPNG(in.Signature) {
		verr.Add("signature_output", "must be a drawn signature")
	}
	if !signable(e, g) {
		verr.Add("base", "The quote can no longer be accepted online")
	}
	if err := verr.OrNil(); err != nil {
		return Estimate{}, err
	}
	a, err := s.admins.Get(ctx, e.AdministrationID, user)
	if err != nil {
		return Estimate{}, err
	}
	now, today := s.now(), s.today(a)
	stored, err := s.store.Estimate(ctx, e.AdministrationID, id)
	if err != nil {
		return Estimate{}, err
	}
	stored.State, stored.AcceptedAt, stored.RejectedAt, stored.UpdatedAt = Accepted, &today, nil, now
	g.SignerName, g.SignerEmail, g.SignerIP, g.SignerUserAgent = name, email, truncate(in.IP, 100), truncate(in.UserAgent, 500)
	g.Signature, g.AuditTrail, g.SignedAt, g.UpdatedAt = in.Signature, truncate(in.AuditTrail, 100_000), &now, now
	if err := wrap("store estimate signing", s.store.SaveSigning(ctx, g)); err != nil {
		return Estimate{}, err
	}
	if err := wrap("store estimate", s.store.SaveEstimate(ctx, stored, false)); err != nil {
		return Estimate{}, err
	}
	signed := on(stored.invoice(), "estimate_signed_online").with(map[string]any{"name": name, "email": email})
	if err := s.record(ctx, user, signed); err != nil {
		return Estimate{}, err
	}
	if err := s.mailSigned(ctx, a, user, stored.ID, email); err != nil {
		return Estimate{}, fmt.Errorf("%w: %v", ErrEstimateDelivery, err)
	}
	return s.Estimate(ctx, e.AdministrationID, id, user)
}

// mailSigned sends the signed quote to the contact who signed and to the
// sender's address.
func (s *Service) mailSigned(ctx context.Context, a administration.Listed, user, id resource.ID, signer string) error {
	e, err := s.Estimate(ctx, a.ID, id, user)
	if err != nil {
		return err
	}
	doc, err := s.signedDocument(ctx, a, user, e)
	if err != nil {
		return err
	}
	pdf, err := s.renderer.PDF(doc)
	if err != nil {
		return wrap("render signed estimate", err)
	}
	subject, body := signedEmailText(doc)
	files := []File{{Name: estimateFileName(e, "pdf"), ContentType: "application/pdf", Data: pdf}}
	var errs []error
	for _, to := range []string{signer, doc.Identity.Email} {
		if strings.TrimSpace(to) == "" {
			continue
		}
		errs = append(errs, s.mailer.SendDocument(ctx, Mail{
			To: []string{to}, ReplyTo: doc.Identity.Email, FromName: doc.Identity.CompanyName, Subject: subject, Body: body, Files: files,
			Look: s.emailLook(ctx, a.ID, user, doc, EstimateDetails(doc)),
		}))
	}
	return errors.Join(errs...)
}

// signedEmailText is the email that carries the signed quote.
func signedEmailText(doc Document) (string, string) {
	if strings.HasPrefix(doc.Invoice.Language, "nl") {
		return ExpandEstimate("Offerte {estimate_id} is geaccepteerd", doc),
			ExpandEstimate("Offerte {estimate_id} van {company_name} is online geaccepteerd en ondertekend. In de bijlage vind je de ondertekende offerte.", doc)
	}
	return ExpandEstimate("Quote {estimate_id} has been accepted", doc),
		ExpandEstimate("Quote {estimate_id} from {company_name} has been accepted and signed online. The signed quote is attached.", doc)
}

// signedDocument is the estimate's document with its signatures.
func (s *Service) signedDocument(ctx context.Context, a administration.Listed, user resource.ID, e Estimate) (Document, error) {
	if e.Signing == nil || !e.Signing.Signed() {
		return Document{}, ErrEstimateNotSigned
	}
	doc, err := s.estimateDocument(ctx, a, user, e)
	if err != nil {
		return Document{}, err
	}
	g := *e.Signing
	doc.Signing = &g
	return doc, nil
}

// onlineMessage is the line that brings the contact to the online quote.
func onlineMessage(language, link string) (text, button string) {
	if strings.HasPrefix(language, "nl") {
		return "Bekijk en accepteer de offerte online: " + link, "Offerte bekijken"
	}
	return "View and accept the quote online: " + link, "View quote"
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
