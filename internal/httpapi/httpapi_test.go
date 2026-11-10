package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/adirkuhn/moneyharpy/api"
	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/httpapi"
	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
	"github.com/adirkuhn/moneyharpy/internal/ratelimit"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgxpool"
)

const password = "correct horse battery"

var (
	sharedPool *pgxpool.Pool
	spec       *openapi3.T
	specRouter routers.Router
	emailSeq   atomic.Int64
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		panic(err)
	}
	sharedPool = pool

	loader := openapi3.NewLoader()
	if spec, err = loader.LoadFromData(api.Spec); err != nil {
		panic(err)
	}
	if err := spec.Validate(ctx); err != nil {
		panic(err)
	}
	if specRouter, err = gorillamux.NewRouter(spec); err != nil {
		panic(err)
	}

	code := m.Run()
	cleanup()
	os.Exit(code)
}

type fakeGoogle struct {
	external identity.External
	err      error
	gotNonce string
}

func (g *fakeGoogle) AuthURL(_ context.Context, state, nonce string) (string, error) {
	return "https://accounts.example/auth?state=" + state + "&nonce=" + nonce, nil
}

func (g *fakeGoogle) Exchange(_ context.Context, _, nonce string) (identity.External, error) {
	g.gotNonce = nonce
	return g.external, g.err
}

type fakeLM2 struct {
	external identity.External
	err      error
	gotToken string
}

func (l *fakeLM2) LoginURL(_ context.Context, state string) (string, error) {
	return "https://lm.example/app-login?state=" + state, nil
}

func (l *fakeLM2) Identify(_ context.Context, token string) (identity.External, error) {
	l.gotToken = token
	return l.external, l.err
}

type options struct {
	pool       *pgxpool.Pool
	apiLimiter *ratelimit.Limiter
}

type harness struct {
	t      *testing.T
	server *httptest.Server
	google *fakeGoogle
	lm2    *fakeLM2
	settle *settings.Service
}

func newHarness(t *testing.T, opts options) *harness {
	t.Helper()
	pool := opts.pool
	if pool == nil {
		pool = sharedPool
	}
	if opts.apiLimiter == nil {
		opts.apiLimiter = ratelimit.New(100000, time.Minute)
	}
	ids := resource.NewGenerator(emailSeq.Add(1) % 1024)
	settle := settings.NewService(postgres.NewSettingsStore(pool), settings.NewCipher("test-secret"))
	google := &fakeGoogle{}

	lm2 := &fakeLM2{}

	h := &harness{t: t, google: google, lm2: lm2, settle: settle}
	h.server = httptest.NewUnstartedServer(nil)
	h.server.Start()
	t.Cleanup(h.server.Close)

	handler, err := httpapi.NewHandler(httpapi.Deps{
		Administrations: administration.NewService(postgres.NewAdministrationStore(pool), ids, time.Now),
		Identity:        identity.NewService(postgres.NewIdentityStore(pool), settle, ids, identity.NewHasher(identity.Params{Time: 1, MemoryKiB: 8, Threads: 1}), time.Now),
		Settings:        settle,
		Resources:       postgres.NewResourceLocator(pool),
		Google:          google,
		LM2:             lm2,
		BaseURL:         h.server.URL,
		Logger:          slog.New(slog.DiscardHandler),
		Web:             fstest.MapFS{"index.html": {Data: []byte("<html>spa</html>")}, "assets/app.js": {Data: []byte("js")}},
		APILimiter:      opts.apiLimiter,
		AuthLimiter:     ratelimit.New(100000, time.Minute),
	})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	h.server.Config.Handler = handler
	return h
}

// client is a browser-like client whose every API response is checked against
// the OpenAPI contract.
type client struct {
	t     *testing.T
	base  string
	http  *http.Client
	token string
}

