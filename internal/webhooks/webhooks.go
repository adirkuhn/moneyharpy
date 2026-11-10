// Package webhooks notifies other systems of changes in an administration,
// after Moneybird's webhooks: subscriptions to events, signed deliveries
// and retries until the receiver answers 200.
package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Errors returned by Service.
var (
	ErrNotFound = errors.New("webhook not found")
	// ErrForbidden reports a user without the settings permission.
	ErrForbidden = errors.New("not allowed")
)

// maxAttempts is how often a delivery is tried, as at Moneybird.
const maxAttempts = 10

// backoff is the wait before each retry.
var backoff = []time.Duration{time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour, 4 * time.Hour, 8 * time.Hour}

// Webhook sends events of one administration to a URL.
type Webhook struct {
	ID               resource.ID
	AdministrationID resource.ID
	URL              string
	Events           []string
	// Token is sent with every delivery so receivers can tell webhooks apart.
	Token string
	// Secret signs deliveries; shown only when the webhook is created.
	Secret string
	// ConnectionID is the connection that made the webhook, if one did; the
	// webhook goes when the connection is revoked.
	ConnectionID   *resource.ID
	LastHTTPStatus *int
	LastHTTPBody   *string
	DeactivatedAt  *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Ref locates the webhook.
func (w Webhook) Ref() resource.Ref {
	return resource.Ref{ID: w.ID, Type: resource.TypeWebhook, AdministrationID: w.AdministrationID}
}

// Event is a change to send.
type Event struct {
	EntityType string
	EntityID   string
	Action     string
	State      string
	// Entity is the resource as the API shows it.
	Entity json.RawMessage
}

// Delivery is an event on its way to one webhook.
type Delivery struct {
	ID             resource.ID
	WebhookID      resource.ID
	IdempotencyKey string
	Payload        json.RawMessage
	Attempts       int
	NextAttemptAt  *time.Time
	DeliveredAt    *time.Time
	LastStatus     *int
	LastError      string
	CreatedAt      time.Time
	// URL and Secret come from the webhook, for sending.
	URL, Secret string
}

// Response is what a receiver answered.
type Response struct {
	Status int
	Body   string
}

// Sender posts a delivery. It refuses private and loopback addresses unless
// allowLocal.
type Sender interface {
	Post(ctx context.Context, url string, headers map[string]string, body []byte, allowLocal bool) (Response, error)
}

// Policy tells whether webhooks may reach plain HTTP and local addresses.
type Policy interface {
	WebhooksAllowLocal(ctx context.Context) (bool, error)
}

// Administrations gives access to administrations.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Store persists webhooks and their deliveries.
type Store interface {
	Webhooks(ctx context.Context, admin resource.ID) ([]Webhook, error)
	Webhook(ctx context.Context, admin, id resource.ID) (Webhook, error)
	CreateWebhook(ctx context.Context, w Webhook) error
	DeleteWebhook(ctx context.Context, admin, id resource.ID) error
	SetDeactivated(ctx context.Context, admin, id resource.ID, at *time.Time) error
	// Enqueue stores new deliveries.
	Enqueue(ctx context.Context, deliveries []Delivery) error
	// Due hands out up to limit deliveries whose time has come, leasing them
	// until lease so no other worker sends them meanwhile.
	Due(ctx context.Context, now, lease time.Time, limit int) ([]Delivery, error)
	// SaveAttempt records an attempt on the delivery and its webhook.
	SaveAttempt(ctx context.Context, d Delivery, r *Response) error
}

// Service implements the webhook use cases.
type Service struct {
	admins Administrations
	store  Store
	sender Sender
	policy Policy
	ids    *resource.Generator
	now    func() time.Time
}

// NewService returns a Service.
func NewService(admins Administrations, store Store, sender Sender, policy Policy, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, store: store, sender: sender, policy: policy, ids: ids, now: now}
}

func (s *Service) access(ctx context.Context, admin, user resource.ID) error {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return err
	}
	if !a.Can(administration.Settings) {
		return ErrForbidden
	}
	return nil
}

