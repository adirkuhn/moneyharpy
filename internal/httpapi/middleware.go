package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/ratelimit"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	validator "github.com/oapi-codegen/nethttp-middleware"
)

const (
	sessionCookie = "moneyharpy_session"
	maxBodyBytes  = 1 << 20
	// maxUploadBytes fits the largest document style file, a 10 MB background.
	maxUploadBytes = 10<<20 + 1<<10

	contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; font-src 'self' data:; connect-src 'self'; worker-src 'self' blob:; " +
		"frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
)

type contextKey int

const (
	userKey contextKey = iota
	sessionKey
)

func userFrom(ctx context.Context) (identity.User, bool) {
	u, ok := ctx.Value(userKey).(identity.User)
	return u, ok
}

func (s *server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.fail(w, r, fmt.Errorf("panic: %v", v))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// negotiateFormat makes the Moneybird-style .json suffix optional and refuses
// the XML format that MoneyHarpy does not offer.
func (s *server) negotiateFormat(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v2/") {
			if strings.HasSuffix(r.URL.Path, ".xml") {
				writeError(w, http.StatusNotAcceptable, "Only JSON is supported")
				return
			}
			r.URL.Path = strings.TrimSuffix(r.URL.Path, ".json")
			r.URL.RawPath = ""
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) throttle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var limiter *ratelimit.Limiter
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v2/"):
			limiter = s.deps.APILimiter
		case r.URL.Path == "/api/auth/login", r.URL.Path == "/api/auth/register",
			r.URL.Path == "/api/auth/password_reset/request", r.URL.Path == "/api/auth/password_reset/confirm":
			limiter = s.deps.AuthLimiter
		case r.URL.Path == "/api/onboarding/company_search", r.URL.Path == "/api/onboarding/address_lookup":
			limiter = s.deps.LookupLimiter
		default:
			next.ServeHTTP(w, r)
			return
		}

		d := limiter.Allow(clientIP(r, s.deps.TrustProxyHeaders))
		reset := strconv.Itoa(int(math.Ceil(d.Reset.Seconds())))
		w.Header().Set("RateLimit-Limit", strconv.Itoa(d.Limit))
		w.Header().Set("RateLimit-Remaining", strconv.Itoa(d.Remaining))
		w.Header().Set("RateLimit-Reset", reset)
		if !d.Allowed {
			w.Header().Set("Retry-After", reset)
			writeJSON(w, http.StatusTooManyRequests, "Retry later")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP identifies the caller for throttling. Behind a trusted proxy it uses
// the last X-Forwarded-For entry, the only one the proxy itself wrote: earlier
// entries are supplied by the client and could be forged to dodge limits.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		forwarded := r.Header.Get("X-Forwarded-For")
		last := strings.TrimSpace(forwarded[strings.LastIndexByte(forwarded, ',')+1:])
		if net.ParseIP(last) != nil {
			return last
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// crossOriginProtection stops other sites from making state-changing requests
// with a signed-in user's cookie. Non-browser API clients are unaffected.
func (s *server) crossOriginProtection() func(http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	if err := cop.AddTrustedOrigin(s.deps.BaseURL); err != nil {
		s.deps.Logger.Warn("base URL is not usable as a trusted origin", "error", err)
	}
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusForbidden, "Cross-origin request rejected")
	}))
	return cop.Handler
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := int64(maxBodyBytes)
		if isFileUpload(r.Method, r.URL.Path) {
			limit = maxUploadBytes
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

// authenticate identifies the caller if it presents valid credentials. Whether
// an operation requires them is decided by the contract, in requireUser.
func (s *server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var (
			user identity.User
			err  = identity.ErrUnauthenticated
		)
		if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			user, err = s.deps.Identity.AuthenticateToken(ctx, bearer)
		} else if c, cerr := r.Cookie(sessionCookie); cerr == nil {
			if user, err = s.deps.Identity.Authenticate(ctx, c.Value); err == nil {
				ctx = context.WithValue(ctx, sessionKey, c.Value)
			}
		}
		switch {
		case err == nil:
			ctx = context.WithValue(ctx, userKey, user)
		case !errors.Is(err, identity.ErrUnauthenticated):
			s.fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requireUser(ctx context.Context, in *openapi3filter.AuthenticationInput) error {
	if _, ok := userFrom(in.RequestValidationInput.Request.Context()); !ok {
		return identity.ErrUnauthenticated
	}
	return nil
}

// contractError answers requests the contract refuses. Messages carry the
// failed rule but never the submitted value, which may be a password.
func (s *server) contractError(ctx context.Context, err error, w http.ResponseWriter, r *http.Request, opts validator.ErrorHandlerOpts) {
	switch opts.StatusCode {
	case http.StatusUnauthorized:
		writeError(w, http.StatusUnauthorized, "Authorization required")
	case http.StatusNotFound:
		writeError(w, http.StatusNotFound, "Not found")
	case http.StatusMethodNotAllowed:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	case http.StatusBadRequest:
		writeError(w, http.StatusBadRequest, describeRequestError(err))
	default:
		s.fail(w, r, err)
	}
}

func describeRequestError(err error) string {
	var schemaErr *openapi3.SchemaError
	if errors.As(err, &schemaErr) {
		if path := strings.Join(schemaErr.JSONPointer(), "."); path != "" {
			return path + ": " + schemaErr.Reason
		}
		return schemaErr.Reason
	}
	var reqErr *openapi3filter.RequestError
	if errors.As(err, &reqErr) && reqErr.Parameter != nil {
		return "invalid parameter " + reqErr.Parameter.Name
	}
	return "invalid request"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// The client went away if this fails; there is nobody left to tell.
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, Error{Error: message})
}

func validationBody(fields map[string][]string) ValidationError {
	return ValidationError{Error: fields}
}

func (s *server) setSession(w http.ResponseWriter, token string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

// isFileUpload reports requests that carry a file or a whole bank
// statement, which may be larger than other bodies.
func isFileUpload(method, path string) bool {
	path = strings.TrimSuffix(path, ".json")
	switch method {
	case http.MethodPut:
		return strings.Contains(path, "/document_styles/") && (strings.HasSuffix(path, "/logo") || strings.HasSuffix(path, "/background"))
	case http.MethodPost:
		return strings.Contains(path, "/financial_accounts/") && strings.HasSuffix(path, "/import") || strings.HasSuffix(path, "/financial_statements")
	case http.MethodPatch:
		return strings.Contains(path, "/financial_statements/")
	}
	return false
}