func (h *harness) client() *client {
	h.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		h.t.Fatalf("cookiejar.New() error = %v", err)
	}
	return &client{t: h.t, base: h.server.URL, http: &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func (c *client) do(method, path string, in any, headers ...[2]string) response {
	c.t.Helper()
	var body io.Reader
	if raw, ok := in.(string); ok {
		body = strings.NewReader(raw)
	} else if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			c.t.Fatalf("Marshal() error = %v", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		c.t.Fatalf("NewRequest() error = %v", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for _, h := range headers {
		req.Header.Set(h[0], h[1])
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s error = %v", method, path, err)
	}
	raw, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		c.t.Fatalf("read body: %v", err)
	}
	c.checkContract(req, res, raw)
	return response{status: res.StatusCode, header: res.Header, body: raw}
}

func (c *client) checkContract(req *http.Request, res *http.Response, body []byte) {
	c.t.Helper()
	if !strings.HasPrefix(req.URL.Path, "/api/") || req.URL.Path == "/api/openapi.yaml" {
		return
	}
	lookup := req.Clone(context.Background())
	lookup.URL.Path = strings.TrimSuffix(lookup.URL.Path, ".json")
	route, params, err := specRouter.FindRoute(lookup)
	if err != nil {
		return // Paths outside the contract are asserted by their own tests.
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: lookup, PathParams: params, Route: route},
		Status:                 res.StatusCode,
		Header:                 res.Header,
		Options:                &openapi3filter.Options{IncludeResponseStatus: true},
	}
	input.SetBodyBytes(body)
	if err := openapi3filter.ValidateResponse(context.Background(), input); err != nil {
		c.t.Errorf("%s %s violates contract: %v\nbody: %s", req.Method, req.URL.Path, err, body)
	}
}

func decode[T any](t *testing.T, r response) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(r.body, &v); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", r.body, err)
	}
	return v
}

func wantStatus(t *testing.T, r response, want int) {
	t.Helper()
	if r.status != want {
		t.Fatalf("status = %d, want %d; body: %s", r.status, want, r.body)
	}
}

func uniqueEmail() string {
	return "user" + resource.NewGenerator(emailSeq.Add(1)%1024).Next().String() + "@example.com"
}

func (c *client) register() httpapi.CurrentUser {
	c.t.Helper()
	r := c.do(http.MethodPost, "/api/auth/register", map[string]string{"email": uniqueEmail(), "name": "Ann", "password": password})
	wantStatus(c.t, r, http.StatusCreated)
	return decode[httpapi.CurrentUser](c.t, r)
}

func (c *client) createAdministration(name string) httpapi.Administration {
	c.t.Helper()
	r := c.do(http.MethodPost, "/api/v2/administrations", map[string]any{"administration": map[string]string{"name": name}})
	wantStatus(c.t, r, http.StatusCreated)
	return decode[httpapi.Administration](c.t, r)
}

func TestAdministrationsRequireAuthentication(t *testing.T) {
	r := newHarness(t, options{}).client().do(http.MethodGet, "/api/v2/administrations", nil)
	wantStatus(t, r, http.StatusUnauthorized)
}

func TestListAdministrationsMoneybirdShape(t *testing.T) {
	h := newHarness(t, options{})
	c := h.client()
	c.register()
	created := c.createAdministration("Harpy B.V.")

	for _, path := range []string{"/api/v2/administrations", "/api/v2/administrations.json"} {
		t.Run(path, func(t *testing.T) {
			r := c.do(http.MethodGet, path, nil)
			wantStatus(t, r, http.StatusOK)
			got := decode[[]map[string]any](t, r)
			if len(got) != 1 {
				t.Fatalf("GET %s returned %d rows, want 1", path, len(got))
			}
			want := map[string]any{
				"id": created.Id, "name": "Harpy B.V.", "language": "en", "currency": "EUR", "country": "NL",
				"time_zone": "Europe/Amsterdam", "access": "user", "suspended": false,
				"period_locked_until": nil, "period_start_date": time.Now().Format("2006") + "-01-01",
				"favorite": false, "uri": "/" + created.Id,
			}
			if diff := cmp.Diff(want, got[0]); diff != "" {
				t.Errorf("GET %s diff (-want +got):\n%s", path, diff)
			}
		})
	}
}

func TestXMLFormatIsNotAcceptable(t *testing.T) {
	c := newHarness(t, options{}).client()
	c.register()
	r := c.do(http.MethodGet, "/api/v2/administrations.xml", nil)
	wantStatus(t, r, http.StatusNotAcceptable)
}

