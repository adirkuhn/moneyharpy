package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/banking"
)

// reconnectWithin is how long before a consent ends the user is asked to
// reconnect.
const reconnectWithin = 14 * 24 * time.Hour

type bankConnectionJSON struct {
	ID                  string     `json:"id"`
	AdministrationID    string     `json:"administration_id"`
	Provider            string     `json:"provider"`
	Bank                string     `json:"bank"`
	Country             string     `json:"country"`
	State               string     `json:"state"`
	ValidUntil          *time.Time `json:"valid_until"`
	NeedsReconnect      bool       `json:"needs_reconnect"`
	LastSyncAt          *time.Time `json:"last_sync_at"`
	LastError           string     `json:"last_error"`
	FinancialAccountIDs []string   `json:"financial_account_ids"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func toBankConnection(c banking.Connection, now time.Time) bankConnectionJSON {
	out := bankConnectionJSON{
		ID: c.ID.String(), AdministrationID: c.AdministrationID.String(), Provider: c.Provider, Bank: c.Bank, Country: c.Country, State: c.State,
		ValidUntil: c.ValidUntil, LastSyncAt: c.LastSyncAt, LastError: c.LastError, FinancialAccountIDs: []string{}, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		NeedsReconnect: c.State == banking.ConnectionExpired || (c.ValidUntil != nil && c.ValidUntil.Sub(now) < reconnectWithin),
	}
	for _, a := range c.Accounts {
		out.FinancialAccountIDs = append(out.FinancialAccountIDs, a.AccountID.String())
	}
	return out
}

type bankAuthorizationJSON struct {
	AuthorizationURL string `json:"authorization_url"`
	BankConnectionID string `json:"bank_connection_id"`
}

// ListBankConnections implements ServerInterface.
func (s *server) ListBankConnections(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	list, err := s.deps.Banking.Connections(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]bankConnectionJSON, 0, len(list))
	for _, c := range list {
		out = append(out, toBankConnection(c, time.Now()))
	}
	writeJSON(w, http.StatusOK, out)
}

// ListConnectableBanks implements ServerInterface.
func (s *server) ListConnectableBanks(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListConnectableBanksParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	country := "NL"
	if params.Country != nil {
		country = *params.Country
	}
	banks, err := s.deps.Banking.Banks(r.Context(), admin, user(r).ID, country)
	if err != nil {
		s.failBankLink(w, r, err)
		return
	}
	out := make([]ConnectableBank, 0, len(banks))
	for _, b := range banks {
		out = append(out, ConnectableBank{Name: b.Name, Country: b.Country, Logo: b.Logo})
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateBankConnection implements ServerInterface.
func (s *server) CreateBankConnection(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[CreateBankConnectionJSONRequestBody](w, r)
	if !ok {
		return
	}
	url, c, err := s.deps.Banking.StartConnection(r.Context(), admin, user(r).ID, banking.ConnectionRequest{Bank: body.Bank, Country: body.Country})
	if err != nil {
		s.failBankLink(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, bankAuthorizationJSON{AuthorizationURL: url, BankConnectionID: c.ID.String()})
}

// ReconnectBankConnection implements ServerInterface.
func (s *server) ReconnectBankConnection(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	url, err := s.deps.Banking.Reconnect(r.Context(), admin, user(r).ID, rid)
	if err != nil {
		s.failBankLink(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, bankAuthorizationJSON{AuthorizationURL: url, BankConnectionID: rid.String()})
}

// SyncBankConnection implements ServerInterface.
func (s *server) SyncBankConnection(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	c, err := s.deps.Banking.SyncConnection(r.Context(), admin, user(r).ID, rid)
	if err != nil {
		s.failBankLink(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBankConnection(c, time.Now()))
}

// DeleteBankConnection implements ServerInterface.
func (s *server) DeleteBankConnection(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Banking.Disconnect(r.Context(), admin, user(r).ID, rid); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CompleteBankConnection implements ServerInterface.
func (s *server) CompleteBankConnection(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[CompleteBankConnectionJSONRequestBody](w, r)
	if !ok {
		return
	}
	c, err := s.deps.Banking.FinishConnection(r.Context(), user(r).ID, body.State, body.Code)
	if err != nil {
		s.failBankLink(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBankConnection(c, time.Now()))
}

// failBankLink shows what the bank or Enable Banking answered, as the user
// can often act on it, such as linking accounts in the control panel.
func (s *server) failBankLink(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, banking.ErrLinkFailed) || errors.Is(err, banking.ErrLinkExpired) || errors.Is(err, banking.ErrLinkRateLimited) {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.fail(w, r, err)
}
