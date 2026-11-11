package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
	"github.com/adirkuhn/moneyharpy/internal/webhooks"
)

// change is what a successful API write tells about what changed.
type change struct {
	params      map[string]string
	status      int
	request     map[string]any
	response    []byte
	responseMap map[string]any
}

// nested returns the object a request wraps its fields in, such as
// {"tax_rate": {...}}.
func (c change) nested(key string) map[string]any {
	m, _ := c.request[key].(map[string]any)
	return m
}

func (c change) id() string {
	if id, ok := c.responseMap["id"].(string); ok {
		return id
	}
	return c.params["id"]
}

// one makes a single event about the response.
func (c change) one(entityType, action string) []webhooks.Event {
	entity := json.RawMessage(c.response)
	if len(c.response) == 0 {
		entity = json.RawMessage(`{"id":"` + c.params["id"] + `"}`)
	}
	state, _ := c.responseMap["state"].(string)
	return []webhooks.Event{{EntityType: entityType, EntityID: c.id(), Action: action, State: state, Entity: entity}}
}

// payment tells about a payment in the response: registered, and linked
// when a bank transaction made it.
func (c change) payment() []webhooks.Event {
	events := c.one("Payment", "payment_registered")
	if id, _ := c.responseMap["financial_mutation_id"].(string); id != "" {
		events = append(events, c.one("Payment", "payment_linked_to_financial_mutation")...)
	}
	return events
}

// newestPayment tells about the payment last added to the response's
// payments, as a bank link or credit settlement adds it.
func (c change) newestPayment(linked bool) []webhooks.Event {
	list, _ := c.responseMap["payments"].([]any)
	var newest map[string]any
	for _, p := range list {
		m, _ := p.(map[string]any)
		if newest == nil || fmt.Sprint(m["created_at"]) >= fmt.Sprint(newest["created_at"]) {
			newest = m
		}
	}
	if newest == nil {
		return nil
	}
	raw, _ := json.Marshal(newest)
	id, _ := newest["id"].(string)
	events := []webhooks.Event{{EntityType: "Payment", EntityID: id, Action: "payment_registered", Entity: raw}}
	if linked {
		events = append(events, webhooks.Event{EntityType: "Payment", EntityID: id, Action: "payment_linked_to_financial_mutation", Entity: raw})
	}
	return events
}

// toggled picks the event of an update by the active field it sends.
func (c change) toggled(entityType, key, updated, activated, deactivated string) []webhooks.Event {
	switch c.nested(key)["active"] {
	case true:
		return c.one(entityType, activated)
	case false:
		return c.one(entityType, deactivated)
	}
	if updated == "" {
		return nil
	}
	return c.one(entityType, updated)
}