func TestBearerTokenAuthenticatesAPI(t *testing.T) {
	h := newHarness(t, options{})
	browser := h.client()
	browser.register()
	browser.createAdministration("Via token")

	r := browser.do(http.MethodPost, "/api/user/tokens", map[string]string{"name": "ci"})
	wantStatus(t, r, http.StatusCreated)
	created := decode[httpapi.CreatedApiToken](t, r)

	api := h.client()
	api.token = created.Token
	r = api.do(http.MethodGet, "/api/v2/administrations.json", nil)
	wantStatus(t, r, http.StatusOK)
	if got := decode[[]httpapi.Administration](t, r); len(got) != 1 || got[0].Name != "Via token" {
		t.Errorf("GET administrations = %+v, want the token owner's administration", got)
	}

	listed := decode[[]map[string]any](t, browser.do(http.MethodGet, "/api/user/tokens", nil))
	if _, leaked := listed[0]["token"]; leaked {
		t.Error("token list contains the secret")
	}

	wantStatus(t, browser.do(http.MethodDelete, "/api/user/tokens/"+created.Id, nil), http.StatusNoContent)
	wantStatus(t, api.do(http.MethodGet, "/api/v2/administrations.json", nil), http.StatusUnauthorized)
}

func TestContractRejectsMalformedRequests(t *testing.T) {
	h := newHarness(t, options{})
	c := h.client()
	c.register()

	tests := []struct {
		name string
		path string
		body any
	}{
		{name: "unknown field", path: "/api/v2/administrations", body: map[string]any{"administration": map[string]string{"name": "A", "evil": "x"}}},
		{name: "missing name", path: "/api/v2/administrations", body: map[string]any{"administration": map[string]string{}}},
		{name: "not json", path: "/api/v2/administrations", body: "{nope"},
		{name: "bad enum", path: "/api/v2/administrations", body: map[string]any{"administration": map[string]string{"name": "A", "language": "xx"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantStatus(t, c.do(http.MethodPost, tt.path, tt.body), http.StatusBadRequest)
		})
	}
}

func TestInvalidPathIDIsRejected(t *testing.T) {
	c := newHarness(t, options{}).client()
	c.register()
	wantStatus(t, c.do(http.MethodGet, "/api/v2/administrations/abc", nil), http.StatusBadRequest)
}

func TestUnknownAPIPathIsJSON404(t *testing.T) {
	c := newHarness(t, options{}).client()
	c.register()
	r := c.do(http.MethodGet, "/api/v2/nothing/here", nil)
	wantStatus(t, r, http.StatusNotFound)
	if got := r.header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", got)
	}
}

func TestValidationErrorsUseMoneybirdShape(t *testing.T) {
	c := newHarness(t, options{}).client()
	c.register()
	r := c.do(http.MethodPost, "/api/v2/administrations", map[string]any{"administration": map[string]string{"name": "   "}})
	wantStatus(t, r, http.StatusUnprocessableEntity)
	want := map[string]map[string][]string{"error": {"name": {"can't be blank"}}}
	if diff := cmp.Diff(want, decode[map[string]map[string][]string](t, r)); diff != "" {
		t.Errorf("error body diff (-want +got):\n%s", diff)
	}
}

func TestTenantIsolation(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	a := owner.createAdministration("Private")

	for _, tt := range []struct{ method, path string }{
		{http.MethodGet, "/api/v2/administrations/" + a.Id},
		{http.MethodPut, "/api/v2/administrations/" + a.Id + "/favorite"},
		{http.MethodDelete, "/api/v2/administrations/" + a.Id + "/favorite"},
		{http.MethodGet, "/api/resources/" + a.Id},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			wantStatus(t, stranger.do(tt.method, tt.path, nil), http.StatusNotFound)
		})
	}
}

func TestFavoritesSortFirst(t *testing.T) {
	c := newHarness(t, options{}).client()
	c.register()
	c.createAdministration("Alpha")
	zeta := c.createAdministration("Zeta")

	wantStatus(t, c.do(http.MethodPut, "/api/v2/administrations/"+zeta.Id+"/favorite", nil), http.StatusNoContent)
	got := decode[[]httpapi.Administration](t, c.do(http.MethodGet, "/api/v2/administrations", nil))
	if got[0].Name != "Zeta" || !got[0].Favorite {
		t.Errorf("first administration = %+v, want favorite Zeta", got[0])
	}

	wantStatus(t, c.do(http.MethodDelete, "/api/v2/administrations/"+zeta.Id+"/favorite", nil), http.StatusNoContent)
	got = decode[[]httpapi.Administration](t, c.do(http.MethodGet, "/api/v2/administrations", nil))
	if got[0].Name != "Alpha" {
		t.Errorf("first administration = %+v, want Alpha", got[0])
	}
}

