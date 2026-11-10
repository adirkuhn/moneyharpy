package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/adirkuhn/moneyharpy/api"
	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/connections"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/hookclient"
	"github.com/adirkuhn/moneyharpy/internal/httpapi"
	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/membership"
	"github.com/adirkuhn/moneyharpy/internal/overview"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
	"github.com/adirkuhn/moneyharpy/internal/ratelimit"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/adirkuhn/moneyharpy/internal/webhooks"
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

type fakeLookups struct {
	companies    []administration.CompanyMatch
	companiesErr error
	gotQuery     string
	address      administration.AddressMatch
	addressErr   error
}

func (f *fakeLookups) SearchCompanies(_ context.Context, query string) ([]administration.CompanyMatch, error) {
	f.gotQuery = query
	return f.companies, f.companiesErr
}

func (f *fakeLookups) LookupAddress(_ context.Context, _, _ string) (administration.AddressMatch, error) {
	return f.address, f.addressErr
}

type sentMail struct{ to, token, language string }

type fakeMailer struct {
	resets      chan sentMail
	testErr     error
	tested      []string
	invitations []sentMail
}

func (m *fakeMailer) SendInvitation(_ context.Context, to, token, _, _, language string) error {
	m.invitations = append(m.invitations, sentMail{to: to, token: token, language: language})
	return nil
}

func (m *fakeMailer) InvitationURL(token string) string {
	return "http://example.com/invitations/" + token
}

func (m *fakeMailer) SendPasswordReset(_ context.Context, to, token, language string) error {
	m.resets <- sentMail{to, token, language}
	return nil
}

func (m *fakeMailer) SendTest(_ context.Context, to string) error {
	m.tested = append(m.tested, to)
	return m.testErr
}

// fakeFiles is an in-memory file store.
type fakeFiles struct {
	mu       sync.Mutex
	data     map[string][]byte
	checkErr error
}

func (f *fakeFiles) Put(_ context.Context, key, _ string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = data
	return nil
}

func (f *fakeFiles) Open(_ context.Context, key string) (io.ReadCloser, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.data[key]
	if !ok {
		return nil, "", invoicing.ErrFileNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), "", nil
}

func (f *fakeFiles) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, key)
	return nil
}

func (f *fakeFiles) Check(context.Context) error { return f.checkErr }

type options struct {
	pool       *pgxpool.Pool
	apiLimiter *ratelimit.Limiter
}