// webhookEvents maps contract operations to Moneybird's webhook events.
var webhookEvents = map[string]func(c change) []webhooks.Event{
	"updateAdministration": func(c change) []webhooks.Event { return c.one("Administration", "administration_details_edited") },
	"updateBookkeepingSettings": func(c change) []webhooks.Event {
		if _, ok := c.nested("bookkeeping_settings")["period_locked_until"]; ok {
			return c.one("Administration", "administration_update_period_locked_until")
		}
		return nil
	},
	"updateBankSettings": func(c change) []webhooks.Event {
		switch c.nested("bank_settings")["automatic_booking"] {
		case true:
			return c.one("Administration", "administration_automatic_bookers_activated")
		case false:
			return c.one("Administration", "administration_automatic_bookers_deactivated")
		}
		return nil
	},
	"createIdentity":        func(c change) []webhooks.Event { return c.one("Identity", "identity_created") },
	"updateIdentity":        func(c change) []webhooks.Event { return c.one("Identity", "identity_updated") },
	"deleteIdentity":        func(c change) []webhooks.Event { return c.one("Identity", "identity_destroyed") },
	"updateDefaultIdentity": func(c change) []webhooks.Event { return c.one("Identity", "default_identity_updated") },
	"createWorkflow":        func(c change) []webhooks.Event { return c.one("Workflow", "workflow_created") },
	"updateWorkflow": func(c change) []webhooks.Event {
		if c.nested("workflow")["active"] == false {
			return c.one("Workflow", "workflow_deactivated")
		}
		return c.one("Workflow", "workflow_updated")
	},
	"deleteWorkflow":      func(c change) []webhooks.Event { return c.one("Workflow", "workflow_destroyed") },
	"createDocumentStyle": func(c change) []webhooks.Event { return c.one("DocumentStyle", "document_style_created") },
	"updateDocumentStyle": func(c change) []webhooks.Event { return c.one("DocumentStyle", "document_style_updated") },
	"deleteDocumentStyle": func(c change) []webhooks.Event { return c.one("DocumentStyle", "document_style_destroyed") },
	"createTaxRate":       func(c change) []webhooks.Event { return c.one("TaxRate", "tax_rate_created") },
	"updateTaxRate": func(c change) []webhooks.Event {
		return c.toggled("TaxRate", "tax_rate", "tax_rate_updated", "tax_rate_activated", "tax_rate_deactivated")
	},
	"createLedgerAccount": func(c change) []webhooks.Event { return c.one("LedgerAccount", "ledger_account_created") },
	"updateLedgerAccount": func(c change) []webhooks.Event {
		return c.toggled("LedgerAccount", "ledger_account", "ledger_account_updated", "ledger_account_activated", "ledger_account_deactivated")
	},
	"deleteLedgerAccount": func(c change) []webhooks.Event {
		if c.status == http.StatusNoContent {
			return c.one("LedgerAccount", "ledger_account_destroyed")
		}
		return c.one("LedgerAccount", "ledger_account_deactivated")
	},
	"applyLedgerAccountTemplate": func(c change) []webhooks.Event {
		var list []json.RawMessage
		_ = json.Unmarshal(c.response, &list)
		var out []webhooks.Event
		for _, raw := range list {
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			id, _ := m["id"].(string)
			out = append(out, webhooks.Event{EntityType: "LedgerAccount", EntityID: id, Action: "ledger_account_created", Entity: raw})
		}
		return out
	},
	"createFinancialAccount": func(c change) []webhooks.Event { return c.one("FinancialAccount", "financial_account_created") },
	"updateFinancialAccount": func(c change) []webhooks.Event {
		if _, renamed := c.nested("financial_account")["name"]; renamed {
			return append(c.toggled("FinancialAccount", "financial_account", "", "financial_account_activated", "financial_account_deactivated"), c.one("FinancialAccount", "financial_account_renamed")...)
		}
		return c.toggled("FinancialAccount", "financial_account", "", "financial_account_activated", "financial_account_deactivated")
	},
	"createFinancialStatement": func(c change) []webhooks.Event { return c.one("FinancialStatement", "financial_statement_created") },
	"updateFinancialStatement": func(c change) []webhooks.Event { return c.one("FinancialStatement", "financial_statement_updated") },
	"deleteFinancialStatement": func(c change) []webhooks.Event { return c.one("FinancialStatement", "financial_statement_destroyed") },
	"importFinancialStatement": func(c change) []webhooks.Event {
		var res struct {
			Statements []json.RawMessage `json:"financial_statements"`
		}
		_ = json.Unmarshal(c.response, &res)
		var out []webhooks.Event
		for _, raw := range res.Statements {
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			id, _ := m["id"].(string)
			out = append(out, webhooks.Event{EntityType: "FinancialStatement", EntityID: id, Action: "financial_statement_created", Entity: raw})
		}
		return out
	},
	"linkFinancialMutationBooking": func(c change) []webhooks.Event {
		if c.request["booking_type"] == "SalesInvoice" {
			return c.newestPayment(true)
		}
		return c.one("FinancialMutation", "ledger_account_booking_created")
	},
	"unlinkFinancialMutationBooking": func(c change) []webhooks.Event {
		if c.request["booking_type"] == "Payment" {
			id := fmt.Sprint(c.request["booking_id"])
			return []webhooks.Event{{EntityType: "Payment", EntityID: id, Action: "payment_destroyed", Entity: json.RawMessage(`{"id":"` + id + `"}`)}}
		}
		return c.one("FinancialMutation", "ledger_account_booking_destroyed")
	},
	"createBookingRule":  func(c change) []webhooks.Event { return c.one("BookingRule", "booking_rule_created") },
	"updateBookingRule":  func(c change) []webhooks.Event { return c.one("BookingRule", "booking_rule_updated") },
	"deleteBookingRule":  func(c change) []webhooks.Event { return c.one("BookingRule", "booking_rule_destroyed") },
	"createSalesInvoice": func(c change) []webhooks.Event { return c.one("SalesInvoice", "sales_invoice_created") },
	"updateSalesInvoice": func(c change) []webhooks.Event { return c.one("SalesInvoice", "sales_invoice_updated") },
	"deleteSalesInvoice": func(c change) []webhooks.Event { return c.one("SalesInvoice", "sales_invoice_destroyed") },
	"sendSalesInvoice": func(c change) []webhooks.Event {
		action := map[string]string{"Email": "sales_invoice_send_email", "Post": "sales_invoice_send_post"}[fmt.Sprint(c.responseMap["delivery_method"])]
		if action == "" {
			action = "sales_invoice_send_manually"
		}
		events := c.one("SalesInvoice", action)
		if state, _ := c.responseMap["state"].(string); state == "open" || state == "scheduled" {
			events = append(events, c.one("SalesInvoice", "sales_invoice_state_changed_to_"+state)...)
		}
		return events
	},
	"duplicateCreditSalesInvoice": func(c change) []webhooks.Event { return c.one("SalesInvoice", "sales_invoice_created_from_original") },
	"createSalesInvoicePayment":   func(c change) []webhooks.Event { return c.payment() },
	"registerSalesInvoicePayment": func(c change) []webhooks.Event { return c.payment() },
	"deleteSalesInvoicePayment": func(c change) []webhooks.Event {
		id := c.params["payment_id"]
		return []webhooks.Event{{EntityType: "Payment", EntityID: id, Action: "payment_destroyed", Entity: json.RawMessage(`{"id":"` + id + `"}`)}}
	},
	"settleSalesCreditInvoice":        func(c change) []webhooks.Event { return c.newestPayment(false) },
	"markSalesInvoiceAsDubious":       func(c change) []webhooks.Event { return c.one("SalesInvoice", "sales_invoice_marked_as_dubious") },
	"markSalesInvoiceAsUncollectible": func(c change) []webhooks.Event { return c.one("SalesInvoice", "sales_invoice_marked_as_uncollectible") },
	"pauseSalesInvoice":               func(c change) []webhooks.Event { return c.one("SalesInvoice", "sales_invoice_paused") },
	"resumeSalesInvoice":              func(c change) []webhooks.Event { return c.one("SalesInvoice", "sales_invoice_unpaused") },
	"createContact":                   func(c change) []webhooks.Event { return c.one("Contact", "contact_created") },
	"updateContact": func(c change) []webhooks.Event {
		switch c.nested("contact")["archived"] {
		case true:
			return c.one("Contact", "contact_archived")
		case false:
			return c.one("Contact", "contact_activated")
		}
		return c.one("Contact", "contact_changed")
	},
	"archiveContact":       func(c change) []webhooks.Event { return c.one("Contact", "contact_archived") },
	"deleteContact":        func(c change) []webhooks.Event { return c.one("Contact", "contact_destroyed") },
	"createContactPerson":  func(c change) []webhooks.Event { return c.one("ContactPerson", "contact_person_created") },
	"updateContactPerson":  func(c change) []webhooks.Event { return c.one("ContactPerson", "contact_person_updated") },
	"deleteContactPerson":  func(c change) []webhooks.Event { return c.one("ContactPerson", "contact_person_destroyed") },
	"createUserInvitation": func(c change) []webhooks.Event { return c.one("UserInvitation", "user_invited") },
	"removeUser":           func(c change) []webhooks.Event { return c.one("User", "user_removed") },
	"createConnection": func(c change) []webhooks.Event {
		// The token itself never travels in an event.
		delete(c.responseMap, "token")
		c.response, _ = json.Marshal(c.responseMap)
		return c.one("AccessToken", "access_token_created")
	},
	"revokeConnection": func(c change) []webhooks.Event { return c.one("AccessToken", "access_token_revoked") },
}

type recordingWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *recordingWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *recordingWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// recordEvents publishes Moneybird webhook events for successful writes
// through the API, which is also how the web app changes things.
func (s *server) recordEvents(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, _ := r.Context().Value(routeKey).(routeInfo)
		events, ok := webhookEvents[info.op]
		if !ok || s.deps.Webhooks == nil {
			next.ServeHTTP(w, r)
			return
		}
		var reqBody []byte
		if r.Body != nil && strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			reqBody, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(reqBody))
		}
		rec := &recordingWriter{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status < 200 || rec.status > 299 {
			return
		}
		raw := info.params["administration_id"]
		if raw == "" {
			raw = info.params["id"]
		}
		admin, err := resource.ParseID(raw)
		if err != nil {
			return
		}
		c := change{params: info.params, status: rec.status, response: bytes.TrimSpace(rec.body.Bytes())}
		_ = json.Unmarshal(reqBody, &c.request)
		_ = json.Unmarshal(c.response, &c.responseMap)
		ctx := context.WithoutCancel(r.Context())
		for _, e := range events(c) {
			if err := s.deps.Webhooks.Publish(ctx, admin, e); err != nil {
				s.deps.Logger.ErrorContext(ctx, "publish webhook event", "action", e.Action, "error", err)
			}
		}
	})
}

// SalesNotifier publishes what sales invoices tell, such as a change of
// state by the background job, as webhook events.
type SalesNotifier struct {
	Webhooks *webhooks.Service
	Logger   *slog.Logger
}

// Notify implements sales.Notifier.
func (n SalesNotifier) Notify(ctx context.Context, admin resource.ID, action string, inv sales.Invoice) {
	entity, _ := json.Marshal(map[string]any{
		"id": inv.ID.String(), "administration_id": admin.String(), "contact_id": inv.ContactID.String(), "invoice_id": nz(inv.InvoiceID),
		"state": inv.State, "total_price_incl_tax": inv.Totals.InclTax.String(), "total_unpaid": inv.Unpaid().String(),
		"reminder_count": inv.ReminderCount, "version": inv.Version(), "uri": inv.Ref().URI(),
	})
	e := webhooks.Event{EntityType: "SalesInvoice", EntityID: inv.ID.String(), Action: action, State: inv.State, Entity: entity}
	if err := n.Webhooks.Publish(context.WithoutCancel(ctx), admin, e); err != nil && n.Logger != nil {
		n.Logger.ErrorContext(ctx, "publish webhook event", "action", action, "error", err)
	}
}
