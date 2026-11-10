package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/connections"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/adirkuhn/moneyharpy/internal/webhooks"
)

func TestConnectionsRoundTrip(t *testing.T) {
	ctx := context.Background()
	a, _, owner := newAdministrationWithIdentity(t)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	store := postgres.NewConnectionStore(testPool)
	svc := connections.NewService(admins, store, ids, time.Now)

	c, token, err := svc.Create(ctx, a.ID, owner, connections.NewConnection{Kind: connections.KindAPI, Name: "Shop", Scopes: []administration.Permission{administration.Bank}, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	g, err := svc.Authenticate(ctx, token)
	if err != nil || g.ID != c.ID || g.AdministrationID != a.ID || !g.ReadOnly || g.UserEmail == "" || g.LastUsedAt == nil {
		t.Fatalf("Authenticate() = %+v, %v", g, err)
	}
	list, err := svc.Connections(ctx, a.ID, owner)
	if err != nil || len(list) != 1 || list[0].GrantedBy == "" {
		t.Errorf("Connections() = %+v, %v", list, err)
	}
	if err := svc.SetUsersMayConnect(ctx, a.ID, owner, true); err != nil {
		t.Fatal(err)
	}
	if on, _ := svc.UsersMayConnect(ctx, a.ID, owner); !on {
		t.Error("users may not connect")
	}
	// A user who left takes their connections' access with them.
	if _, err := testPool.Exec(ctx, "UPDATE administration_members SET active = false WHERE user_id = $1", int64(owner)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, connections.ErrInvalidToken) {
		t.Errorf("inactive user's token error = %v", err)
	}
	if _, err := testPool.Exec(ctx, "UPDATE administration_members SET active = true WHERE user_id = $1", int64(owner)); err != nil {
		t.Fatal(err)
	}
	if err := svc.Revoke(ctx, a.ID, c.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, connections.ErrInvalidToken) {
		t.Errorf("revoked token error = %v", err)
	}
}

type okSender struct{ status int }

func (s okSender) Post(context.Context, string, map[string]string, []byte, bool) (webhooks.Response, error) {
	return webhooks.Response{Status: s.status, Body: "fine"}, nil
}

type allowAll struct{}

func (allowAll) WebhooksAllowLocal(context.Context) (bool, error) { return true, nil }

func TestWebhooksRoundTrip(t *testing.T) {
	ctx := context.Background()
	a, _, owner := newAdministrationWithIdentity(t)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	store := postgres.NewWebhookStore(testPool, settings.NewCipher("test-secret"))
	svc := webhooks.NewService(admins, store, okSender{200}, allowAll{}, ids, time.Now)

	w, err := svc.Create(ctx, a.ID, owner, webhooks.NewWebhook{URL: "http://receiver.test/hook", Events: []string{"tax_rate"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Webhook(ctx, a.ID, w.ID, owner)
	if err != nil || got.URL != w.URL || got.Secret != "" || len(got.Events) != 1 {
		t.Fatalf("Webhook() = %+v, %v", got, err)
	}
	if err := svc.Publish(ctx, a.ID, webhooks.Event{EntityType: "TaxRate", EntityID: "1", Action: "tax_rate_created", Entity: json.RawMessage(`{"id":"1"}`)}); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.Deliver(ctx, 10); err != nil || n != 1 {
		t.Fatalf("Deliver() = %d, %v", n, err)
	}
	if n, _ := svc.Deliver(ctx, 10); n != 0 {
		t.Error("delivered twice")
	}
	if got, _ = svc.Webhook(ctx, a.ID, w.ID, owner); got.LastHTTPStatus == nil || *got.LastHTTPStatus != 200 || *got.LastHTTPBody != "fine" {
		t.Errorf("last response = %+v", got)
	}
	if got, err = svc.SetActive(ctx, a.ID, w.ID, owner, false); err != nil || got.DeactivatedAt == nil {
		t.Errorf("deactivate = %+v, %v", got, err)
	}
	if err := svc.Delete(ctx, a.ID, w.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Webhook(ctx, a.ID, w.ID, owner); !errors.Is(err, webhooks.ErrNotFound) {
		t.Errorf("deleted webhook error = %v", err)
	}
}
