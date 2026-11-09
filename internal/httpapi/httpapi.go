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
	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/ratelimit"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/settings"
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

// Deps are the collaborators of the HTTP layer.
type Deps struct {
	Administrations *administration.Service
	Identity        *identity.Service
	Settings        *settings.Service
	Resources       ResourceLocator
	Google          GoogleClient
	// BaseURL is the public origin of this installation.
	BaseURL string
	Logger  *slog.Logger
	// Web holds the built web application.
	Web fs.FS
	// APILimiter throttles /api/v2, AuthLimiter throttles password endpoints.
	APILimiter  *ratelimit.Limiter
	AuthLimiter *ratelimit.Limiter
	// TrustProxyHeaders takes the client address from X-Forwarded-For. Enable
	// it only behind exactly one reverse proxy that appends to the header.
	TrustProxyHeaders bool
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

	s := &server{deps: d, secureCookies: strings.HasPrefix(d.BaseURL, "https://")}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, s.recoverPanics, securityHeaders)
	r.Use(s.negotiateFormat, s.throttle, s.crossOriginProtection(), limitBody, s.authenticate)
	r.Use(validator.OapiRequestValidatorWithOptions(spec, &validator.Options{
		SilenceServersWarning: true,
		Skipper:               func(r *http.Request) bool { return !inContract(r.URL.Path) },
		ErrorHandlerWithOpts:  s.contractError,
		Options: openapi3filter.Options{
			AuthenticationFunc: requireUser,
		},
	}))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/api/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		// The client went away if this fails; there is nobody left to tell.
		_, _ = w.Write(api.Spec)
	})
	r.NotFound(s.serveWeb)
	HandlerFromMux(s, r)
	return r, nil
}

func inContract(path string) bool {
	return strings.HasPrefix(path, "/api/") && path != "/api/openapi.yaml"
}

// server implements the generated ServerInterface.
type server struct {
	deps          Deps
	secureCookies bool
}

func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var verr *resource.ValidationError
	switch {
	case errors.As(err, &verr):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(verr.Fields))
	case errors.Is(err, identity.ErrEmailTaken):
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"email": {"has already been taken"}}))
	case errors.Is(err, identity.ErrRegistrationClosed):
		writeError(w, http.StatusForbidden, "Registration is closed")
	case errors.Is(err, identity.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "Invalid email or password")
	case errors.Is(err, administration.ErrNotFound),
		errors.Is(err, resource.ErrNotFound),
		errors.Is(err, identity.ErrTokenNotFound):
		writeError(w, http.StatusNotFound, "Not found")
	default:
		s.deps.Logger.ErrorContext(r.Context(), "request failed", "path", r.URL.Path, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal server error")
	}
}
