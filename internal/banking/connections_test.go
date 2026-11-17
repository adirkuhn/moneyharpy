package banking

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/bankfile"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const callback = "https://books.example.com/bank_connection_callback"

// fakeLink stands in for Enable Banking.
type fakeLink struct {
	mu        sync.Mutex
	off       bool
	authorize []LinkAuthorization
	sessions  map[string]LinkSession
	entries   map[string][]bankfile.Entry
	fetched   []string
	fetchErr  error
	closed    []string
}

func (f *fakeLink) Link(context.Context) (BankLink, error) {
	if f.off {
		return nil, ErrLinkOff
	}
	return f, nil
}

func (f *fakeLink) Banks(_ context.Context, country string) ([]LinkBank, error) {
	return []LinkBank{{Name: "Example Bank", Country: country, Logo: "https://example.com/bank.png", MaxConsent: 180 * 24 * time.Hour}}, nil
}

func (f *fakeLink) Authorize(_ context.Context, a LinkAuthorization) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authorize = append(f.authorize, a)
	return "https://bank.example.com/login?state=" + a.State, nil
}

func (f *fakeLink) Session(_ context.Context, code string) (LinkSession, error) {
	s, ok := f.sessions[code]
	if !ok {
		return LinkSession{}, errors.New("unknown code")
	}
	return s, nil
}

func (f *fakeLink) Transactions(_ context.Context, uid string, from time.Time) ([]bankfile.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetched = append(f.fetched, uid+"@"+from.Format(time.DateOnly))
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	return f.entries[uid], nil
}

func (f *fakeLink) Close(_ context.Context, session string) error {
	f.closed = append(f.closed, session)
	return nil
}

func connectedService(t *testing.T) (*Service, *fakeLink) {
	t.Helper()
	svc, _, _ := newTestService()
	link := &fakeLink{sessions: map[string]LinkSession{}, entries: map[string][]bankfile.Entry{}}
	svc.UseBankLinks(link, newMemConnections(), callback)
	return svc, link
}

// stateOf is the state the last authorisation sent to the bank.
func (f *fakeLink) stateOf(t *testing.T) string {
	t.Helper()
	if len(f.authorize) == 0 {
		t.Fatal("no authorisation was started")
	}
	return f.authorize[len(f.authorize)-1].State
}

func TestConnectingABankBooksItsAccountsTransactions(t *testing.T) {
	svc, link := connectedService(t)
	ctx := context.Background()
	known := bank(t, svc) // NL91ABNA0417164300

	url, c, err := svc.StartConnection(ctx, adminID, owner, ConnectionRequest{Bank: "Example Bank", Country: "NL"})
	if err != nil {
		t.Fatal(err)
	}
	a := link.authorize[0]
	if a.Bank != "Example Bank" || a.Country != "NL" || a.RedirectURL != callback || len(a.State) < 32 || !strings.HasSuffix(url, a.State) || !a.ValidUntil.Equal(now.Add(180*24*time.Hour)) {
		t.Errorf("authorisation = %+v, url %q", a, url)
	}
	if c.State != ConnectionPending {
		t.Errorf("connection = %+v, want pending", c)
	}

	link.sessions["code-1"] = LinkSession{ID: "session-1", ValidUntil: now.Add(180 * 24 * time.Hour), Accounts: []LinkAccount{
		{UID: "uid-known", IBAN: "NL91ABNA0417164300", Currency: "EUR", Name: "Business"},
		{UID: "uid-new", IBAN: "NL20INGB0001234567", Currency: "EUR", Name: "Savings"},
	}}
	link.entries["uid-known"] = []bankfile.Entry{{Date: now.AddDate(0, 0, -2), Amount: 25000, Currency: "EUR", Message: "Invoice 2026-0001", ContraName: "Example Client B.V.", TransactionID: "E1"}}
	link.entries["uid-new"] = []bankfile.Entry{{Date: now.AddDate(0, 0, -1), Amount: -2550, Currency: "EUR", Message: "Monthly hosting plan", TransactionID: "E2"}}

	done, err := svc.FinishConnection(ctx, owner, link.stateOf(t), "code-1")
	if err != nil {
		t.Fatal(err)
	}
	if done.State != ConnectionActive || done.SessionID != "session-1" || done.LastSyncAt == nil || done.LastError != "" || len(done.Accounts) != 2 {
		t.Fatalf("finished = %+v", done)
	}
	if done.Accounts[0].AccountID != known.ID {
		t.Errorf("first account linked to %v, want the account with its IBAN", done.Accounts[0].AccountID)
	}
	accounts, _ := svc.Accounts(ctx, adminID, owner)
	var savings *Account
	for i := range accounts {
		if accounts[i].Identifier == "NL20INGB0001234567" {
			savings = &accounts[i]
		}
	}
	if savings == nil || savings.Name != "Savings" || savings.Provider != ProviderEnableBanking {
		t.Fatalf("new account = %+v", savings)
	}
	for acc, want := range map[resource.ID]string{known.ID: "Invoice 2026-0001", savings.ID: "Monthly hosting plan"} {
		ms, _ := svc.Mutations(ctx, adminID, owner, MutationFilter{MutationQuery: MutationQuery{AccountID: &acc}})
		if len(ms) != 1 || ms[0].Message != want {
			t.Errorf("account %v has %+v, want %q", acc, ms, want)
		}
	}
	// The first fetch reaches 90 days back.
	if link.fetched[0] != "uid-known@"+now.AddDate(0, 0, -90).Format(time.DateOnly) {
		t.Errorf("fetched = %v", link.fetched)
	}
	if _, err := svc.FinishConnection(ctx, owner, link.stateOf(t), "code-1"); !errors.Is(err, ErrConnectionNotFound) {
		t.Errorf("finishing twice = %v, want the state used up", err)
	}
}

