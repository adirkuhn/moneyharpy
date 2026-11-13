// Package httpapi exposes the application over HTTP. Its contract lives in
// api/openapi.yaml: requests are validated against it before reaching a
// handler, and operations are authenticated according to its security rules.
package httpapi

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../api/openapi.yaml

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/adirkuhn/moneyharpy/api"
	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/assets"
	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/connections"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/downloads"
	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/membership"
	"github.com/adirkuhn/moneyharpy/internal/ocr"
	"github.com/adirkuhn/moneyharpy/internal/overview"
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/projects"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/ratelimit"
	"github.com/adirkuhn/moneyharpy/internal/reports"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/adirkuhn/moneyharpy/internal/subscriptions"
	"github.com/adirkuhn/moneyharpy/internal/tasklists"
	"github.com/adirkuhn/moneyharpy/internal/timeentries"
	"github.com/adirkuhn/moneyharpy/internal/webhooks"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	validator "github.com/oapi-codegen/nethttp-middleware"
)

// ResourceLocator finds where a resource lives.
type ResourceLocator interface {
	Locate(ctx context.Context, id, user resource.ID) (resource.Ref, error)
}

// GoogleClient signs users in through Google.
type GoogleClient interface {
	AuthURL(ctx context.Context, state, nonce string) (string, error)
	Exchange(ctx context.Context, code, nonce string) (identity.External, error)
}

// LM2Client signs users in through an Akantio LM2 installation.
type LM2Client interface {
	LoginURL(ctx context.Context, state string) (string, error)
	Identify(ctx context.Context, token string) (identity.External, error)
}

// CompanySearcher finds companies in the Dutch company register.
type CompanySearcher interface {
	SearchCompanies(ctx context.Context, query string) ([]administration.CompanyMatch, error)
}

// RegisterChecker checks the experimental KVK website search.
type RegisterChecker interface {
	Check(ctx context.Context) error
}

// AddressLookup completes Dutch addresses.
type AddressLookup interface {
	LookupAddress(ctx context.Context, zipcode, houseNumber string) (administration.AddressMatch, error)
}

// StorageChecker checks that the file storage settings work.
type StorageChecker interface {
	Check(ctx context.Context) error
}

// TestMailer checks that the mail settings work.
type TestMailer interface {
	SendTest(ctx context.Context, to string) error
}

// Deps are the collaborators of the HTTP layer.
type Deps struct {
	Administrations *administration.Service
	Invoicing       *invoicing.Service
	Bookkeeping     *bookkeeping.Service
	Banking         *banking.Service
	Overview        *overview.Service
	Contacts        *contacts.Service
	Journal         *journal.Service
	Sales           *sales.Service
	Purchases       *purchases.Service
	Assets          *assets.Service
	Products        *products.Service
	Projects        *projects.Service
	Subscriptions   *subscriptions.Service
	Downloads       *downloads.Service
	Reports         *reports.Service
	TimeEntries     *timeentries.Service
	TaskLists       *tasklists.Service
	Membership      *membership.Service
	Connections     *connections.Service
	Webhooks        *webhooks.Service
	Storage         StorageChecker
	Identity        *identity.Service
	Settings        *settings.Service
	Resources       ResourceLocator
	Google          GoogleClient
	LM2             LM2Client
	Companies       CompanySearcher
	CompanyCheck    RegisterChecker
	Addresses       AddressLookup
	Mail            TestMailer
	// OCR reads and renders attachments; nil works as an unavailable
	// service.
	OCR ocr.Service
	// BaseURL is the public origin of this installation.
	BaseURL string
	Logger  *slog.Logger
	// Web holds the built web application.
	Web fs.FS
	// APILimiter throttles /api/v2, AuthLimiter throttles password endpoints.
	APILimiter  *ratelimit.Limiter
	AuthLimiter *ratelimit.Limiter
	// LookupLimiter throttles calls that reach third-party registers.
	LookupLimiter *ratelimit.Limiter
	// ResetLimiter caps reset mails per address, so nobody's inbox can be
	// flooded.
	ResetLimiter *ratelimit.Limiter
	// FileKey signs the temporary download links; a random key is used
	// when it is empty, which works with one instance only.
	FileKey []byte
	// TrustProxyHeaders takes the client address from X-Forwarded-For. Enable
	// it only behind exactly one reverse proxy that appends to the header.
	TrustProxyHeaders bool
}

