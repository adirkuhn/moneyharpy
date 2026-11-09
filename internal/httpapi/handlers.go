package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/settings"
)

const (
	sessionMaxAge   = 30 * 24 * time.Hour
	oauthCookie     = "moneyharpy_oauth"
	oauthCookieAge  = 10 * time.Minute
	oauthCookiePath = "/api/auth/google"
	afterLoginPath  = "/administrations"
)

func decodeBody[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return v, false
	}
	return v, true
}

func user(r *http.Request) identity.User {
	u, ok := userFrom(r.Context())
	if !ok {
		// Unreachable: the contract rejects unauthenticated calls first.
		panic("handler reached without an authenticated user")
	}
	return u
}

func toAdministration(l administration.Listed) Administration {
	return Administration{
		Id:                l.ID.String(),
		Name:              l.Name,
		Language:          Language(l.Language),
		Currency:          l.Currency,
		Country:           l.Country,
		TimeZone:          l.TimeZone,
		Access:            AdministrationAccess(l.Access),
		Suspended:         l.Suspended,
		PeriodLockedUntil: l.PeriodLockedUntil,
		PeriodStartDate:   l.PeriodStartDate,
		Favorite:          l.Favorite,
		Uri:               l.Ref().URI(),
	}
}

func toCurrentUser(u identity.User) CurrentUser {
	return CurrentUser{Id: u.ID.String(), Email: u.Email, Name: u.Name, InstanceAdmin: u.InstanceAdmin}
}