func TestSyncFetchesFromTheLastFetchAndSkipsWhatItHas(t *testing.T) {
	svc, link := connectedService(t)
	ctx := context.Background()
	c := connect(t, svc, link)
	link.fetched = nil
	later := now.Add(7 * time.Hour)
	svc.now = func() time.Time { return later }

	synced, err := svc.SyncConnection(ctx, adminID, owner, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	// From a week before the last fetch, as banks book late; what is known is skipped.
	if link.fetched[0] != "uid-1@"+now.AddDate(0, 0, -7).Format(time.DateOnly) || !synced.LastSyncAt.Equal(later) {
		t.Errorf("fetched = %v, synced %+v", link.fetched, synced)
	}
	ms, _ := svc.Mutations(ctx, adminID, owner, MutationFilter{})
	if len(ms) != 1 {
		t.Errorf("mutations = %d, want the one transaction once", len(ms))
	}
}

func TestAnExpiredConsentAsksToReconnect(t *testing.T) {
	svc, link := connectedService(t)
	ctx := context.Background()
	c := connect(t, svc, link)

	link.fetchErr = ErrLinkExpired
	got, err := svc.SyncConnection(ctx, adminID, owner, c.ID)
	if err != nil || got.State != ConnectionExpired || got.LastError == "" {
		t.Fatalf("sync = %+v, %v; want expired", got, err)
	}

	// Reconnecting goes to the same bank in one step, and keeps the accounts.
	url, err := svc.Reconnect(ctx, adminID, owner, c.ID)
	if err != nil || !strings.Contains(url, "bank.example.com") {
		t.Fatalf("Reconnect() = %q, %v", url, err)
	}
	if a := link.authorize[len(link.authorize)-1]; a.Bank != "Example Bank" || a.Country != "NL" {
		t.Errorf("reconnect authorisation = %+v", a)
	}
	link.fetchErr = nil
	link.sessions["code-2"] = LinkSession{ID: "session-2", ValidUntil: now.Add(180 * 24 * time.Hour), Accounts: []LinkAccount{{UID: "uid-2", IBAN: "NL91ABNA0417164300", Currency: "EUR"}}}
	again, err := svc.FinishConnection(ctx, owner, link.stateOf(t), "code-2")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != c.ID || again.State != ConnectionActive || again.SessionID != "session-2" || again.Accounts[0].UID != "uid-2" || again.Accounts[0].AccountID != c.Accounts[0].AccountID {
		t.Errorf("reconnected = %+v", again)
	}
	if len(link.closed) != 1 || link.closed[0] != "session-1" {
		t.Errorf("closed = %v, want the old session closed", link.closed)
	}
	list, _ := svc.Connections(ctx, adminID, owner)
	if len(list) != 1 {
		t.Errorf("connections = %+v, want the one connection", list)
	}
}

func TestConnectionsDueForSyncAndExpiring(t *testing.T) {
	svc, link := connectedService(t)
	ctx := context.Background()
	c := connect(t, svc, link)

	svc.now = func() time.Time { return now.Add(23 * time.Hour) }
	if n := svc.SyncDue(ctx); n != 0 {
		t.Errorf("synced %d within a day", n)
	}
	svc.now = func() time.Time { return now.Add(25 * time.Hour) }
	if n := svc.SyncDue(ctx); n != 1 {
		t.Errorf("synced %d after a day, want 1", n)
	}
	// Past the consent, the connection expires without asking the bank.
	link.fetched = nil
	svc.now = func() time.Time { return now.Add(181 * 24 * time.Hour) }
	svc.SyncDue(ctx)
	got, _ := svc.Connection(ctx, adminID, owner, c.ID)
	if got.State != ConnectionExpired || len(link.fetched) != 0 {
		t.Errorf("after the consent: %+v, fetched %v", got, link.fetched)
	}
}

func TestRateLimitsAndErrorsKeepTheConnection(t *testing.T) {
	svc, link := connectedService(t)
	ctx := context.Background()
	c := connect(t, svc, link)
	link.fetchErr = ErrLinkRateLimited
	got, err := svc.SyncConnection(ctx, adminID, owner, c.ID)
	if err != nil || got.State != ConnectionActive || !strings.Contains(got.LastError, "later") {
		t.Errorf("rate limited = %+v, %v", got, err)
	}
	link.fetchErr = errors.New("bank unavailable")
	got, _ = svc.SyncConnection(ctx, adminID, owner, c.ID)
	if got.State != ConnectionActive || got.LastError != "bank unavailable" {
		t.Errorf("bank error = %+v", got)
	}
}

func TestDisconnectClosesTheSessionAndKeepsTheTransactions(t *testing.T) {
	svc, link := connectedService(t)
	ctx := context.Background()
	c := connect(t, svc, link)
	if err := svc.Disconnect(ctx, adminID, owner, c.ID); err != nil {
		t.Fatal(err)
	}
	if len(link.closed) != 1 {
		t.Errorf("closed = %v", link.closed)
	}
	if list, _ := svc.Connections(ctx, adminID, owner); len(list) != 0 {
		t.Errorf("connections = %+v", list)
	}
	if ms, _ := svc.Mutations(ctx, adminID, owner, MutationFilter{}); len(ms) != 1 {
		t.Errorf("mutations = %d, want them kept", len(ms))
	}
}

func TestConnectionsNeedTheSettingAndKnownBanks(t *testing.T) {
	svc, link := connectedService(t)
	ctx := context.Background()
	link.off = true
	if _, _, err := svc.StartConnection(ctx, adminID, owner, ConnectionRequest{Bank: "Example Bank", Country: "NL"}); !errors.Is(err, ErrLinkOff) {
		t.Errorf("off = %v", err)
	}
	link.off = false
	if _, _, err := svc.StartConnection(ctx, adminID, owner, ConnectionRequest{Bank: "Other Bank", Country: "NL"}); fieldsOf(t, err)["bank"] == nil {
		t.Error("started a connection with an unknown bank")
	}
	if _, err := svc.FinishConnection(ctx, owner, "no-such-state", "code"); !errors.Is(err, ErrConnectionNotFound) {
		t.Errorf("unknown state = %v", err)
	}
	// A session that shares no accounts tells the user to link them in Enable Banking.
	_, _, _ = svc.StartConnection(ctx, adminID, owner, ConnectionRequest{Bank: "Example Bank", Country: "NL"})
	link.sessions["empty"] = LinkSession{ID: "s", ValidUntil: now.Add(time.Hour)}
	if _, err := svc.FinishConnection(ctx, owner, link.stateOf(t), "empty"); fieldsOf(t, err)["accounts"] == nil {
		t.Errorf("empty session = %v", err)
	}
}

// connect connects one account with one booked transaction.
func connect(t *testing.T, svc *Service, link *fakeLink) Connection {
	t.Helper()
	ctx := context.Background()
	if _, _, err := svc.StartConnection(ctx, adminID, owner, ConnectionRequest{Bank: "Example Bank", Country: "NL"}); err != nil {
		t.Fatal(err)
	}
	link.sessions["code-1"] = LinkSession{ID: "session-1", ValidUntil: now.Add(180 * 24 * time.Hour), Accounts: []LinkAccount{{UID: "uid-1", IBAN: "NL91ABNA0417164300", Currency: "EUR"}}}
	link.entries["uid-1"] = []bankfile.Entry{{Date: now.AddDate(0, 0, -2), Amount: 25000, Currency: "EUR", Message: "Invoice 2026-0001", TransactionID: "E1"}}
	c, err := svc.FinishConnection(ctx, owner, link.stateOf(t), "code-1")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// memConnections keeps connections in memory.
type memConnections struct {
	mu    sync.Mutex
	conns map[resource.ID]Connection
}

func newMemConnections() *memConnections { return &memConnections{conns: map[resource.ID]Connection{}} }

func (m *memConnections) Connections(_ context.Context, admin resource.ID) ([]Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Connection
	for _, c := range m.conns {
		if c.AdministrationID == admin {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *memConnections) Connection(_ context.Context, admin, id resource.ID) (Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.conns[id]
	if !ok || c.AdministrationID != admin {
		return Connection{}, ErrConnectionNotFound
	}
	return c, nil
}

func (m *memConnections) ConnectionByAuthState(_ context.Context, state string) (Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.conns {
		if c.AuthState != "" && c.AuthState == state {
			return c, nil
		}
	}
	return Connection{}, ErrConnectionNotFound
}

func (m *memConnections) SaveConnection(_ context.Context, c Connection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conns[c.ID] = c
	return nil
}

func (m *memConnections) DeleteConnection(_ context.Context, admin, id resource.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.conns, id)
	return nil
}

func (m *memConnections) DueConnections(_ context.Context, before time.Time) ([]Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Connection
	for _, c := range m.conns {
		if c.State == ConnectionActive && (c.LastSyncAt == nil || c.LastSyncAt.Before(before)) {
			out = append(out, c)
		}
	}
	return out, nil
}

// fakeKeyLink stands in for Revolut's Business API.
type fakeKeyLink struct {
	accounts map[string][]LinkAccount // by key
	entries  map[string][]bankfile.Entry
	err      error
	fetched  []string
}

func (f *fakeKeyLink) Accounts(_ context.Context, provider, key string) ([]LinkAccount, error) {
	if provider != ProviderRevolut {
		return nil, errors.New("unknown provider")
	}
	if f.err != nil {
		return nil, f.err
	}
	a, ok := f.accounts[key]
	if !ok {
		return nil, ErrLinkExpired
	}
	return a, nil
}

func (f *fakeKeyLink) Transactions(_ context.Context, _, key, uid string, from time.Time) ([]bankfile.Entry, error) {
	f.fetched = append(f.fetched, key+"/"+uid+"@"+from.Format(time.DateOnly))
	if f.err != nil {
		return nil, f.err
	}
	return f.entries[uid], nil
}

// sealer marks sealed secrets so tests see they are never stored plain.
type sealer struct{}

func (sealer) Seal(s string) (string, error) { return "sealed:" + s, nil }
func (sealer) Open(s string) (string, error) {
	plain, ok := strings.CutPrefix(s, "sealed:")
	if !ok {
		return "", errors.New("not sealed")
	}
	return plain, nil
}

func keyService(t *testing.T) (*Service, *fakeKeyLink, *memConnections) {
	t.Helper()
	svc, _, _ := newTestService()
	keys := &fakeKeyLink{accounts: map[string][]LinkAccount{
		"key-1": {{UID: "acc-eur", IBAN: "NL91ABNA0417164300", Currency: "EUR", Name: "Main"}, {UID: "acc-gbp", Currency: "GBP", Name: "GBP"}},
		"key-2": {{UID: "acc-eur", IBAN: "NL91ABNA0417164300", Currency: "EUR", Name: "Main"}},
	}, entries: map[string][]bankfile.Entry{"acc-eur": {{Date: now.AddDate(0, 0, -1), Amount: -2570, Currency: "EUR", Message: "INV-2026-77", ContraName: "Hosting Company", TransactionID: "tx-2"}}}}
	store := newMemConnections()
	svc.UseBankLinks(nil, store, callback)
	svc.UseKeyLinks(keys, sealer{})
	return svc, keys, store
}

func TestConnectingRevolutWithAnAPIKey(t *testing.T) {
	svc, keys, store := keyService(t)
	ctx := context.Background()
	c, err := svc.ConnectWithKey(ctx, adminID, owner, ProviderRevolut, " key-1 ")
	if err != nil {
		t.Fatal(err)
	}
	if c.Provider != ProviderRevolut || c.Bank != "Revolut Business" || c.State != ConnectionActive || c.ValidUntil != nil || c.LastSyncAt == nil || len(c.Accounts) != 1 {
		t.Fatalf("connection = %+v; want the account with an IBAN connected and fetched", c)
	}
	stored, _ := store.Connection(ctx, adminID, c.ID)
	if stored.Secret != "sealed:key-1" {
		t.Errorf("stored secret = %q, want it sealed", stored.Secret)
	}
	if keys.fetched[0] != "key-1/acc-eur@"+now.AddDate(0, 0, -90).Format(time.DateOnly) {
		t.Errorf("fetched = %v", keys.fetched)
	}
	ms, _ := svc.Mutations(ctx, adminID, owner, MutationFilter{})
	if len(ms) != 1 || ms[0].Message != "INV-2026-77" {
		t.Errorf("mutations = %+v", ms)
	}
	accounts, _ := svc.Accounts(ctx, adminID, owner)
	if !slicesContain(accounts, func(a Account) bool { return a.Identifier == "NL91ABNA0417164300" && a.Provider == ProviderRevolut }) {
		t.Errorf("accounts = %+v", accounts)
	}
	if _, err := svc.Reconnect(ctx, adminID, owner, c.ID); fieldsOf(t, err)["api_key"] == nil {
		t.Errorf("reconnect = %v, want to be asked for a new key", err)
	}
}

func TestARejectedKeyAsksForANewOne(t *testing.T) {
	svc, keys, _ := keyService(t)
	ctx := context.Background()
	if _, err := svc.ConnectWithKey(ctx, adminID, owner, ProviderRevolut, "wrong"); fieldsOf(t, err)["api_key"] == nil {
		t.Errorf("wrong key = %v", err)
	}
	if _, err := svc.ConnectWithKey(ctx, adminID, owner, "other", "key-1"); fieldsOf(t, err)["provider"] == nil {
		t.Errorf("unknown provider = %v", err)
	}
	c, _ := svc.ConnectWithKey(ctx, adminID, owner, ProviderRevolut, "key-1")
	keys.err = ErrLinkExpired
	got, err := svc.SyncConnection(ctx, adminID, owner, c.ID)
	if err != nil || got.State != ConnectionExpired {
		t.Fatalf("revoked key = %+v, %v", got, err)
	}
	keys.err = nil
	again, err := svc.ReplaceKey(ctx, adminID, owner, c.ID, "key-2")
	if err != nil || again.ID != c.ID || again.State != ConnectionActive || again.LastError != "" {
		t.Fatalf("replaced = %+v, %v", again, err)
	}
	if got, _ := svc.Connection(ctx, adminID, owner, c.ID); got.Secret != "sealed:key-2" {
		t.Errorf("secret = %q", got.Secret)
	}
	// Due fetches use the key too.
	keys.fetched = nil
	svc.now = func() time.Time { return now.Add(25 * time.Hour) }
	if n := svc.SyncDue(ctx); n != 1 || len(keys.fetched) != 1 || !strings.HasPrefix(keys.fetched[0], "key-2/") {
		t.Errorf("due = %d, fetched %v", n, keys.fetched)
	}
}

func slicesContain[T any](list []T, f func(T) bool) bool {
	for _, x := range list {
		if f(x) {
			return true
		}
	}
	return false
}