type harness struct {
	t      *testing.T
	server *httptest.Server
	google *fakeGoogle
	lm2    *fakeLM2
	lookup *fakeLookups
	mailer *fakeMailer
	files  *fakeFiles
	settle *settings.Service
	hooks  *webhooks.Service
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

	lookup := &fakeLookups{}
	mailer := &fakeMailer{resets: make(chan sentMail, 8)}

	files := &fakeFiles{data: map[string][]byte{}}
	h := &harness{t: t, google: google, lm2: lm2, lookup: lookup, mailer: mailer, files: files, settle: settle}
	h.server = httptest.NewUnstartedServer(nil)
	h.server.Start()
	t.Cleanup(h.server.Close)

	admins := administration.NewService(postgres.NewAdministrationStore(pool), ids, time.Now)
	books := bookkeeping.NewService(admins, postgres.NewBookkeepingStore(pool), ids, time.Now)
	h.hooks = webhooks.NewService(admins, postgres.NewWebhookStore(pool, settings.NewCipher("test-secret")), hookclient.New(), settle, ids, time.Now)
	invoices := invoicing.NewService(admins, postgres.NewInvoicingStore(pool), files, ids, time.Now)
	people := identity.NewService(postgres.NewIdentityStore(pool), settle, ids, identity.NewHasher(identity.Params{Time: 1, MemoryKiB: 8, Threads: 1}), mailer, settings.NewCipher("test-secret"), time.Now)
	bank := banking.NewService(admins, books, postgres.NewBankingStore(pool), ids, time.Now)
	handler, err := httpapi.NewHandler(httpapi.Deps{
		Administrations: admins,
		Invoicing:       invoices,
		Membership:      membership.NewService(admins, postgres.NewMembershipStore(pool), mailer, people, invoices, ids, time.Now),
		Connections:     connections.NewService(admins, postgres.NewConnectionStore(pool), ids, time.Now),
		Webhooks:        h.hooks,
		Bookkeeping:     books,
		Banking:         bank,
		Overview:        overview.NewService(admins, bank),
		Contacts:        contacts.NewService(admins, invoices, admins, nil, postgres.NewContactStore(pool), ids, time.Now),
		Storage:         files,
		Identity:        people,
		Settings:        settle,
		Resources:       postgres.NewResourceLocator(pool),
		Google:          google,
		LM2:             lm2,
		Companies:       lookup,
		Addresses:       lookup,
		LookupLimiter:   ratelimit.New(100000, time.Minute),
		ResetLimiter:    ratelimit.New(2, time.Hour),
		Mail:            mailer,
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
				"small_business_scheme": false, "company_activities_description": "", "company_website_url": "",
				"company_website_absent": false, "user_type": "owner", "require_two_factor": false, "support_access_until": nil,
				"permissions": []any{"sales_invoices", "documents", "estimates", "bank", "settings", "ownership", "time_entries"},
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

func TestCreateAdministrationStartsDefaultIdentity(t *testing.T) {
	c := newHarness(t, options{}).client()
	c.register()
	r := c.do(http.MethodPost, "/api/v2/administrations", map[string]any{"administration": map[string]string{
		"name": "Harpy", "address1": "Dam 1", "address2": "Floor 2", "zipcode": "1012JS", "city": "Amsterdam", "chamber_of_commerce": "12345678",
	}})
	wantStatus(t, r, http.StatusCreated)
	created := decode[httpapi.Administration](t, r)

	for _, path := range []string{"/api/v2/" + created.Id + "/identities/default", "/api/v2/" + created.Id + "/identities/default.json"} {
		got := decode[map[string]any](t, c.do(http.MethodGet, path, nil))
		for field, want := range map[string]any{
			"company_name": "Harpy", "address1": "Dam 1", "address2": "Floor 2", "zipcode": "1012JS", "city": "Amsterdam",
			"chamber_of_commerce": "12345678", "administration_id": created.Id, "country": "NL", "phone": nil, "default": true,
		} {
			if got[field] != want {
				t.Errorf("GET %s %s = %v, want %v", path, field, got[field], want)
			}
		}
	}

	row := decode[[]map[string]any](t, c.do(http.MethodGet, "/api/v2/administrations", nil))[0]
	if _, leaked := row["address1"]; leaked {
		t.Error("administrations list includes address1, want Moneybird's fields only")
	}
}

func TestIdentities(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	base := "/api/v2/" + a.Id + "/identities"
	body := func(fields map[string]string) map[string]any { return map[string]any{"identity": fields} }

	r := owner.do(http.MethodPost, base, body(map[string]string{"company_name": "Trade", "country": "NL", "bank_account_number": "NL91 ABNA 0417 1643 00"}))
	wantStatus(t, r, http.StatusCreated)
	extra := decode[httpapi.Identity](t, r)
	if extra.Default {
		t.Error("created identity is default")
	}
	if want := "/" + a.Id + "/identities/" + extra.Id; extra.Uri != want {
		t.Errorf("uri = %q, want %q", extra.Uri, want)
	}
	if extra.BankAccountNumber == nil || *extra.BankAccountNumber != "NL91ABNA0417164300" {
		t.Errorf("bank_account_number = %v", extra.BankAccountNumber)
	}

	listed := decode[[]httpapi.Identity](t, owner.do(http.MethodGet, base, nil))
	if len(listed) != 2 || !listed[0].Default {
		t.Errorf("list = %+v, want default first", listed)
	}

	r = owner.do(http.MethodPatch, base+"/"+extra.Id, body(map[string]string{"phone": "0612345678"}))
	wantStatus(t, r, http.StatusOK)
	if got := decode[httpapi.Identity](t, r); got.Phone == nil || *got.Phone != "0612345678" || got.CompanyName == nil || *got.CompanyName != "Trade" {
		t.Errorf("patched = %+v", got)
	}

	r = owner.do(http.MethodPatch, base+"/default", body(map[string]string{"email": "books@example.com"}))
	wantStatus(t, r, http.StatusOK)
	if got := decode[httpapi.Identity](t, r); got.Email == nil || *got.Email != "books@example.com" {
		t.Errorf("patched default = %+v", got)
	}

	r = owner.do(http.MethodPost, base, body(map[string]string{"company_name": "X", "country": "NL", "bank_account_number": "NL91ABNA0417164301"}))
	wantStatus(t, r, http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPost, base, map[string]any{"identity": map[string]string{"evil": "x"}}), http.StatusBadRequest)

	located := decode[httpapi.ResourceLocation](t, owner.do(http.MethodGet, "/api/resources/"+extra.Id, nil))
	if located.Uri != extra.Uri {
		t.Errorf("located uri = %q, want %q", located.Uri, extra.Uri)
	}

	for _, tt := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, base, nil},
		{http.MethodPost, base, body(map[string]string{"company_name": "X", "country": "NL"})},
		{http.MethodGet, base + "/default", nil},
		{http.MethodPatch, base + "/default", body(map[string]string{"phone": "1"})},
		{http.MethodGet, base + "/" + extra.Id, nil},
		{http.MethodPatch, base + "/" + extra.Id, body(map[string]string{"phone": "1"})},
		{http.MethodDelete, base + "/" + extra.Id, nil},
	} {
		t.Run("stranger "+tt.method+" "+tt.path, func(t *testing.T) {
			wantStatus(t, stranger.do(tt.method, tt.path, tt.body), http.StatusNotFound)
		})
	}

	def := decode[httpapi.Identity](t, owner.do(http.MethodGet, base+"/default", nil))
	wantStatus(t, owner.do(http.MethodDelete, base+"/"+def.Id, nil), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodDelete, base+"/"+extra.Id, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, base+"/"+extra.Id, nil), http.StatusNotFound)
}

