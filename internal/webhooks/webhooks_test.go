package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID       resource.ID = 100
	owner, viewer resource.ID = 1, 2
)

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID {
		return administration.Listed{}, administration.ErrNotFound
	}
	perms := administration.Permissions
	if user == viewer {
		perms = []administration.Permission{administration.Bank}
	}
	return administration.Listed{Membership: administration.Membership{Permissions: perms}}, nil
}

type memStore struct {
	hooks      map[resource.ID]Webhook
	deliveries map[resource.ID]Delivery
}

func (m *memStore) Webhooks(context.Context, resource.ID) ([]Webhook, error) {
	var out []Webhook
	for _, w := range m.hooks {
		out = append(out, w)
	}
	return out, nil
}
func (m *memStore) Webhook(_ context.Context, _, id resource.ID) (Webhook, error) {
	w, ok := m.hooks[id]
	if !ok {
		return Webhook{}, ErrNotFound
	}
	return w, nil
}
func (m *memStore) CreateWebhook(_ context.Context, w Webhook) error {
	m.hooks[w.ID] = w
	return nil
}
func (m *memStore) DeleteWebhook(_ context.Context, _, id resource.ID) error {
	delete(m.hooks, id)
	return nil
}
func (m *memStore) SetDeactivated(_ context.Context, _, id resource.ID, at *time.Time) error {
	w := m.hooks[id]
	w.DeactivatedAt = at
	m.hooks[id] = w
	return nil
}
func (m *memStore) Enqueue(_ context.Context, ds []Delivery) error {
	for _, d := range ds {
		m.deliveries[d.ID] = d
	}
	return nil
}
func (m *memStore) Due(_ context.Context, now, lease time.Time, limit int) ([]Delivery, error) {
	var out []Delivery
	for id, d := range m.deliveries {
		if d.NextAttemptAt != nil && !d.NextAttemptAt.After(now) && len(out) < limit {
			w := m.hooks[d.WebhookID]
			d.URL, d.Secret = w.URL, w.Secret
			d.NextAttemptAt = &lease
			m.deliveries[id] = d
			out = append(out, d)
		}
	}
	return out, nil
}
func (m *memStore) SaveAttempt(_ context.Context, d Delivery, r *Response) error {
	m.deliveries[d.ID] = d
	if r != nil {
		w := m.hooks[d.WebhookID]
		w.LastHTTPStatus, w.LastHTTPBody = &r.Status, &r.Body
		m.hooks[d.WebhookID] = w
	}
	return nil
}

type post struct {
	url     string
	headers map[string]string
	body    []byte
}

type fakeSender struct {
	sent   []post
	status int
	err    error
}

func (f *fakeSender) Post(_ context.Context, url string, h map[string]string, body []byte, _ bool) (Response, error) {
	f.sent = append(f.sent, post{url, h, body})
	return Response{Status: f.status, Body: "ok"}, f.err
}

type policy bool

func (p policy) WebhooksAllowLocal(context.Context) (bool, error) { return bool(p), nil }

func newTestService(allowLocal bool) (*Service, *memStore, *fakeSender, *time.Time) {
	store := &memStore{hooks: map[resource.ID]Webhook{}, deliveries: map[resource.ID]Delivery{}}
	sender := &fakeSender{status: 200}
	now := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	return NewService(fakeAdmins{}, store, sender, policy(allowLocal), resource.NewGenerator(1), func() time.Time { return now }), store, sender, &now
}

func fieldsOf(t *testing.T, err error) map[string][]string {
	t.Helper()
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error = %v, want *resource.ValidationError", err)
	}
	return verr.Fields
}

func TestCreateTestsTheURL(t *testing.T) {
	svc, store, sender, _ := newTestService(false)
	ctx := context.Background()
	w, err := svc.Create(ctx, adminID, owner, NewWebhook{URL: "https://example.com/hook", Events: []string{"tax_rate", "ledger_account_created", "tax_rate"}})
	if err != nil {
		t.Fatal(err)
	}
	if w.Secret == "" || w.Token == "" || len(w.Events) != 2 || len(sender.sent) != 1 || !strings.Contains(string(sender.sent[0].body), "test_webhook") {
		t.Errorf("Create() = %+v, sent %+v", w, sender.sent)
	}
	if list, _ := svc.Webhooks(ctx, adminID, owner); list[0].Secret != "" {
		t.Error("secret shown after creation")
	}
	sender.status = 404
	if fieldsOf(t, func() error {
		_, err := svc.Create(ctx, adminID, owner, NewWebhook{URL: "https://example.com/missing"})
		return err
	}())["url"] == nil {
		t.Error("accepted a URL that does not answer 200")
	}
	sender.status, sender.err = 200, errors.New("blocked address")
	if fieldsOf(t, func() error {
		_, err := svc.Create(ctx, adminID, owner, NewWebhook{URL: "https://10.0.0.1/"})
		return err
	}())["url"] == nil {
		t.Error("accepted an unreachable URL")
	}
	sender.err = nil
	_, err = svc.Create(ctx, adminID, owner, NewWebhook{URL: "http://example.com/", Events: []string{"spaceship_launched"}})
	fields := fieldsOf(t, err)
	if fields["url"] == nil || fields["enabled_events"] == nil {
		t.Errorf("fields = %v", fields)
	}
	if _, err := svc.Create(ctx, adminID, viewer, NewWebhook{URL: "https://example.com/"}); !errors.Is(err, ErrForbidden) {
		t.Errorf("viewer error = %v", err)
	}
	if len(store.hooks) != 1 {
		t.Errorf("stored %d webhooks", len(store.hooks))
	}
}