// uploadTypes are the binary request and response bodies in the contract.
var uploadTypes = []string{"image/png", "image/jpeg", "image/gif", "application/pdf", "text/xml"}

func init() {
	for _, t := range uploadTypes {
		openapi3filter.RegisterBodyDecoder(t, openapi3filter.FileBodyDecoder)
	}
}

// NewHandler returns the handler for the whole application.
func NewHandler(d Deps) (http.Handler, error) {
	spec, err := openapi3.NewLoader().LoadFromData(api.Spec)
	if err != nil {
		return nil, fmt.Errorf("load contract: %w", err)
	}
	if err := spec.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("validate contract: %w", err)
	}

	s := &server{deps: d, secureCookies: strings.HasPrefix(d.BaseURL, "https://"), fileKey: fileKey(d.FileKey)}
	rules, err := permissionRules(spec)
	if err != nil {
		return nil, err
	}
	authorize, err := s.authorize(spec, rules)
	if err != nil {
		return nil, err
	}
	s.tools = buildTools(spec, rules)

	r := chi.NewRouter()
	r.Use(middleware.RequestID, s.recoverPanics, securityHeaders)
	r.Use(s.negotiateFormat, s.throttle, s.crossOriginProtection(), limitBody, s.authenticate, s.limitGrants)
	r.Use(validator.OapiRequestValidatorWithOptions(spec, &validator.Options{
		SilenceServersWarning: true,
		Skipper:               func(r *http.Request) bool { return !inContract(r.URL.Path) },
		ErrorHandlerWithOpts:  s.contractError,
		Options: openapi3filter.Options{
			AuthenticationFunc: requireUser,
		},
	}), authorize, s.recordEvents)

	// Any method, so the health check can also stand in as a test webhook receiver.
	r.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/api/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		// The client went away if this fails; there is nobody left to tell.
		_, _ = w.Write(api.Spec)
	})
	r.NotFound(s.serveWeb)
	HandlerFromMux(s, r)
	s.root = r
	return r, nil
}

func inContract(path string) bool {
	return strings.HasPrefix(path, "/api/") && path != "/api/openapi.yaml"
}

// server implements the generated ServerInterface.
type server struct {
	deps          Deps
	secureCookies bool
	// root is the whole handler, for running MCP tools as API requests.
	root  http.Handler
	tools []operationTool
	// fileKey signs download links.
	fileKey []byte
}