// ListAdministrations implements ServerInterface.
func (s *server) ListAdministrations(w http.ResponseWriter, r *http.Request) {
	listed, err := s.deps.Administrations.List(r.Context(), user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]Administration, len(listed))
	for i, l := range listed {
		out[i] = toAdministration(l)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateAdministration implements ServerInterface.
func (s *server) CreateAdministration(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[CreateAdministrationRequest](w, r)
	if !ok {
		return
	}
	in := body.Administration
	created, err := s.deps.Administrations.Create(r.Context(), user(r).ID, administration.NewAdministration{
		Name:     in.Name,
		Language: string(deref(in.Language)),
		Currency: deref(in.Currency),
		Country:  deref(in.Country),
		TimeZone: deref(in.TimeZone),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAdministration(created))
}

// GetAdministration implements ServerInterface.
func (s *server) GetAdministration(w http.ResponseWriter, r *http.Request, id Id) {
	rid, ok := parseID(w, id)
	if !ok {
		return
	}
	got, err := s.deps.Administrations.Get(r.Context(), rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAdministration(got))
}

// FavoriteAdministration implements ServerInterface.
func (s *server) FavoriteAdministration(w http.ResponseWriter, r *http.Request, id Id) {
	s.setFavorite(w, r, id, true)
}

// UnfavoriteAdministration implements ServerInterface.
func (s *server) UnfavoriteAdministration(w http.ResponseWriter, r *http.Request, id Id) {
	s.setFavorite(w, r, id, false)
}

func (s *server) setFavorite(w http.ResponseWriter, r *http.Request, id Id, favorite bool) {
	rid, ok := parseID(w, id)
	if !ok {
		return
	}
	if err := s.deps.Administrations.SetFavorite(r.Context(), rid, user(r).ID, favorite); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LocateResource implements ServerInterface.
func (s *server) LocateResource(w http.ResponseWriter, r *http.Request, id Id) {
	rid, ok := parseID(w, id)
	if !ok {
		return
	}
	ref, err := s.deps.Resources.Locate(r.Context(), rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ResourceLocation{Id: ref.ID.String(), Type: string(ref.Type), Uri: ref.URI()})
}

// AuthProviders implements ServerInterface.
func (s *server) AuthProviders(w http.ResponseWriter, r *http.Request) {
	inst, err := s.deps.Settings.Instance(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, AuthProviders{RegistrationOpen: inst.RegistrationOpen, Google: inst.Google.Enabled})
}

// Register implements ServerInterface.
func (s *server) Register(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[RegisterRequest](w, r)
	if !ok {
		return
	}
	res, err := s.deps.Identity.Register(r.Context(), identity.Registration{Email: body.Email, Name: body.Name, Password: body.Password})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.setSession(w, res.SessionToken, sessionMaxAge)
	writeJSON(w, http.StatusCreated, toCurrentUser(res.User))
}

// Login implements ServerInterface.
func (s *server) Login(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[LoginRequest](w, r)
	if !ok {
		return
	}
	res, err := s.deps.Identity.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.setSession(w, res.SessionToken, sessionMaxAge)
	writeJSON(w, http.StatusOK, toCurrentUser(res.User))
}

// Logout implements ServerInterface.
func (s *server) Logout(w http.ResponseWriter, r *http.Request) {
	if token, ok := r.Context().Value(sessionKey).(string); ok {
		if err := s.deps.Identity.Logout(r.Context(), token); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.setSession(w, "", -time.Second)
	w.WriteHeader(http.StatusNoContent)
}

// CurrentUser implements ServerInterface.
func (s *server) CurrentUser(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, toCurrentUser(user(r)))
}

// GoogleStart implements ServerInterface.
func (s *server) GoogleStart(w http.ResponseWriter, r *http.Request) {
	inst, err := s.deps.Settings.Instance(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !inst.Google.Enabled {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	state, nonce := rand.Text(), rand.Text()
	target, err := s.deps.Google.AuthURL(r.Context(), state, nonce)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.setOAuthCookie(w, state+"."+nonce, oauthCookieAge)
	http.Redirect(w, r, target, http.StatusFound)
}

// GoogleCallback implements ServerInterface.
func (s *server) GoogleCallback(w http.ResponseWriter, r *http.Request, params GoogleCallbackParams) {
	s.setOAuthCookie(w, "", -time.Second)

	state, nonce, ok := s.oauthChallenge(r)
	if !ok || params.Error != nil || params.Code == nil || params.State == nil ||
		subtle.ConstantTimeCompare([]byte(*params.State), []byte(state)) != 1 {
		http.Redirect(w, r, "/login?error=google", http.StatusFound)
		return
	}

	external, err := s.deps.Google.Exchange(r.Context(), *params.Code, nonce)
	if err != nil {
		s.deps.Logger.WarnContext(r.Context(), "google exchange failed", "error", err)
		http.Redirect(w, r, "/login?error=google", http.StatusFound)
		return
	}
	res, err := s.deps.Identity.LoginExternal(r.Context(), external)
	switch {
	case errors.Is(err, identity.ErrRegistrationClosed):
		http.Redirect(w, r, "/login?error=registration_closed", http.StatusFound)
		return
	case errors.Is(err, identity.ErrEmailTaken):
		http.Redirect(w, r, "/login?error=email_taken", http.StatusFound)
		return
	case err != nil:
		s.deps.Logger.ErrorContext(r.Context(), "google login failed", "error", err)
		http.Redirect(w, r, "/login?error=google", http.StatusFound)
		return
	}
	s.setSession(w, res.SessionToken, sessionMaxAge)
	http.Redirect(w, r, afterLoginPath, http.StatusFound)
}

func (s *server) oauthChallenge(r *http.Request) (state, nonce string, ok bool) {
	c, err := r.Cookie(oauthCookie)
	if err != nil {
		return "", "", false
	}
	return strings.Cut(c.Value, ".")
}

func (s *server) setOAuthCookie(w http.ResponseWriter, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthCookie,
		Value:    value,
		Path:     oauthCookiePath,
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func toAPIToken(t identity.Token) ApiToken {
	return ApiToken{Id: t.ID.String(), Name: t.Name, CreatedAt: t.CreatedAt}
}

// ListTokens implements ServerInterface.
func (s *server) ListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.deps.Identity.Tokens(r.Context(), user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]ApiToken, len(tokens))
	for i, t := range tokens {
		out[i] = toAPIToken(t)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateToken implements ServerInterface.
func (s *server) CreateToken(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[CreateTokenRequest](w, r)
	if !ok {
		return
	}
	t, secret, err := s.deps.Identity.CreateToken(r.Context(), user(r).ID, body.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, createdToken{ApiToken: toAPIToken(t), Token: secret})
}

// createdToken is the wire form of CreatedApiToken.
type createdToken struct {
	ApiToken
	Token string `json:"token"`
}

// DeleteToken implements ServerInterface.
func (s *server) DeleteToken(w http.ResponseWriter, r *http.Request, id Id) {
	rid, ok := parseID(w, id)
	if !ok {
		return
	}
	if err := s.deps.Identity.DeleteToken(r.Context(), rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toInstanceSettings(inst settings.Instance) InstanceSettings {
	return InstanceSettings{
		RegistrationOpen:      inst.RegistrationOpen,
		GoogleEnabled:         inst.Google.Enabled,
		GoogleClientId:        inst.Google.ClientID,
		GoogleClientSecretSet: inst.Google.ClientSecret != "",
	}
}

func requireInstanceAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !user(r).InstanceAdmin {
		writeError(w, http.StatusForbidden, "Instance administrators only")
		return false
	}
	return true
}

// InstanceSettings implements ServerInterface.
func (s *server) InstanceSettings(w http.ResponseWriter, r *http.Request) {
	if !requireInstanceAdmin(w, r) {
		return
	}
	inst, err := s.deps.Settings.Instance(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toInstanceSettings(inst))
}

// UpdateInstanceSettings implements ServerInterface.
func (s *server) UpdateInstanceSettings(w http.ResponseWriter, r *http.Request) {
	if !requireInstanceAdmin(w, r) {
		return
	}
	body, ok := decodeBody[UpdateInstanceSettingsRequest](w, r)
	if !ok {
		return
	}
	inst, err := s.deps.Settings.Update(r.Context(), settings.Update{
		RegistrationOpen:   body.RegistrationOpen,
		GoogleEnabled:      body.GoogleEnabled,
		GoogleClientID:     body.GoogleClientId,
		GoogleClientSecret: body.GoogleClientSecret,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toInstanceSettings(inst))
}

// serveWeb serves the web application: real files where they exist, and the
// single-page entry point for every other path, so any URI can be opened.
func (s *server) serveWeb(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	for _, candidate := range []string{name, path.Join(name, "index.html")} {
		if info, err := fs.Stat(s.deps.Web, candidate); candidate != "" && err == nil && !info.IsDir() {
			s.serveFile(w, r, candidate)
			return
		}
	}
	s.serveFile(w, r, "index.html")
}

func (s *server) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	content, err := fs.ReadFile(s.deps.Web, name)
	if errors.Is(err, fs.ErrNotExist) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if err != nil {
		s.fail(w, r, fmt.Errorf("read %s: %w", name, err))
		return
	}
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, name, time.Time{}, strings.NewReader(string(content)))
}

func parseID(w http.ResponseWriter, id Id) (resource.ID, bool) {
	rid, err := resource.ParseID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not found")
		return 0, false
	}
	return rid, true
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