func TestLocateResourceReturnsCanonicalURI(t *testing.T) {
	c := newHarness(t, options{}).client()
	c.register()
	a := c.createAdministration("Located")

	r := c.do(http.MethodGet, "/api/resources/"+a.Id, nil)
	wantStatus(t, r, http.StatusOK)
	want := httpapi.ResourceLocation{Id: a.Id, Type: "administrations", Uri: "/" + a.Id}
	if diff := cmp.Diff(want, decode[httpapi.ResourceLocation](t, r)); diff != "" {
		t.Errorf("locate diff (-want +got):\n%s", diff)
	}
}

func TestSessionLifecycle(t *testing.T) {
	h := newHarness(t, options{})
	c := h.client()
	email := uniqueEmail()
	body := map[string]string{"email": email, "name": "Ann", "password": password}
	r := c.do(http.MethodPost, "/api/auth/register", body)
	wantStatus(t, r, http.StatusCreated)

	cookie := r.header.Get("Set-Cookie")
	for _, attr := range []string{"moneyharpy_session=", "HttpOnly", "SameSite=Lax", "Path=/"} {
		if !strings.Contains(cookie, attr) {
			t.Errorf("Set-Cookie = %q, want attribute %q", cookie, attr)
		}
	}

	wantStatus(t, c.do(http.MethodGet, "/api/auth/me", nil), http.StatusOK)
	wantStatus(t, c.do(http.MethodPost, "/api/auth/logout", nil), http.StatusNoContent)
	wantStatus(t, c.do(http.MethodGet, "/api/auth/me", nil), http.StatusUnauthorized)

	wrong := map[string]string{"email": email, "password": "not the right password"}
	wantStatus(t, c.do(http.MethodPost, "/api/auth/login", wrong), http.StatusUnauthorized)

	right := map[string]string{"email": email, "password": password}
	wantStatus(t, c.do(http.MethodPost, "/api/auth/login", right), http.StatusOK)
	wantStatus(t, c.do(http.MethodGet, "/api/auth/me", nil), http.StatusOK)

	wantStatus(t, c.do(http.MethodPost, "/api/auth/register", body), http.StatusUnprocessableEntity)
}

func TestCrossSiteWritesAreRejected(t *testing.T) {
	c := newHarness(t, options{}).client()
	c.register()
	r := c.do(http.MethodPost, "/api/auth/logout", nil, [2]string{"Sec-Fetch-Site", "cross-site"})
	wantStatus(t, r, http.StatusForbidden)
	wantStatus(t, c.do(http.MethodGet, "/api/auth/me", nil), http.StatusOK)
}

func TestRateLimitFollowsMoneybirdHeaders(t *testing.T) {
	c := newHarness(t, options{apiLimiter: ratelimit.New(1, 5*time.Minute)}).client()
	wantStatus(t, c.do(http.MethodGet, "/api/v2/administrations", nil), http.StatusUnauthorized)

	r := c.do(http.MethodGet, "/api/v2/administrations", nil)
	wantStatus(t, r, http.StatusTooManyRequests)
	if got := string(r.body); strings.TrimSpace(got) != `"Retry later"` {
		t.Errorf("body = %q, want JSON string Retry later", got)
	}
	for _, name := range []string{"Retry-After", "RateLimit-Limit", "RateLimit-Remaining", "RateLimit-Reset"} {
		if r.header.Get(name) == "" {
			t.Errorf("header %s missing", name)
		}
	}
}

func TestInstanceSettings(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	admin, member := h.client(), h.client()

	if u := admin.register(); !u.InstanceAdmin {
		t.Fatal("first registered user is not instance admin")
	}
	if u := member.register(); u.InstanceAdmin {
		t.Fatal("second registered user is instance admin")
	}

	wantStatus(t, member.do(http.MethodGet, "/api/instance/settings", nil), http.StatusForbidden)
	wantStatus(t, member.do(http.MethodPatch, "/api/instance/settings", map[string]bool{"registration_open": false}), http.StatusForbidden)

	wantStatus(t, admin.do(http.MethodPatch, "/api/instance/settings", map[string]any{"google_enabled": true}), http.StatusUnprocessableEntity)

	const secret = "super-secret-value"
	r := admin.do(http.MethodPatch, "/api/instance/settings", map[string]any{
		"registration_open": false, "google_enabled": true, "google_client_id": "id", "google_client_secret": secret,
	})
	wantStatus(t, r, http.StatusOK)
	if strings.Contains(string(r.body), secret) {
		t.Error("settings response contains the secret")
	}
	want := httpapi.InstanceSettings{RegistrationOpen: false, GoogleEnabled: true, GoogleClientId: "id", GoogleClientSecretSet: true}
	if diff := cmp.Diff(want, decode[httpapi.InstanceSettings](t, r)); diff != "" {
		t.Errorf("settings diff (-want +got):\n%s", diff)
	}

	providers := decode[httpapi.AuthProviders](t, h.client().do(http.MethodGet, "/api/auth/providers", nil))
	if diff := cmp.Diff(httpapi.AuthProviders{RegistrationOpen: false, Google: true}, providers); diff != "" {
		t.Errorf("providers diff (-want +got):\n%s", diff)
	}

	late := h.client().do(http.MethodPost, "/api/auth/register", map[string]string{"email": uniqueEmail(), "name": "Late", "password": password})
	wantStatus(t, late, http.StatusForbidden)
}