func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var verr *resource.ValidationError
	switch {
	case errors.As(err, &verr):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(verr.Fields))
	case errors.Is(err, identity.ErrEmailTaken):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"email": {"has already been taken"}}))
	case errors.Is(err, invoicing.ErrDefaultInUse):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"The default cannot be deleted or deactivated"}}))
	case errors.Is(err, bookkeeping.ErrSystemAccount):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"System ledger accounts cannot be deleted, deactivated or change type"}}))
	case errors.Is(err, bookkeeping.ErrHasChildren):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"Remove or move the sub-categories first"}}))
	case errors.Is(err, bookkeeping.ErrInUse):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"This category is used by transactions or booking rules; keep it deactivated instead"}}))
	case errors.Is(err, banking.ErrBookingUnavailable):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"booking_type": {"is not available yet in MoneyHarpy"}}))
	case errors.Is(err, purchases.ErrNotEditable):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"This document cannot be changed in its state; remove its payments first"}}))
	case errors.Is(err, sales.ErrNotEditable):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"This invoice cannot be changed in its state; credit it instead"}}))
	case errors.Is(err, sales.ErrDeliveryUnavailable):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"delivery_method": {"is not available yet; send the invoice by email or yourself"}}))
	case errors.Is(err, sales.ErrDelivery):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"The invoice is booked but could not be emailed; check the mail settings and send it again"}}))
	case errors.Is(err, banking.ErrStatementBooked):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"Remove the bookings of this statement's transactions first"}}))
	case errors.Is(err, connections.ErrForbidden):
		writeError(w, http.StatusForbidden, "Only owners can create connections here, unless they let users connect")
	case errors.Is(err, webhooks.ErrForbidden):
		writeError(w, http.StatusForbidden, "Your user has no permission for this")
	case errors.Is(err, errNeedsTwoFactor):
		writeError(w, http.StatusForbidden, "This administration requires two-step verification. Turn it on for your account first.")
	case errors.Is(err, errNoPermission), errors.Is(err, membership.ErrForbidden):
		writeError(w, http.StatusForbidden, "Your user has no permission for this")
	case errors.Is(err, membership.ErrLastOwner):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"The administration needs an owner; make someone else an owner first"}}))
	case errors.Is(err, membership.ErrAlreadyMember):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"You are a user of this administration already"}}))
	case errors.Is(err, membership.ErrInvitationUsed):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"This invitation was accepted or has expired; ask for a new one"}}))
	case errors.Is(err, membership.ErrTwoFactorNeeded):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"Turn on two-step verification for your own account first"}}))
	case errors.Is(err, identity.ErrInvalidCode):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"code": {"is not valid"}}))
	case errors.Is(err, identity.ErrChallengeInvalid):
		writeError(w, http.StatusUnauthorized, "The sign-in expired; sign in again")
	case errors.Is(err, identity.ErrTwoFactorOn):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"Two-step verification is on already"}}))
	case errors.Is(err, identity.ErrTwoFactorOff):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"Two-step verification is off; set it up first"}}))
	case errors.Is(err, identity.ErrTwoFactorRequired):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"An administration you work in requires two-step verification"}}))
	case errors.Is(err, administration.ErrDefaultIdentity):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"The default identity cannot be deleted"}}))
	case errors.Is(err, identity.ErrResetTokenInvalid):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"token": {"is invalid or has expired"}}))
	case errors.Is(err, identity.ErrRegistrationClosed):
		writeError(w, http.StatusForbidden, "Registration is closed")
	case errors.Is(err, identity.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "Invalid email or password")
	case errors.Is(err, administration.ErrNotFound),
		errors.Is(err, administration.ErrIdentityNotFound),
		errors.Is(err, administration.ErrCustomFieldNotFound),
		errors.Is(err, invoicing.ErrWorkflowNotFound),
		errors.Is(err, invoicing.ErrDocumentStyleNotFound),
		errors.Is(err, invoicing.ErrFileNotFound),
		errors.Is(err, resource.ErrNotFound),
		bookkeeping.IsNotFound(err),
		banking.IsNotFound(err),
		contacts.IsNotFound(err),
		journal.IsNotFound(err),
		sales.IsNotFound(err),
		purchases.IsNotFound(err),
		assets.IsNotFound(err),
		errors.Is(err, purchases.ErrNoSEPAFile),
		errors.Is(err, products.ErrNotFound),
		errors.Is(err, projects.ErrNotFound),
		errors.Is(err, subscriptions.ErrNotFound),
		errors.Is(err, downloads.ErrNotFound),
		errors.Is(err, membership.ErrUserNotFound),
		errors.Is(err, connections.ErrNotFound),
		errors.Is(err, webhooks.ErrNotFound),
		errors.Is(err, membership.ErrInvitationNotFound),
		errors.Is(err, administration.ErrLookupUnavailable),
		errors.Is(err, administration.ErrAddressUnknown),
		errors.Is(err, identity.ErrTokenNotFound):
		writeError(w, http.StatusNotFound, "Not found")
	case errors.Is(err, administration.ErrRegisterUnavailable):
		s.registerProblem(w, r, err, "kvk_unavailable", "KVK search is unavailable right now")
	case errors.Is(err, administration.ErrRegisterBlocked):
		s.registerProblem(w, r, err, "kvk_blocked", "KVK blocked the search request")
	case errors.Is(err, administration.ErrRegisterBusy):
		w.Header().Set("Retry-After", "60")
		s.registerProblem(w, r, err, "kvk_rate_limited", "Too many KVK searches, retry in a minute")
	default:
		s.deps.Logger.ErrorContext(r.Context(), "request failed", "path", r.URL.Path, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal server error")
	}
}

// registerProblem answers a company register failure with a code clients can
// explain, and logs the detail for the administrator.
func (s *server) registerProblem(w http.ResponseWriter, r *http.Request, err error, code, message string) {
	s.deps.Logger.WarnContext(r.Context(), "company register search failed", "path", r.URL.Path, "error", err)
	writeJSON(w, http.StatusServiceUnavailable, Error{Error: message, Code: &code})
}
