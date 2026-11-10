package httpapi

import (
	"io"
	"net/http"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/connections"
	"github.com/adirkuhn/moneyharpy/internal/mcp"
	"github.com/adirkuhn/moneyharpy/internal/webhooks"
)

// GetConnectionSettings implements ServerInterface.
func (s *server) GetConnectionSettings(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	on, err := s.deps.Connections.UsersMayConnect(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ConnectionSettings{UsersMayConnect: on})
}

// UpdateConnectionSettings implements ServerInterface.
func (s *server) UpdateConnectionSettings(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[ConnectionSettings](w, r)
	if !ok {
		return
	}
	if err := s.deps.Connections.SetUsersMayConnect(r.Context(), admin, user(r).ID, body.UsersMayConnect); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func toConnection(c connections.Connection, me string) Connection {
	scopes := make([]Scope, len(c.Scopes))
	for i, p := range c.Scopes {
		scopes[i] = Scope(p)
	}
	return Connection{
		Id: c.ID.String(), Kind: ConnectionKind(c.Kind), Name: c.Name, Scopes: scopes, ReadOnly: c.ReadOnly, GrantedBy: c.GrantedBy,
		CreatedAt: c.CreatedAt, LastUsedAt: c.LastUsedAt, Mine: c.UserID.String() == me,
	}
}

// ListConnections implements ServerInterface.
func (s *server) ListConnections(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	list, err := s.deps.Connections.Connections(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]Connection, len(list))
	for i, c := range list {
		out[i] = toConnection(c, user(r).ID.String())
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateConnection implements ServerInterface. A connection token cannot
// make further connections.
func (s *server) CreateConnection(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	if _, viaToken := grantFrom(r.Context()); viaToken {
		writeError(w, http.StatusForbidden, "Connection tokens cannot create connections")
		return
	}
	body, ok := decodeBody[ConnectionRequest](w, r)
	if !ok {
		return
	}
	in := body.Connection
	scopes := make([]administration.Permission, len(in.Scopes))
	for i, p := range in.Scopes {
		scopes[i] = administration.Permission(p)
	}
	c, token, err := s.deps.Connections.Create(r.Context(), admin, user(r).ID, connections.NewConnection{Kind: string(in.Kind), Name: in.Name, Scopes: scopes, ReadOnly: deref(in.ReadOnly)})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c.GrantedBy = user(r).Name
	v := toConnection(c, user(r).ID.String())
	out := CreatedConnection{
		Id: v.Id, Kind: CreatedConnectionKind(v.Kind), Name: v.Name, Scopes: v.Scopes, ReadOnly: v.ReadOnly, GrantedBy: v.GrantedBy,
		CreatedAt: v.CreatedAt, LastUsedAt: v.LastUsedAt, Mine: true, Token: token,
	}
	if c.Kind == connections.KindAI {
		out.McpUrl = ptr(s.deps.BaseURL + "/api/mcp")
	}
	writeJSON(w, http.StatusCreated, out)
}

// RevokeConnection implements ServerInterface.
func (s *server) RevokeConnection(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, cid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Connections.Revoke(r.Context(), admin, cid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type webhookJSON struct {
	ID               string     `json:"id"`
	AdministrationID string     `json:"administration_id"`
	URL              string     `json:"url"`
	EnabledEvents    []string   `json:"enabled_events"`
	LastHTTPStatus   *int       `json:"last_http_status"`
	LastHTTPBody     *string    `json:"last_http_body"`
	Token            string     `json:"token"`
	Secret           string     `json:"secret,omitempty"`
	DeactivatedAt    *time.Time `json:"deactivated_at"`
	URI              string     `json:"uri"`
}

func toWebhook(h webhooks.Webhook) webhookJSON {
	return webhookJSON{
		ID: h.ID.String(), AdministrationID: h.AdministrationID.String(), URL: h.URL, EnabledEvents: h.Events, LastHTTPStatus: h.LastHTTPStatus,
		LastHTTPBody: h.LastHTTPBody, Token: h.Token, Secret: h.Secret, DeactivatedAt: h.DeactivatedAt, URI: h.Ref().URI(),
	}
}

// ListWebhooks implements ServerInterface.
func (s *server) ListWebhooks(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	list, err := s.deps.Webhooks.Webhooks(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]webhookJSON, len(list))
	for i, h := range list {
		out[i] = toWebhook(h)
	}
	writeJSON(w, http.StatusOK, out)
}

// ListWebhookEvents implements ServerInterface.
func (s *server) ListWebhookEvents(w http.ResponseWriter, _ *http.Request, _ AdministrationId) {
	writeJSON(w, http.StatusOK, WebhookEvents{Events: webhooks.Events, Groups: webhooks.Groups})
}

// CreateWebhook implements ServerInterface.
func (s *server) CreateWebhook(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[WebhookRequest](w, r)
	if !ok {
		return
	}
	in := webhooks.NewWebhook{URL: body.Url, Events: deref(body.EnabledEvents)}
	if g, viaToken := grantFrom(r.Context()); viaToken {
		in.ConnectionID = &g.ID
	}
	h, err := s.deps.Webhooks.Create(r.Context(), admin, user(r).ID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toWebhook(h))
}

// GetWebhook implements ServerInterface.
func (s *server) GetWebhook(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, hid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	h, err := s.deps.Webhooks.Webhook(r.Context(), admin, hid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toWebhook(h))
}

// DeleteWebhook implements ServerInterface.
func (s *server) DeleteWebhook(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, hid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Webhooks.Delete(r.Context(), admin, hid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) setWebhookActive(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, on bool) {
	admin, hid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	h, err := s.deps.Webhooks.SetActive(r.Context(), admin, hid, user(r).ID, on)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toWebhook(h))
}

// ActivateWebhook implements ServerInterface.
func (s *server) ActivateWebhook(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.setWebhookActive(w, r, administrationID, id, true)
}

// DeactivateWebhook implements ServerInterface.
func (s *server) DeactivateWebhook(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.setWebhookActive(w, r, administrationID, id, false)
}

// Mcp implements ServerInterface: the MCP endpoint for AI connections.
func (s *server) Mcp(w http.ResponseWriter, r *http.Request) {
	g, ok := grantFrom(r.Context())
	if !ok || g.Kind != connections.KindAI {
		writeError(w, http.StatusForbidden, "Use the token of an AI connection")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	server := mcp.Server{Name: "MoneyHarpy", Version: "1", Instructions: "Tools work in the administration this connection belongs to. Amounts are decimal strings; dates are YYYY-MM-DD."}
	status, out := server.Handle(r.Context(), body, &toolbox{s: s, grant: g, req: r})
	if out == nil {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, out)
}
