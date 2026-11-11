package webhooks

import (
	"slices"
	"strings"
)

// Events are the Moneybird webhook events MoneyHarpy sends, with
// Moneybird's names. Others follow as their features arrive.
var Events = []string{
	"administration_details_edited", "administration_update_period_locked_until",
	"administration_automatic_bookers_activated", "administration_automatic_bookers_deactivated",
	"identity_created", "identity_updated", "identity_destroyed", "default_identity_updated",
	"workflow_created", "workflow_updated", "workflow_deactivated", "workflow_destroyed",
	"document_style_created", "document_style_updated", "document_style_destroyed",
	"tax_rate_created", "tax_rate_updated", "tax_rate_activated", "tax_rate_deactivated",
	"ledger_account_created", "ledger_account_updated", "ledger_account_activated", "ledger_account_deactivated", "ledger_account_destroyed",
	"ledger_account_booking_created", "ledger_account_booking_destroyed",
	"financial_account_created", "financial_account_renamed", "financial_account_activated", "financial_account_deactivated",
	"financial_statement_created", "financial_statement_updated", "financial_statement_destroyed",
	"booking_rule_created", "booking_rule_updated", "booking_rule_destroyed",
	"contact_created", "contact_changed", "contact_archived", "contact_activated", "contact_destroyed",
	"contact_person_created", "contact_person_updated", "contact_person_destroyed",
	"sales_invoice_created", "sales_invoice_created_from_original", "sales_invoice_updated", "sales_invoice_destroyed",
	"sales_invoice_send_email", "sales_invoice_send_post", "sales_invoice_send_manually", "sales_invoice_state_changed_to_open",
	"sales_invoice_state_changed_to_scheduled", "sales_invoice_paused", "sales_invoice_unpaused",
	"sales_invoice_state_changed_to_late", "sales_invoice_state_changed_to_reminded", "sales_invoice_state_changed_to_paid",
	"sales_invoice_state_changed_to_uncollectible", "sales_invoice_marked_as_dubious", "sales_invoice_marked_as_uncollectible",
	"sales_invoice_send_reminder_email", "sales_invoice_send_reminder_manually",
	"external_sales_invoice_created", "external_sales_invoice_updated", "external_sales_invoice_destroyed",
	"external_sales_invoice_state_changed_to_open", "external_sales_invoice_state_changed_to_late", "external_sales_invoice_state_changed_to_paid",
	"external_sales_invoice_state_changed_to_uncollectible", "external_sales_invoice_marked_as_dubious", "external_sales_invoice_marked_as_uncollectible",
	"recurring_sales_invoice_created", "recurring_sales_invoice_updated", "recurring_sales_invoice_deactivated", "recurring_sales_invoice_destroyed",
	"recurring_sales_invoice_invoice_created",
	"payment_registered", "payment_destroyed", "payment_linked_to_financial_mutation",
	"user_invited", "user_removed",
	"access_token_created", "access_token_revoked",
}

// Groups are the prefixes people can subscribe to, as at Moneybird: a group
// matches every event starting with it.
var Groups = []string{
	"administration", "identity", "default_identity", "workflow", "document_style", "tax_rate", "ledger_account",
	"ledger_account_booking", "financial_account", "financial_statement", "booking_rule", "contact", "contact_person", "sales_invoice", "external_sales_invoice", "recurring_sales_invoice", "payment", "user", "access_token",
}

// known reports whether a subscription names an event or a group.
func known(name string) bool { return slices.Contains(Events, name) || slices.Contains(Groups, name) }

// subscribed reports whether a webhook with these subscriptions gets the
// event. Without subscriptions, Moneybird sends its legacy sales invoice
// events, which MoneyHarpy has none of yet.
func subscribed(enabled []string, event string) bool {
	for _, e := range enabled {
		if e == event || strings.HasPrefix(event, e+"_") {
			return true
		}
	}
	return false
}