func TestPlainHTTPWhenAllowed(t *testing.T) {
	svc, _, _, _ := newTestService(true)
	if _, err := svc.Create(context.Background(), adminID, owner, NewWebhook{URL: "http://localhost:9000/hook"}); err != nil {
		t.Errorf("Create() = %v", err)
	}
}

func TestPublishAndDeliverSigned(t *testing.T) {
	svc, store, sender, now := newTestService(false)
	ctx := context.Background()
	w, _ := svc.Create(ctx, adminID, owner, NewWebhook{URL: "https://example.com/hook", Events: []string{"tax_rate"}})
	off, _ := svc.Create(ctx, adminID, owner, NewWebhook{URL: "https://example.com/off", Events: []string{"tax_rate"}})
	if _, err := svc.SetActive(ctx, adminID, off.ID, owner, false); err != nil {
		t.Fatal(err)
	}
	other, _ := svc.Create(ctx, adminID, owner, NewWebhook{URL: "https://example.com/other", Events: []string{"booking_rule"}})
	_ = other
	sender.sent = nil
	entity := json.RawMessage(`{"id":"5","name":"VAT 21%"}`)
	if err := svc.Publish(ctx, adminID, Event{EntityType: "TaxRate", EntityID: "5", Action: "tax_rate_created", Entity: entity}); err != nil {
		t.Fatal(err)
	}
	if len(store.deliveries) != 1 {
		t.Fatalf("queued %d deliveries, want 1", len(store.deliveries))
	}
	if n, err := svc.Deliver(ctx, 10); err != nil || n != 1 {
		t.Fatalf("Deliver() = %d, %v", n, err)
	}
	p := sender.sent[0]
	var payload map[string]any
	if err := json.Unmarshal(p.body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["action"] != "tax_rate_created" || payload["webhook_token"] != w.Token || payload["entity_id"] != "5" || payload["state"] != nil {
		t.Errorf("payload = %v", payload)
	}
	mac := hmac.New(sha256.New, []byte(w.Secret))
	_, _ = fmt.Fprintf(mac, "%d.%s", now.Unix(), p.body)
	if want := fmt.Sprintf("t=%d,v1=%s", now.Unix(), hex.EncodeToString(mac.Sum(nil))); p.headers["Moneybird-Signature"] != want {
		t.Errorf("signature = %s, want %s", p.headers["Moneybird-Signature"], want)
	}
	if p.headers["Idempotency-Key"] == "" {
		t.Error("no idempotency key")
	}
	for _, d := range store.deliveries {
		if d.DeliveredAt == nil || d.NextAttemptAt != nil {
			t.Errorf("delivery = %+v", d)
		}
	}
	if *store.hooks[w.ID].LastHTTPStatus != 200 {
		t.Error("last status not kept")
	}
}

func TestRetriesThenGivesUp(t *testing.T) {
	svc, store, sender, now := newTestService(false)
	ctx := context.Background()
	if _, err := svc.Create(ctx, adminID, owner, NewWebhook{URL: "https://example.com/hook", Events: []string{"user"}}); err != nil {
		t.Fatal(err)
	}
	sender.status = 500
	if err := svc.Publish(ctx, adminID, Event{EntityType: "User", EntityID: "1", Action: "user_removed", Entity: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if n, _ := svc.Deliver(ctx, 10); n != 1 {
			t.Fatalf("attempt %d delivered %d", attempt, n)
		}
		*now = now.Add(9 * time.Hour)
	}
	if n, _ := svc.Deliver(ctx, 10); n != 0 {
		t.Errorf("tried again after %d attempts", maxAttempts)
	}
	for _, d := range store.deliveries {
		if d.Attempts != maxAttempts || d.NextAttemptAt != nil || d.DeliveredAt != nil || *d.LastStatus != 500 {
			t.Errorf("delivery = %+v", d)
		}
	}
}

func TestSubscriptions(t *testing.T) {
	for _, c := range []struct {
		enabled []string
		event   string
		want    bool
	}{
		{[]string{"ledger_account"}, "ledger_account_booking_created", true},
		{[]string{"ledger_account_booking"}, "ledger_account_created", false},
		{[]string{"tax_rate_created"}, "tax_rate_created", true},
		{[]string{"tax"}, "tax_rate_created", true},
		{nil, "tax_rate_created", false},
	} {
		if got := subscribed(c.enabled, c.event); got != c.want {
			t.Errorf("subscribed(%v, %s) = %v", c.enabled, c.event, got)
		}
	}
	for _, e := range Events {
		// Moneybird lists no group for its credit invoice event.
		group := e == "credit_invoice_created_from_original"
		for _, g := range Groups {
			group = group || strings.HasPrefix(e, g+"_")
		}
		if !group {
			t.Errorf("%s belongs to no group", e)
		}
	}
}