func TestUpdateAdministration(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	path := "/api/v2/administrations/" + a.Id
	patch := func(fields map[string]string) map[string]any { return map[string]any{"administration": fields} }

	r := owner.do(http.MethodPatch, path, patch(map[string]string{"name": "New", "language": "nl", "currency": "USD", "time_zone": "Europe/Brussels"}))
	wantStatus(t, r, http.StatusOK)
	got := decode[httpapi.Administration](t, r)
	if got.Name != "New" || got.Language != "nl" || got.Currency != "USD" || got.TimeZone != "Europe/Brussels" {
		t.Errorf("updated = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodPatch, path, patch(map[string]string{"currency": "ABC"})), http.StatusUnprocessableEntity)
	wantStatus(t, stranger.do(http.MethodPatch, path, patch(map[string]string{"name": "X"})), http.StatusNotFound)
}

func TestCompanyProfile(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	a := owner.createAdministration("Harpy")
	if a.SmallBusinessScheme || a.CompanyWebsiteUrl != "" || a.CompanyActivitiesDescription != "" || a.CompanyWebsiteAbsent {
		t.Errorf("new administration profile = %+v, want empty", a)
	}
	path := "/api/v2/administrations/" + a.Id

	r := owner.do(http.MethodPatch, path, map[string]any{"administration": map[string]any{
		"small_business_scheme": true, "company_activities_description": "Bakery", "company_website_url": "https://example.com",
	}})
	wantStatus(t, r, http.StatusOK)
	got := decode[httpapi.Administration](t, r)
	if !got.SmallBusinessScheme || got.CompanyActivitiesDescription != "Bakery" || got.CompanyWebsiteUrl != "https://example.com" {
		t.Errorf("updated = %+v", got)
	}
	r = owner.do(http.MethodPatch, path, map[string]any{"administration": map[string]any{"company_website_url": "example"}})
	wantStatus(t, r, http.StatusUnprocessableEntity)
	if !bytes.Contains(r.body, []byte("company_website_url")) {
		t.Errorf("error body = %s, want company_website_url", r.body)
	}
}

func TestCustomFields(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	base := "/api/v2/" + a.Id + "/custom_fields"
	field := func(name, source string) map[string]any {
		return map[string]any{"custom_field": map[string]string{"name": name, "source": source}}
	}

	r := owner.do(http.MethodPost, base, field("Slogan", "identity"))
	wantStatus(t, r, http.StatusCreated)
	slogan := decode[httpapi.CustomField](t, r)
	if slogan.Name != "Slogan" || slogan.Source != httpapi.CustomFieldSourceIdentity || slogan.Uri != "/"+a.Id+"/custom_fields/"+slogan.Id {
		t.Errorf("created = %+v", slogan)
	}
	contact := decode[httpapi.CustomField](t, owner.do(http.MethodPost, base, field("Ref", "contact")))

	listed := decode[[]httpapi.CustomField](t, owner.do(http.MethodGet, base+".json", nil))
	if len(listed) != 2 || listed[0].Id != slogan.Id {
		t.Errorf("list = %+v", listed)
	}
	wantStatus(t, owner.do(http.MethodPost, base, field(" ", "identity")), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPost, base, field("X", "planet")), http.StatusBadRequest)
	wantStatus(t, stranger.do(http.MethodGet, base, nil), http.StatusNotFound)
	wantStatus(t, stranger.do(http.MethodPost, base, field("X", "identity")), http.StatusNotFound)

	identity := "/api/v2/" + a.Id + "/identities/default"
	set := func(id, value string, extra map[string]any) map[string]any {
		fields := map[string]any{"custom_fields_attributes": []map[string]string{{"id": id, "value": value}}}
		maps.Copy(fields, extra)
		return map[string]any{"identity": fields}
	}
	r = owner.do(http.MethodPatch, identity, set(slogan.Id, "Fresh", map[string]any{"tax_number": "", "tax_number_absence_reason": "industry_exemption"}))
	wantStatus(t, r, http.StatusOK)
	got := decode[httpapi.Identity](t, r)
	want := []httpapi.CustomFieldValue{{Id: slogan.Id, Name: "Slogan", Value: "Fresh"}}
	if diff := cmp.Diff(want, got.CustomFields); diff != "" {
		t.Errorf("custom_fields diff (-want +got):\n%s", diff)
	}
	if got.TaxNumberAbsenceReason == nil || *got.TaxNumberAbsenceReason != httpapi.TaxNumberAbsenceReasonIndustryExemption {
		t.Errorf("tax_number_absence_reason = %v", got.TaxNumberAbsenceReason)
	}
	wantStatus(t, owner.do(http.MethodPatch, identity, set(contact.Id, "x", nil)), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPatch, identity, map[string]any{"identity": map[string]string{"tax_number_absence_reason": "lazy"}}), http.StatusBadRequest)
}

