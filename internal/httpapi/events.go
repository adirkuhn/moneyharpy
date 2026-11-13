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
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
	"github.com/adirkuhn/moneyharpy/internal/tasklists"
	"github.com/adirkuhn/moneyharpy/internal/timeentries"
	"github.com/adirkuhn/moneyharpy/internal/webhooks"
)

// change is what a successful API write tells about what changed.
type change struct {
	params      map[string]string
	status      int
	header      http.Header
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

// invoice tells about a sales invoice the request saved, and the time
// entries it billed.
func (c change) invoice(action string) []webhooks.Event {
	return append(c.one("SalesInvoice", action), c.billedTime()...)
}

// billedTime tells that time entries a sales invoice request names went on
// the invoice.
func (c change) billedTime() []webhooks.Event {
	if ids, _ := c.nested("sales_invoice")["time_entry_ids"].([]any); len(ids) == 0 {
		return nil
	}
	return c.one("SalesInvoice", "time_entry_sales_invoice_created")
}

// taskUpdated tells what an update of a task changed, as Moneybird does:
// its name, the report or category it follows, or else anything.
func (c change) taskUpdated() []webhooks.Event {
	task := c.nested("task")
	var out []webhooks.Event
	if _, ok := task["name"]; ok {
		out = append(out, c.one(tasklists.NoteEntityType, "task_lists_task_name")...)
	}
	for _, key := range []string{"report_type", "related_entity_id"} {
		v, ok := task[key]
		switch {
		case !ok:
		case v == nil || v == "":
			out = append(out, c.one(tasklists.NoteEntityType, "task_lists_task_unlinked")...)
		case key == "report_type":
			out = append(out, c.one(tasklists.NoteEntityType, "task_lists_task_report_type_linked")...)
		default:
			out = append(out, c.one(tasklists.NoteEntityType, "task_lists_task_category_linked")...)
		}
	}
	return append(out, c.one(tasklists.NoteEntityType, "task_lists_task_updated")...)
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
		if t := c.request["booking_type"]; t == "SalesInvoice" || t == "ExternalSalesInvoice" || t == "Document" {
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
	"createSalesInvoice": func(c change) []webhooks.Event { return c.invoice("sales_invoice_created") },
	"updateSalesInvoice": func(c change) []webhooks.Event { return c.invoice("sales_invoice_updated") },
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
	"duplicateCreditSalesInvoice": func(c change) []webhooks.Event { return c.one("SalesInvoice", "credit_invoice_created_from_original") },
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
	"createExternalSalesInvoice": func(c change) []webhooks.Event {
		return c.one("ExternalSalesInvoice", "external_sales_invoice_created")
	},
	"createExternalSalesInvoiceFromAttachment": func(c change) []webhooks.Event {
		return c.one("ExternalSalesInvoice", "external_sales_invoice_created")
	},
	"updateExternalSalesInvoice": func(c change) []webhooks.Event {
		return c.one("ExternalSalesInvoice", "external_sales_invoice_updated")
	},
	"deleteExternalSalesInvoice": func(c change) []webhooks.Event {
		return c.one("ExternalSalesInvoice", "external_sales_invoice_destroyed")
	},
	"createExternalSalesInvoicePayment": func(c change) []webhooks.Event { return c.payment() },
	"deleteExternalSalesInvoicePayment": func(c change) []webhooks.Event {
		id := c.params["payment_id"]
		return []webhooks.Event{{EntityType: "Payment", EntityID: id, Action: "payment_destroyed", Entity: json.RawMessage(`{"id":"` + id + `"}`)}}
	},
	"markExternalSalesInvoiceAsDubious": func(c change) []webhooks.Event {
		return c.one("ExternalSalesInvoice", "external_sales_invoice_marked_as_dubious")
	},
	"markExternalSalesInvoiceAsUncollectible": func(c change) []webhooks.Event {
		return c.one("ExternalSalesInvoice", "external_sales_invoice_marked_as_uncollectible")
	},
	"createRecurringSalesInvoice": func(c change) []webhooks.Event {
		return c.one(sales.RecurringEntityType, "recurring_sales_invoice_created")
	},
	"updateRecurringSalesInvoice": func(c change) []webhooks.Event {
		return c.one(sales.RecurringEntityType, "recurring_sales_invoice_updated")
	},
	"deactivateRecurringSalesInvoice": func(c change) []webhooks.Event {
		return c.one(sales.RecurringEntityType, "recurring_sales_invoice_deactivated")
	},
	"deleteRecurringSalesInvoice": func(c change) []webhooks.Event {
		if c.header.Get("X-Kept-As") == "deactivated" {
			return c.one(sales.RecurringEntityType, "recurring_sales_invoice_deactivated")
		}
		return c.one(sales.RecurringEntityType, "recurring_sales_invoice_destroyed")
	},
	"createSubscription": func(c change) []webhooks.Event { return c.one("Subscription", "subscription_created") },
	"updateSubscription": func(c change) []webhooks.Event {
		// A new product, amount or discount updates the subscription; other
		// fields edit it.
		sub := c.nested("subscription")
		for _, key := range []string{"product_id", "amount", "discount", "start_date"} {
			if _, ok := sub[key]; ok {
				return c.one("Subscription", "subscription_updated")
			}
		}
		return c.one("Subscription", "subscription_edited")
	},
	"cancelSubscription": func(c change) []webhooks.Event { return c.one("Subscription", "subscription_cancelled") },
	"createSubscriptionOneOffSalesInvoice": func(c change) []webhooks.Event {
		return append(c.one("SalesInvoice", "sales_invoice_created"), c.one("SalesInvoice", "sales_invoice_state_changed_to_scheduled")...)
	},
	"createProduct": func(c change) []webhooks.Event { return c.one("Product", "product_created") },
	"updateProduct": func(c change) []webhooks.Event {
		return c.toggled("Product", "product", "product_updated", "product_activated", "product_deactivated")
	},
	"deleteProduct": func(c change) []webhooks.Event {
		if c.header.Get("X-Kept-As") == "deactivated" {
			return c.one("Product", "product_deactivated")
		}
		return c.one("Product", "product_destroyed")
	},
	"createProject": func(c change) []webhooks.Event { return c.one("Project", "project_created") },
	"updateProject": func(c change) []webhooks.Event {
		switch c.nested("project")["state"] {
		case "archived":
			return c.one("Project", "project_archived")
		case "active":
			return c.one("Project", "project_activated")
		}
		return c.one("Project", "project_updated")
	},
	"deleteProject": func(c change) []webhooks.Event {
		if c.header.Get("X-Kept-As") == "archived" {
			return c.one("Project", "project_archived")
		}
		return c.one("Project", "project_destroyed")
	},
	"createTimeEntry":            func(c change) []webhooks.Event { return c.one(timeentries.EntityType, "time_entry_created") },
	"updateTimeEntry":            func(c change) []webhooks.Event { return c.one(timeentries.EntityType, "time_entry_updated") },
	"stopTimeEntry":              func(c change) []webhooks.Event { return c.one(timeentries.EntityType, "time_entry_updated") },
	"resumeTimeEntry":            func(c change) []webhooks.Event { return c.one(timeentries.EntityType, "time_entry_updated") },
	"deleteTimeEntry":            func(c change) []webhooks.Event { return c.one(timeentries.EntityType, "time_entry_destroyed") },
	"createTaskList":             func(c change) []webhooks.Event { return c.one(tasklists.ListEntityType, "task_lists_list_created") },
	"createTaskListFromTemplate": func(c change) []webhooks.Event { return c.one(tasklists.ListEntityType, "task_lists_list_created") },
	"updateTaskList":             func(c change) []webhooks.Event { return c.one(tasklists.ListEntityType, "task_lists_list_updated") },
	"deleteTaskList":             func(c change) []webhooks.Event { return c.one(tasklists.ListEntityType, "task_lists_list_destroyed") },
	"createTaskListTask":         func(c change) []webhooks.Event { return c.one(tasklists.NoteEntityType, "task_lists_task_created") },
	"updateTaskListTask":         func(c change) []webhooks.Event { return c.taskUpdated() },
	"deleteTaskListTask":         func(c change) []webhooks.Event { return c.one(tasklists.NoteEntityType, "task_lists_task_destroyed") },
	"completeTaskListTask":       func(c change) []webhooks.Event { return c.one(tasklists.NoteEntityType, "task_lists_task_completed") },
	"reopenTaskListTask":         func(c change) []webhooks.Event { return c.one(tasklists.NoteEntityType, "task_lists_task_reopened") },
	"assignTaskListTask":         func(c change) []webhooks.Event { return c.one(tasklists.NoteEntityType, "task_lists_task_assigned") },
	"unassignTaskListTask":       func(c change) []webhooks.Event { return c.one(tasklists.NoteEntityType, "task_lists_task_unassigned") },
	"createTaskListTemplate": func(c change) []webhooks.Event {
		return c.one(tasklists.TemplateEntityType, "task_lists_list_template_created")
	},
	"updateTaskListTemplate": func(c change) []webhooks.Event {
		return c.one(tasklists.TemplateEntityType, "task_lists_list_template_updated")
	},
	"deleteTaskListTemplate": func(c change) []webhooks.Event {
		return c.one(tasklists.TemplateEntityType, "task_lists_list_template_destroyed")
	},
	"createContact": func(c change) []webhooks.Event { return c.one("Contact", "contact_created") },
	"updateContact": func(c change) []webhooks.Event {
		switch c.nested("contact")["archived"] {
		case true:
			return c.one("Contact", "contact_archived")
		case false:
			return c.one("Contact", "contact_activated")
		}
		return c.one("Contact", "contact_changed")
	},
	"archiveContact": func(c change) []webhooks.Event { return c.one("Contact", "contact_archived") },
	"deleteContact": func(c change) []webhooks.Event {
		if c.header.Get("X-Kept-As") == "archived" {
			return c.one("Contact", "contact_archived")
		}
		return c.one("Contact", "contact_destroyed")
	},
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
	// Incoming documents and purchase transactions.
	"createPurchaseInvoice":          func(c change) []webhooks.Event { return c.one("Document", "document_saved") },
	"updatePurchaseInvoice":          func(c change) []webhooks.Event { return c.one("Document", "document_updated") },
	"deletePurchaseInvoice":          func(c change) []webhooks.Event { return c.destroyed("Document", "document_destroyed") },
	"createPurchaseInvoicePayment":   func(c change) []webhooks.Event { return c.payment() },
	"registerPurchaseInvoicePayment": func(c change) []webhooks.Event { return c.payment() },
	"deletePurchaseInvoicePayment":   func(c change) []webhooks.Event { return c.destroyedPayment() },
	"createReceipt":                  func(c change) []webhooks.Event { return c.one("Document", "document_saved") },
	"updateReceipt":                  func(c change) []webhooks.Event { return c.one("Document", "document_updated") },
	"deleteReceipt":                  func(c change) []webhooks.Event { return c.destroyed("Document", "document_destroyed") },
	"createReceiptPayment":           func(c change) []webhooks.Event { return c.payment() },
	"registerReceiptPayment":         func(c change) []webhooks.Event { return c.payment() },
	"deleteReceiptPayment":           func(c change) []webhooks.Event { return c.destroyedPayment() },
	"createGeneralDocument":          func(c change) []webhooks.Event { return c.one("Document", "document_saved") },
	"updateGeneralDocument":          func(c change) []webhooks.Event { return c.one("Document", "document_updated") },
	"deleteGeneralDocument":          func(c change) []webhooks.Event { return c.destroyed("Document", "document_destroyed") },
	"createTypelessDocument":         func(c change) []webhooks.Event { return c.one("Document", "document_saved") },
	"deleteTypelessDocument":         func(c change) []webhooks.Event { return c.destroyed("Document", "document_destroyed") },
	// A document turned into a journal entry is no document any more.
	"setIncomingDocumentType": func(c change) []webhooks.Event {
		if c.responseMap["type"] == "GeneralJournalDocument" {
			return c.destroyed("Document", "document_destroyed")
		}
		return c.one("Document", "document_updated")
	},
	"createPurchaseTransaction": func(c change) []webhooks.Event {
		return c.one("PurchaseTransaction", "purchase_transaction_created")
	},
	"deletePurchaseTransaction": func(c change) []webhooks.Event {
		return c.destroyed("PurchaseTransaction", "purchase_transaction_deleted")
	},
	"deletePurchaseTransactionDeprecated": func(c change) []webhooks.Event {
		return c.destroyed("PurchaseTransaction", "purchase_transaction_deleted")
	},
	"createPurchaseTransactionBatch": func(c change) []webhooks.Event {
		return c.one("PurchaseTransactionBatch", "purchase_transaction_batch_created")
	},
	// Assets; the linear value changes MoneyHarpy books itself are told by
	// AssetNotifier.
	"createAsset": func(c change) []webhooks.Event {
		out := c.one("Asset", "company_assets_asset_created")
		if c.responseMap["value_change_plan"] != nil {
			out = append(out, c.one("Asset", "company_assets_value_change_plan_created")...)
		}
		return out
	},
	"updateAsset": func(c change) []webhooks.Event {
		out := c.one("Asset", "company_assets_asset_updated")
		if _, ok := c.nested("asset")["value_change_plan_attributes"]; ok {
			out = append(out, c.one("Asset", "company_assets_value_change_plan_updated")...)
		}
		return out
	},
	"deleteAsset":       func(c change) []webhooks.Event { return c.destroyed("Asset", "company_assets_asset_destroyed") },
	"createAssetSource": func(c change) []webhooks.Event { return c.one("Source", "company_assets_source_created") },
	"deleteAssetSource": func(c change) []webhooks.Event {
		id := c.params["source_id"]
		return []webhooks.Event{{EntityType: "Source", EntityID: id, Action: "company_assets_source_destroyed", Entity: json.RawMessage(`{"id":"` + id + `"}`)}}
	},
	"createAssetManualValueChange": func(c change) []webhooks.Event {
		return c.one("ValueChange", "company_assets_value_changes_manual_created")
	},
	"createAssetArbitraryValueChange": func(c change) []webhooks.Event {
		return c.one("ValueChange", "company_assets_value_changes_arbitrary_created")
	},
	"createAssetDivestmentValueChange": func(c change) []webhooks.Event {
		return append(c.one("ValueChange", "company_assets_value_changes_divestment_created"), c.disposed()...)
	},
	"createAssetFullDepreciationValueChange": func(c change) []webhooks.Event {
		return append(c.one("ValueChange", "company_assets_value_changes_full_depreciation_created"), c.disposed()...)
	},
	"createAssetDisposal": func(c change) []webhooks.Event { return c.disposed() },
	"createSepaCreditTransfer": func(c change) []webhooks.Event {
		var list []json.RawMessage
		_ = json.Unmarshal(c.response, &list)
		var out []webhooks.Event
		for _, raw := range list {
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			id, _ := m["id"].(string)
			out = append(out, webhooks.Event{EntityType: "PurchaseTransactionBatch", EntityID: id, Action: "purchase_transaction_batch_created", Entity: raw})
		}
		return out
	},
}

// disposed tells that an asset was disposed of, which ends its
// depreciation plan.
func (c change) disposed() []webhooks.Event {
	id := c.params["id"]
	entity := json.RawMessage(`{"asset_id":"` + id + `"}`)
	return []webhooks.Event{
		{EntityType: "Disposal", EntityID: id, Action: "company_assets_disposal_created", Entity: entity},
		{EntityType: "Asset", EntityID: id, Action: "company_assets_value_change_plan_deactivated", Entity: entity},
	}
}

// destroyed tells about a deleted record, which the response does not hold.
func (c change) destroyed(entityType, action string) []webhooks.Event {
	id := c.params["id"]
	return []webhooks.Event{{EntityType: entityType, EntityID: id, Action: action, Entity: json.RawMessage(`{"id":"` + id + `"}`)}}
}

// destroyedPayment tells about a deleted payment.
func (c change) destroyedPayment() []webhooks.Event {
	id := c.params["payment_id"]
	return []webhooks.Event{{EntityType: "Payment", EntityID: id, Action: "payment_destroyed", Entity: json.RawMessage(`{"id":"` + id + `"}`)}}
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
		c := change{params: info.params, status: rec.status, header: rec.Header(), response: bytes.TrimSpace(rec.body.Bytes())}
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
// state by the background job or an invoice a recurring invoice made, as
// webhook events.
type SalesNotifier struct {
	Webhooks *webhooks.Service
	Logger   *slog.Logger
}

// Notify implements sales.Notifier, for sales and external invoices.
func (n SalesNotifier) Notify(ctx context.Context, admin resource.ID, action string, inv sales.Invoice) {
	fields := map[string]any{
		"id": inv.ID.String(), "administration_id": admin.String(), "contact_id": inv.ContactID.String(), "invoice_id": nz(inv.InvoiceID),
		"state": inv.State, "total_price_incl_tax": inv.Totals.InclTax.String(), "total_unpaid": inv.Unpaid().String(),
		"reminder_count": inv.ReminderCount, "version": inv.Version(), "uri": inv.Ref().URI(),
	}
	entityType := "SalesInvoice"
	if inv.Type == sales.ExternalEntityType {
		entityType = sales.ExternalEntityType
		delete(fields, "invoice_id")
		delete(fields, "reminder_count")
		fields["reference"] = nz(inv.Reference)
	}
	entity, _ := json.Marshal(fields)
	e := webhooks.Event{EntityType: entityType, EntityID: inv.ID.String(), Action: action, State: inv.State, Entity: entity}
	if err := n.Webhooks.Publish(context.WithoutCancel(ctx), admin, e); err != nil && n.Logger != nil {
		n.Logger.ErrorContext(ctx, "publish webhook event", "action", action, "error", err)
	}
}

// NotifyRecurring implements sales.Notifier, for recurring invoices.
func (n SalesNotifier) NotifyRecurring(ctx context.Context, admin resource.ID, action string, r sales.Recurring) {
	entity, _ := json.Marshal(map[string]any{
		"id": r.ID.String(), "administration_id": admin.String(), "contact_id": r.ContactID.String(), "active": r.Active,
		"invoice_date": r.InvoiceDate.Format(time.DateOnly), "frequency_type": r.FrequencyType, "frequency": r.Frequency,
		"total_price_incl_tax": r.Totals.InclTax.String(), "version": r.Version(), "uri": r.Ref().URI(),
	})
	e := webhooks.Event{EntityType: sales.RecurringEntityType, EntityID: r.ID.String(), Action: action, Entity: entity}
	if err := n.Webhooks.Publish(context.WithoutCancel(ctx), admin, e); err != nil && n.Logger != nil {
		n.Logger.ErrorContext(ctx, "publish webhook event", "action", action, "error", err)
	}
}