// Webhooks lists the administration's webhooks, oldest first.
func (s *Service) Webhooks(ctx context.Context, admin, user resource.ID) ([]Webhook, error) {
	if err := s.access(ctx, admin, user); err != nil {
		return nil, err
	}
	list, err := s.store.Webhooks(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	slices.SortStableFunc(list, func(a, b Webhook) int { return a.CreatedAt.Compare(b.CreatedAt) })
	for i := range list {
		list[i].Secret = ""
	}
	return list, nil
}

// Webhook returns one webhook, without its secret.
func (s *Service) Webhook(ctx context.Context, admin, id, user resource.ID) (Webhook, error) {
	if err := s.access(ctx, admin, user); err != nil {
		return Webhook{}, err
	}
	w, err := s.store.Webhook(ctx, admin, id)
	w.Secret = ""
	return w, err
}

// NewWebhook is what to subscribe.
type NewWebhook struct {
	URL    string
	Events []string
	// ConnectionID ties the webhook to the connection that made it.
	ConnectionID *resource.ID
}

// Create subscribes a URL. As at Moneybird, the URL must answer a test
// delivery with 200. The secret is returned only now.
func (s *Service) Create(ctx context.Context, admin, user resource.ID, in NewWebhook) (Webhook, error) {
	if err := s.access(ctx, admin, user); err != nil {
		return Webhook{}, err
	}
	allowLocal, err := s.policy.WebhooksAllowLocal(ctx)
	if err != nil {
		return Webhook{}, err
	}
	var verr resource.ValidationError
	raw := strings.TrimSpace(in.URL)
	u, perr := url.Parse(raw)
	switch {
	case perr != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || len(raw) > 2000:
		verr.Add("url", "is not a valid URL")
	case u.Scheme == "http" && !allowLocal:
		verr.Add("url", "must use https")
	}
	events := []string{}
	for _, e := range in.Events {
		e = strings.TrimSpace(e)
		if !known(e) {
			verr.Add("enabled_events", e+" is not an event")
			continue
		}
		if !slices.Contains(events, e) {
			events = append(events, e)
		}
	}
	if err := verr.OrNil(); err != nil {
		return Webhook{}, err
	}
	now := s.now()
	w := Webhook{ID: s.ids.Next(), AdministrationID: admin, URL: raw, Events: events, Token: random(16), Secret: random(32), ConnectionID: in.ConnectionID, CreatedAt: now, UpdatedAt: now}
	body, _ := json.Marshal(map[string]any{"administration_id": admin.String(), "webhook_id": w.ID.String(), "webhook_token": w.Token, "action": "test_webhook"})
	r, err := s.sender.Post(ctx, w.URL, headers(w.Secret, random(16), body, now), body, allowLocal)
	if err != nil {
		verr.Add("url", "could not be reached: "+err.Error())
		return Webhook{}, &verr
	}
	if r.Status != 200 {
		verr.Add("url", fmt.Sprintf("answered %d to a test delivery, not 200", r.Status))
		return Webhook{}, &verr
	}
	if err := s.store.CreateWebhook(ctx, w); err != nil {
		return Webhook{}, fmt.Errorf("store webhook: %w", err)
	}
	return w, nil
}

// Delete removes a webhook.
func (s *Service) Delete(ctx context.Context, admin, id, user resource.ID) error {
	if err := s.access(ctx, admin, user); err != nil {
		return err
	}
	if _, err := s.store.Webhook(ctx, admin, id); err != nil {
		return err
	}
	return s.store.DeleteWebhook(ctx, admin, id)
}

// SetActive turns a webhook on or off.
func (s *Service) SetActive(ctx context.Context, admin, id, user resource.ID, on bool) (Webhook, error) {
	if err := s.access(ctx, admin, user); err != nil {
		return Webhook{}, err
	}
	w, err := s.store.Webhook(ctx, admin, id)
	if err != nil {
		return Webhook{}, err
	}
	var at *time.Time
	if !on {
		now := s.now()
		at = &now
		if w.DeactivatedAt != nil {
			at = w.DeactivatedAt
		}
	}
	if err := s.store.SetDeactivated(ctx, admin, id, at); err != nil {
		return Webhook{}, err
	}
	w.DeactivatedAt, w.Secret = at, ""
	return w, nil
}

// Publish queues an event for every active webhook subscribed to it.
func (s *Service) Publish(ctx context.Context, admin resource.ID, e Event) error {
	list, err := s.store.Webhooks(ctx, admin)
	if err != nil {
		return fmt.Errorf("list webhooks: %w", err)
	}
	now := s.now()
	var out []Delivery
	for _, w := range list {
		if w.DeactivatedAt != nil || !subscribed(w.Events, e.Action) {
			continue
		}
		var state any
		if e.State != "" {
			state = e.State
		}
		payload, err := json.Marshal(map[string]any{
			"administration_id": admin.String(), "webhook_id": w.ID.String(), "webhook_token": w.Token,
			"entity_type": e.EntityType, "entity_id": e.EntityID, "state": state, "action": e.Action, "entity": e.Entity,
		})
		if err != nil {
			return fmt.Errorf("encode payload: %w", err)
		}
		out = append(out, Delivery{ID: s.ids.Next(), WebhookID: w.ID, IdempotencyKey: random(16), Payload: payload, NextAttemptAt: &now, CreatedAt: now})
	}
	if len(out) == 0 {
		return nil
	}
	return s.store.Enqueue(ctx, out)
}

// Deliver sends the deliveries that are due and returns how many it tried.
func (s *Service) Deliver(ctx context.Context, limit int) (int, error) {
	now := s.now()
	due, err := s.store.Due(ctx, now, now.Add(5*time.Minute), limit)
	if err != nil {
		return 0, fmt.Errorf("due deliveries: %w", err)
	}
	allowLocal, err := s.policy.WebhooksAllowLocal(ctx)
	if err != nil {
		return 0, err
	}
	for _, d := range due {
		r, err := s.sender.Post(ctx, d.URL, headers(d.Secret, d.IdempotencyKey, d.Payload, s.now()), d.Payload, allowLocal)
		d.Attempts++
		var resp *Response
		switch {
		case err != nil:
			d.LastError = err.Error()
		default:
			resp = &r
			d.LastStatus, d.LastError = &r.Status, ""
		}
		if resp != nil && resp.Status == 200 {
			at := s.now()
			d.DeliveredAt, d.NextAttemptAt = &at, nil
		} else if d.Attempts >= maxAttempts {
			d.NextAttemptAt = nil // Given up.
		} else {
			next := s.now().Add(backoff[min(d.Attempts-1, len(backoff)-1)])
			d.NextAttemptAt = &next
		}
		if err := s.store.SaveAttempt(ctx, d, resp); err != nil {
			return 0, fmt.Errorf("save attempt: %w", err)
		}
	}
	return len(due), nil
}

// Run delivers until ctx ends, checking every interval.
func (s *Service) Run(ctx context.Context, interval time.Duration, onError func(error)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		for {
			n, err := s.Deliver(ctx, 50)
			if err != nil {
				onError(err)
			}
			if err != nil || n < 50 {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Sign makes the Moneybird-Signature header: t=<unix>,v1=<hex HMAC-SHA256
// of "t.body">.
func Sign(secret string, body []byte, at time.Time) string {
	t := at.Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(strconv.AppendInt(nil, t, 10))
	mac.Write([]byte("."))
	mac.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", t, hex.EncodeToString(mac.Sum(nil)))
}

func headers(secret, key string, body []byte, at time.Time) map[string]string {
	return map[string]string{"Content-Type": "application/json", "Idempotency-Key": key, "Moneybird-Signature": Sign(secret, body, at), "User-Agent": "MoneyHarpy-Webhooks"}
}

func random(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("read random: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