func TestOnboardingLookups(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	c := h.client()
	c.register()

	const search = "/api/onboarding/company_search?country=NL&query=harpy"
	const lookup = "/api/onboarding/address_lookup?zipcode=1012RJ&house_number=147"

	t.Run("options without integrations", func(t *testing.T) {
		got := decode[httpapi.OnboardingOptions](t, c.do(http.MethodGet, "/api/onboarding/options", nil))
		if len(got.CompanySearchCountries) != 0 || len(got.AddressLookupCountries) != 0 {
			t.Errorf("options = %+v, want no lookups", got)
		}
		if len(got.Countries) == 0 || len(got.Currencies) == 0 || len(got.PopularCurrencies) == 0 {
			t.Errorf("options = %+v, want reference lists", got)
		}
	})

	t.Run("lookups are unavailable until enabled", func(t *testing.T) {
		h.lookup.companiesErr, h.lookup.addressErr = administration.ErrLookupUnavailable, administration.ErrLookupUnavailable
		defer func() { h.lookup.companiesErr, h.lookup.addressErr = nil, nil }()
		wantStatus(t, c.do(http.MethodGet, search, nil), http.StatusNotFound)
		wantStatus(t, c.do(http.MethodGet, lookup, nil), http.StatusNotFound)
	})

	on, key := true, "key"
	if _, err := h.settle.Update(ctx, settings.Update{KVKEnabled: &on, KVKAPIKey: &key, AddressLookupEnabled: &on}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	t.Run("options with integrations", func(t *testing.T) {
		got := decode[httpapi.OnboardingOptions](t, c.do(http.MethodGet, "/api/onboarding/options", nil))
		want := []string{"NL"}
		if diff := cmp.Diff(want, got.CompanySearchCountries); diff != "" {
			t.Errorf("company_search_countries diff (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(want, got.AddressLookupCountries); diff != "" {
			t.Errorf("address_lookup_countries diff (-want +got):\n%s", diff)
		}
	})

	t.Run("searches the register", func(t *testing.T) {
		h.lookup.companies = []administration.CompanyMatch{{Name: "Harpy", ChamberOfCommerce: "123", City: "Lochem"}}
		r := c.do(http.MethodGet, search, nil)
		wantStatus(t, r, http.StatusOK)
		want := []httpapi.CompanyMatch{{Name: "Harpy", ChamberOfCommerce: "123", City: "Lochem"}}
		if diff := cmp.Diff(want, decode[[]httpapi.CompanyMatch](t, r)); diff != "" {
			t.Errorf("search diff (-want +got):\n%s", diff)
		}
		if h.lookup.gotQuery != "harpy" {
			t.Errorf("query = %q, want harpy", h.lookup.gotQuery)
		}
	})

	t.Run("no matches is an empty list", func(t *testing.T) {
		h.lookup.companies = nil
		r := c.do(http.MethodGet, search, nil)
		wantStatus(t, r, http.StatusOK)
		if got := strings.TrimSpace(string(r.body)); got != "[]" {
			t.Errorf("body = %q, want []", got)
		}
	})

	t.Run("only searches registers it supports", func(t *testing.T) {
		wantStatus(t, c.do(http.MethodGet, "/api/onboarding/company_search?country=BE&query=harpy", nil), http.StatusNotFound)
	})

	t.Run("finds an address", func(t *testing.T) {
		h.lookup.address = administration.AddressMatch{Address1: "NZ Voorburgwal 147", Zipcode: "1012RJ", City: "Amsterdam"}
		r := c.do(http.MethodGet, lookup, nil)
		wantStatus(t, r, http.StatusOK)
		want := httpapi.AddressMatch{Address1: "NZ Voorburgwal 147", Zipcode: "1012RJ", City: "Amsterdam"}
		if diff := cmp.Diff(want, decode[httpapi.AddressMatch](t, r)); diff != "" {
			t.Errorf("lookup diff (-want +got):\n%s", diff)
		}
	})

	t.Run("unknown address is not found", func(t *testing.T) {
		h.lookup.addressErr = administration.ErrAddressUnknown
		defer func() { h.lookup.addressErr = nil }()
		wantStatus(t, c.do(http.MethodGet, lookup, nil), http.StatusNotFound)
	})

	t.Run("requires sign in", func(t *testing.T) {
		wantStatus(t, h.client().do(http.MethodGet, search, nil), http.StatusUnauthorized)
	})

	t.Run("key is never returned", func(t *testing.T) {
		r := c.do(http.MethodGet, "/api/instance/settings", nil)
		wantStatus(t, r, http.StatusOK)
		got := decode[httpapi.InstanceSettings](t, r)
		if !got.KvkEnabled || !got.KvkApiKeySet || !got.AddressLookupEnabled {
			t.Errorf("settings = %+v, want integrations on", got)
		}
		if strings.Contains(string(r.body), `"kvk_api_key":`) {
			t.Error("settings response contains the key")
		}
	})
}

func TestPasswordReset(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	const request, confirm = "/api/auth/password_reset/request", "/api/auth/password_reset/confirm"

	browser := h.client()
	email := uniqueEmail()
	wantStatus(t, browser.do(http.MethodPost, "/api/auth/register", map[string]string{"email": email, "name": "Ann", "password": password}), http.StatusCreated)
	ask := map[string]string{"email": email, "language": "nl"}

	t.Run("unavailable until mail is configured", func(t *testing.T) {
		wantStatus(t, h.client().do(http.MethodPost, request, ask), http.StatusNotFound)
		if got := decode[httpapi.AuthProviders](t, h.client().do(http.MethodGet, "/api/auth/providers", nil)); got.PasswordReset {
			t.Error("providers password_reset = true before mail is configured")
		}
	})

	on, host, port, from, security := true, "mail.example.com", 587, "books@example.com", "starttls"
	if _, err := h.settle.Update(ctx, settings.Update{SMTPEnabled: &on, SMTPHost: &host, SMTPPort: &port, SMTPFrom: &from, SMTPSecurity: &security}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got := decode[httpapi.AuthProviders](t, h.client().do(http.MethodGet, "/api/auth/providers", nil)); !got.PasswordReset {
		t.Error("providers password_reset = false after mail is configured")
	}

	receive := func(t *testing.T) sentMail {
		t.Helper()
		select {
		case m := <-h.mailer.resets:
			return m
		case <-time.After(5 * time.Second):
			t.Fatal("no reset mail was sent")
			return sentMail{}
		}
	}

	t.Run("unknown address looks the same and sends nothing", func(t *testing.T) {
		wantStatus(t, h.client().do(http.MethodPost, request, map[string]string{"email": uniqueEmail()}), http.StatusNoContent)
		select {
		case m := <-h.mailer.resets:
			t.Errorf("mail sent to unknown address: %+v", m)
		case <-time.After(200 * time.Millisecond):
		}
	})

	var token string
	t.Run("mails a token", func(t *testing.T) {
		wantStatus(t, h.client().do(http.MethodPost, request, ask), http.StatusNoContent)
		m := receive(t)
		if m.to != email || m.language != "nl" || m.token == "" {
			t.Errorf("mail = %+v", m)
		}
		token = m.token
	})

	t.Run("rejects a bad token", func(t *testing.T) {
		r := h.client().do(http.MethodPost, confirm, map[string]string{"token": "forged", "password": "a brand new password"})
		wantStatus(t, r, http.StatusUnprocessableEntity)
		if _, ok := decode[httpapi.ValidationError](t, r).Error["token"]; !ok {
			t.Errorf("body = %s, want a token error", r.body)
		}
	})

	t.Run("changes the password and ends sessions", func(t *testing.T) {
		wantStatus(t, h.client().do(http.MethodPost, confirm, map[string]string{"token": token, "password": "a brand new password"}), http.StatusNoContent)
		wantStatus(t, browser.do(http.MethodGet, "/api/auth/me", nil), http.StatusUnauthorized)
		wantStatus(t, h.client().do(http.MethodPost, "/api/auth/login", map[string]string{"email": email, "password": password}), http.StatusUnauthorized)
		wantStatus(t, h.client().do(http.MethodPost, "/api/auth/login", map[string]string{"email": email, "password": "a brand new password"}), http.StatusOK)
	})

	t.Run("stops flooding one inbox", func(t *testing.T) {
		wantStatus(t, h.client().do(http.MethodPost, request, ask), http.StatusNoContent)
		receive(t)
		wantStatus(t, h.client().do(http.MethodPost, request, ask), http.StatusNoContent)
		select {
		case m := <-h.mailer.resets:
			t.Errorf("mail sent past the limit: %+v", m)
		case <-time.After(200 * time.Millisecond):
		}
	})
}

func TestMailSettingsAndTestEmail(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	admin, member := h.client(), h.client()
	me := admin.register()
	member.register()

	const secret = "smtp-password-value"
	r := admin.do(http.MethodPatch, "/api/instance/settings", map[string]any{
		"smtp_enabled": true, "smtp_host": "mail.example.com", "smtp_port": 587, "smtp_username": "u",
		"smtp_password": secret, "smtp_from": "books@example.com", "smtp_security": "starttls",
	})
	wantStatus(t, r, http.StatusOK)
	if strings.Contains(string(r.body), secret) {
		t.Error("settings response contains the password")
	}
	got := decode[httpapi.InstanceSettings](t, r)
	if !got.SmtpEnabled || got.SmtpHost != "mail.example.com" || got.SmtpPort != 587 || !got.SmtpPasswordSet || got.SmtpFrom != "books@example.com" || got.SmtpSecurity != "starttls" {
		t.Errorf("settings = %+v", got)
	}

	const path = "/api/instance/settings/test_email"
	wantStatus(t, member.do(http.MethodPost, path, nil), http.StatusForbidden)
	wantStatus(t, admin.do(http.MethodPost, path, nil), http.StatusNoContent)
	if len(h.mailer.tested) != 1 || h.mailer.tested[0] != me.Email {
		t.Errorf("test mail went to %v, want the admin", h.mailer.tested)
	}

	h.mailer.testErr = errors.New("535 authentication failed")
	r = admin.do(http.MethodPost, path, nil)
	wantStatus(t, r, http.StatusBadGateway)
	if !strings.Contains(string(r.body), "535 authentication failed") {
		t.Errorf("body = %s, want the mail server's reason for the admin", r.body)
	}
}

func TestServesContractAndWebApp(t *testing.T) {
	c := newHarness(t, options{}).client()

	spec := c.do(http.MethodGet, "/api/openapi.yaml", nil)
	wantStatus(t, spec, http.StatusOK)
	if !bytes.Contains(spec.body, []byte("MoneyHarpy API")) {
		t.Error("/api/openapi.yaml does not serve the contract")
	}

	wantStatus(t, c.do(http.MethodGet, "/healthz", nil), http.StatusOK)

	for _, path := range []string{"/", "/administrations", "/123456789012345678/settings", "/r/123"} {
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

func TestStorageSettingsAndCheck(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	admin, member := h.client(), h.client()
	admin.register()
	member.register()

	const secret = "storage-secret-value"
	r := admin.do(http.MethodPatch, "/api/instance/settings", map[string]any{
		"storage_enabled": true, "storage_endpoint": "http://storage:8333", "storage_region": "us-east-1",
		"storage_bucket": "moneyharpy", "storage_access_key": "key", "storage_secret_key": secret, "storage_path_style": true,
	})
	wantStatus(t, r, http.StatusOK)
	if bytes.Contains(r.body, []byte(secret)) {
		t.Error("settings response contains the secret key")
	}
	got := decode[httpapi.InstanceSettings](t, r)
	if !got.StorageEnabled || got.StorageEndpoint != "http://storage:8333" || got.StorageBucket != "moneyharpy" || !got.StorageSecretKeySet || !got.StoragePathStyle {
		t.Errorf("settings = %+v", got)
	}

	const path = "/api/instance/settings/test_storage"
	wantStatus(t, member.do(http.MethodPost, path, nil), http.StatusForbidden)
	wantStatus(t, admin.do(http.MethodPost, path, nil), http.StatusNoContent)
	h.files.checkErr = errors.New("The access key ID you provided does not exist")
	r = admin.do(http.MethodPost, path, nil)
	wantStatus(t, r, http.StatusBadGateway)
	if !bytes.Contains(r.body, []byte("access key ID")) {
		t.Errorf("body = %s, want the storage's reason for the admin", r.body)
	}
}

func TestInvoiceSettings(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	path := "/api/v2/" + a.Id + "/invoice_settings"

	got := decode[httpapi.InvoiceSettings](t, owner.do(http.MethodGet, path, nil))
	if got.InvoiceIdTemplate != "{year}-{id}" || got.InvoiceSequenceId != "0001" || got.InvoiceIdExample != time.Now().Format("2006")+"-0001" {
		t.Errorf("settings = %+v", got)
	}
	r := owner.do(http.MethodPatch, path, map[string]any{"invoice_settings": map[string]any{
		"invoice_id_template": "F{id}", "invoice_sequence_id": "1462", "invoice_bcc_email_address": "a@example.com",
		"invoice_reminders_bcc_enabled": true, "payments_without_proof": false,
	}})
	wantStatus(t, r, http.StatusOK)
	if got = decode[httpapi.InvoiceSettings](t, r); got.InvoiceIdExample != "F1462" || !got.InvoiceRemindersBccEnabled || got.PaymentsWithoutProof {
		t.Errorf("updated = %+v", got)
	}
	r = owner.do(http.MethodPatch, path, map[string]any{"invoice_settings": map[string]any{"invoice_id_template": "{year}"}})
	wantStatus(t, r, http.StatusUnprocessableEntity)
	if !bytes.Contains(r.body, []byte("invoice_id_template")) {
		t.Errorf("body = %s", r.body)
	}
	wantStatus(t, stranger.do(http.MethodGet, path, nil), http.StatusNotFound)
}

func TestWorkflows(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	base := "/api/v2/" + a.Id + "/workflows"

	list := decode[[]httpapi.Workflow](t, owner.do(http.MethodGet, base+".json", nil))
	if len(list) != 2 || *list[1].Type != "InvoiceWorkflow" || !*list[1].Default || len(*list[1].Steps) != 3 {
		t.Fatalf("list = %+v", list)
	}
	r := owner.do(http.MethodPost, base, map[string]any{"workflow": map[string]any{
		"type": "InvoiceWorkflow", "name": "Export", "currency": "USD", "language": "en",
		"steps_attributes": []map[string]any{{"due_interval": 30}, {"reminder_delay": 10, "reminder_text": "Pay", "reminder_auto_send": true}},
	}})
	wantStatus(t, r, http.StatusCreated)
	w := decode[httpapi.Workflow](t, r)
	if *w.Name != "Export" || *w.Default || w.Uri != "/"+a.Id+"/workflows/"+*w.Id || len(*w.Steps) != 2 {
		t.Errorf("created = %+v", w)
	}
	r = owner.do(http.MethodPatch, base+"/"+*w.Id, map[string]any{"workflow": map[string]any{"default": true, "prices_are_incl_tax": true}})
	wantStatus(t, r, http.StatusOK)
	if got := decode[httpapi.Workflow](t, r); !*got.Default || !*got.PricesAreInclTax {
		t.Errorf("updated = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodDelete, base+"/"+*w.Id, nil), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodDelete, base+"/"+*list[1].Id, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodPost, base, map[string]any{"workflow": map[string]any{"type": "InvoiceWorkflow", "name": ""}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPost, base, map[string]any{"workflow": map[string]any{"type": "PurchaseWorkflow"}}), http.StatusBadRequest)
	wantStatus(t, stranger.do(http.MethodGet, base, nil), http.StatusNotFound)
	wantStatus(t, stranger.do(http.MethodGet, base+"/"+*w.Id, nil), http.StatusNotFound)
}

var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)

func TestDocumentStyles(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	base := "/api/v2/" + a.Id + "/document_styles"

	list := decode[[]httpapi.DocumentStyle](t, owner.do(http.MethodGet, base, nil))
	if len(list) != 1 || !*list[0].Default || *list[0].PaperSize != "A4" {
		t.Fatalf("list = %+v", list)
	}
	r := owner.do(http.MethodPost, base, map[string]any{"document_style": map[string]any{
		"name": "The Best", "paper_size": "letter", "font_size": 11,
		"invoice_sender_address": []map[string]any{{"field": "company_name", "bold": true}, {"field": "tax_number", "label": true}},
	}})
	wantStatus(t, r, http.StatusCreated)
	var created map[string]any
	if err := json.Unmarshal(r.body, &created); err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)
	want := []any{map[string]any{"field": "company_name", "bold": true}, map[string]any{"field": "tax_number", "label": true}}
	if diff := cmp.Diff(want, created["invoice_sender_address"]); diff != "" {
		t.Errorf("invoice_sender_address diff (-want +got):\n%s", diff)
	}
	if created["uri"] != "/"+a.Id+"/document_styles/"+id || created["logo_hash"] != nil {
		t.Errorf("created = %v", created)
	}

	logo := base + "/" + id + "/logo"
	r = owner.do(http.MethodPut, logo, string(pngBytes), [2]string{"Content-Type", "image/png"})
	wantStatus(t, r, http.StatusOK)
	if got := decode[httpapi.DocumentStyle](t, r); got.LogoHash == nil || len(*got.LogoHash) != 64 {
		t.Errorf("logo_hash = %v", got.LogoHash)
	}
	r = owner.do(http.MethodGet, logo, nil)
	wantStatus(t, r, http.StatusOK)
	if !bytes.Equal(r.body, pngBytes) || r.header.Get("Content-Type") != "image/png" {
		t.Errorf("logo = %d bytes, %s", len(r.body), r.header.Get("Content-Type"))
	}
	wantStatus(t, stranger.do(http.MethodGet, logo, nil), http.StatusNotFound)
	wantStatus(t, owner.do(http.MethodPut, logo, "%PDF-1.7", [2]string{"Content-Type", "application/pdf"}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPut, logo, "<svg/>", [2]string{"Content-Type", "image/svg+xml"}), http.StatusBadRequest)
	r = owner.do(http.MethodDelete, logo, nil)
	wantStatus(t, r, http.StatusOK)
	wantStatus(t, owner.do(http.MethodGet, logo, nil), http.StatusNotFound)

	// A file lost from storage is reported as missing, not as a failure.
	wantStatus(t, owner.do(http.MethodPut, logo, string(pngBytes), [2]string{"Content-Type", "image/png"}), http.StatusOK)
	h.files.mu.Lock()
	clear(h.files.data)
	h.files.mu.Unlock()
	wantStatus(t, owner.do(http.MethodGet, logo, nil), http.StatusNotFound)

	wantStatus(t, owner.do(http.MethodPatch, base+"/"+id, map[string]any{"document_style": map[string]any{"font_size": 40}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodDelete, base+"/"+*list[0].Id, nil), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodDelete, base+"/"+id, nil), http.StatusNoContent)
	wantStatus(t, stranger.do(http.MethodGet, base, nil), http.StatusNotFound)
}

func TestCustomFieldEditing(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	base := "/api/v2/" + a.Id + "/custom_fields"
	create := func(name string) httpapi.CustomField {
		return decode[httpapi.CustomField](t, owner.do(http.MethodPost, base, map[string]any{"custom_field": map[string]string{"name": name, "source": "identity"}}))
	}
	first, second := create("First"), create("Second")

	r := owner.do(http.MethodPatch, base+"/"+first.Id, map[string]any{"custom_field": map[string]string{"name": "Slogan"}})
	wantStatus(t, r, http.StatusOK)
	if got := decode[httpapi.CustomField](t, r); got.Name != "Slogan" {
		t.Errorf("renamed = %+v", got)
	}
	order := func(ids ...string) map[string]any {
		return map[string]any{"custom_fields_order": map[string]any{"source": "identity", "ids": ids}}
	}
	wantStatus(t, owner.do(http.MethodPut, base+"/order", order(second.Id, first.Id)), http.StatusNoContent)
	list := decode[[]httpapi.CustomField](t, owner.do(http.MethodGet, base, nil))
	if len(list) != 2 || list[0].Id != second.Id {
		t.Errorf("order = %+v", list)
	}
	wantStatus(t, owner.do(http.MethodPut, base+"/order", order(first.Id)), http.StatusUnprocessableEntity)
	wantStatus(t, stranger.do(http.MethodDelete, base+"/"+first.Id, nil), http.StatusNotFound)
	wantStatus(t, owner.do(http.MethodDelete, base+"/"+first.Id, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, base+"/"+first.Id, nil), http.StatusNotFound)
}