func googleEnabled(t *testing.T, h *harness) {
	t.Helper()
	on, id, secret := true, "id", "secret"
	if _, err := h.settle.Update(context.Background(), settings.Update{GoogleEnabled: &on, GoogleClientID: &id, GoogleClientSecret: &secret}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
}

func TestGoogleLogin(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer cleanup()

	h := newHarness(t, options{pool: pool})
	wantStatus(t, h.client().do(http.MethodGet, "/api/auth/google/start", nil), http.StatusNotFound)
	googleEnabled(t, h)

	start := func(t *testing.T, c *client) (state, nonce string) {
		t.Helper()
		r := c.do(http.MethodGet, "/api/auth/google/start", nil)
		wantStatus(t, r, http.StatusFound)
		loc, err := url.Parse(r.header.Get("Location"))
		if err != nil {
			t.Fatalf("Parse(Location) error = %v", err)
		}
		return loc.Query().Get("state"), loc.Query().Get("nonce")
	}

	t.Run("signs in and carries the nonce", func(t *testing.T) {
		h.google.external = identity.External{Provider: "google", Subject: "s1", Email: uniqueEmail(), EmailVerified: true, Name: "G"}
		c := h.client()
		state, nonce := start(t, c)

		r := c.do(http.MethodGet, "/api/auth/google/callback?code=abc&state="+state, nil)
		wantStatus(t, r, http.StatusFound)
		if got := r.header.Get("Location"); got != "/administrations" {
			t.Errorf("Location = %q, want /administrations", got)
		}
		if h.google.gotNonce != nonce {
			t.Errorf("Exchange nonce = %q, want %q", h.google.gotNonce, nonce)
		}
		wantStatus(t, c.do(http.MethodGet, "/api/auth/me", nil), http.StatusOK)
	})

	t.Run("rejects mismatched state", func(t *testing.T) {
		c := h.client()
		start(t, c)
		r := c.do(http.MethodGet, "/api/auth/google/callback?code=abc&state=forged", nil)
		wantStatus(t, r, http.StatusFound)
		if got := r.header.Get("Location"); got != "/login?error=google" {
			t.Errorf("Location = %q, want /login?error=google", got)
		}
		wantStatus(t, c.do(http.MethodGet, "/api/auth/me", nil), http.StatusUnauthorized)
	})

	t.Run("reports provider failure", func(t *testing.T) {
		h.google.err = errors.New("exchange failed")
		defer func() { h.google.err = nil }()
		c := h.client()
		state, _ := start(t, c)
		r := c.do(http.MethodGet, "/api/auth/google/callback?code=abc&state="+state, nil)
		if got := r.header.Get("Location"); got != "/login?error=google" {
			t.Errorf("Location = %q, want /login?error=google", got)
		}
	})
}

func TestLM2Login(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer cleanup()

	h := newHarness(t, options{pool: pool})
	wantStatus(t, h.client().do(http.MethodGet, "/api/auth/lm2/start", nil), http.StatusNotFound)
	if got := decode[httpapi.AuthProviders](t, h.client().do(http.MethodGet, "/api/auth/providers", nil)); got.Lm2 {
		t.Error("providers lm2 = true before it is enabled")
	}

	on, addr, app := true, "https://lm.example", "moneyharpy"
	if _, err := h.settle.Update(ctx, settings.Update{LM2Enabled: &on, LM2URL: &addr, LM2App: &app}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	start := func(t *testing.T, c *client) string {
		t.Helper()
		r := c.do(http.MethodGet, "/api/auth/lm2/start", nil)
		wantStatus(t, r, http.StatusFound)
		loc, err := url.Parse(r.header.Get("Location"))
		if err != nil {
			t.Fatalf("Parse(Location) error = %v", err)
		}
		return loc.Query().Get("state")
	}
	location := func(t *testing.T, r response, want string) {
		t.Helper()
		wantStatus(t, r, http.StatusFound)
		if got := r.header.Get("Location"); got != want {
			t.Errorf("Location = %q, want %q", got, want)
		}
	}

	t.Run("signs in with the returned token", func(t *testing.T) {
		h.lm2.external = identity.External{Provider: "lm2", Subject: "u1", Email: uniqueEmail(), Name: "Ann"}
		c := h.client()
		state := start(t, c)
		location(t, c.do(http.MethodGet, "/api/auth/lm2/callback/"+state+"?token=abc&refresh_token=def", nil), "/administrations")
		if h.lm2.gotToken != "abc" {
			t.Errorf("Identify token = %q, want abc", h.lm2.gotToken)
		}
		wantStatus(t, c.do(http.MethodGet, "/api/auth/me", nil), http.StatusOK)
	})

	t.Run("rejects forged state", func(t *testing.T) {
		c := h.client()
		start(t, c)
		location(t, c.do(http.MethodGet, "/api/auth/lm2/callback/forged?token=abc", nil), "/login?error=lm2")
		wantStatus(t, c.do(http.MethodGet, "/api/auth/me", nil), http.StatusUnauthorized)
	})

	t.Run("rejects a callback without a prior start", func(t *testing.T) {
		location(t, h.client().do(http.MethodGet, "/api/auth/lm2/callback/any?token=abc", nil), "/login?error=lm2")
	})

	t.Run("rejects a state used twice", func(t *testing.T) {
		c := h.client()
		state := start(t, c)
		c.do(http.MethodGet, "/api/auth/lm2/callback/"+state+"?token=abc", nil)
		location(t, c.do(http.MethodGet, "/api/auth/lm2/callback/"+state+"?token=abc", nil), "/login?error=lm2")
	})

	t.Run("rejects a missing token", func(t *testing.T) {
		c := h.client()
		state := start(t, c)
		location(t, c.do(http.MethodGet, "/api/auth/lm2/callback/"+state, nil), "/login?error=lm2")
	})

	t.Run("reports a token LM2 refuses", func(t *testing.T) {
		h.lm2.err = errors.New("refused")
		defer func() { h.lm2.err = nil }()
		c := h.client()
		state := start(t, c)
		location(t, c.do(http.MethodGet, "/api/auth/lm2/callback/"+state+"?token=abc", nil), "/login?error=lm2")
	})

	t.Run("does not take over an existing account by email", func(t *testing.T) {
		owner := h.client()
		email := uniqueEmail()
		wantStatus(t, owner.do(http.MethodPost, "/api/auth/register", map[string]string{"email": email, "name": "Own", "password": password}), http.StatusCreated)

		h.lm2.external = identity.External{Provider: "lm2", Subject: "u2", Email: email, Name: "Own"}
		c := h.client()
		state := start(t, c)
		location(t, c.do(http.MethodGet, "/api/auth/lm2/callback/"+state+"?token=abc", nil), "/login?error=email_taken")
	})
}

func TestServesContractAndWebApp(t *testing.T) {
	c := newHarness(t, options{}).client()

	spec := c.do(http.MethodGet, "/api/openapi.yaml", nil)
	wantStatus(t, spec, http.StatusOK)
	if !bytes.Contains(spec.body, []byte("MoneyHarpy API")) {
		t.Error("/api/openapi.yaml does not serve the contract")
	}

	wantStatus(t, c.do(http.MethodGet, "/healthz", nil), http.StatusOK)

	for _, path := range []string{"/", "/administrations", "/416072101932303822/settings", "/r/123"} {
		r := c.do(http.MethodGet, path, nil)
		wantStatus(t, r, http.StatusOK)
		if string(r.body) != "<html>spa</html>" {
			t.Errorf("GET %s = %q, want the web app", path, r.body)
		}
		if r.header.Get("Content-Security-Policy") == "" {
			t.Errorf("GET %s has no Content-Security-Policy", path)
		}
	}

	asset := c.do(http.MethodGet, "/assets/app.js", nil)
	if string(asset.body) != "js" {
		t.Errorf("GET /assets/app.js = %q, want the asset", asset.body)
	}
}
