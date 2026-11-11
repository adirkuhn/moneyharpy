package httpapi_test

import (
	"flag"
	"fmt"
	"io"
	"sort"
	"sync"
)

// covered holds every contract operation ("METHOD /path") the suite has
// called with a successful (2xx or redirect) response.
var covered sync.Map

// refusedOnPurpose lists operations that only ever answer with an error,
// because the feature behind them is deliberately not offered.
var refusedOnPurpose = map[string]string{
	"GET /api/v2/{administration_id}/contacts/{contact_id}/moneybird_payments_mandate":        "Moneybird Payments is not offered",
	"POST /api/v2/{administration_id}/contacts/{contact_id}/moneybird_payments_mandate":       "Moneybird Payments is not offered",
	"DELETE /api/v2/{administration_id}/contacts/{contact_id}/moneybird_payments_mandate":     "Moneybird Payments is not offered",
	"POST /api/v2/{administration_id}/contacts/{contact_id}/moneybird_payments_mandate/url":   "Moneybird Payments is not offered",
	"GET /api/v2/{administration_id}/estimates/{id}/download_signed_pdf":                      "online signing is not offered",
	"POST /api/v2/{administration_id}/products/{id}/sales_link":                               "online sales are not offered",
	"POST /api/v2/{administration_id}/subscription_templates/{id}/sales_link":                 "subscription templates need Moneybird Payments",
	"GET /api/v2/{administration_id}/subscription_templates/{id}/checkout_identifier":         "subscription templates need Moneybird Payments",
	"GET /api/v2/{administration_id}/customer_contact_portal/{contact_id}/subscriptions/{id}": "only subscriptions sold through a template have a portal link",
}

func recordCoverage(method, path string, status int) {
	if status >= 200 && status < 400 {
		covered.Store(method+" "+path, true)
	}
}

// uncoveredOperations lists contract operations no test called successfully.
func uncoveredOperations() []string {
	var missing []string
	for path, item := range spec.Paths.Map() {
		for method := range item.Operations() {
			key := method + " " + path
			if _, ok := covered.Load(key); ok {
				continue
			}
			if _, ok := refusedOnPurpose[key]; ok {
				continue
			}
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	return missing
}

// fullRun reports whether the whole suite ran, so coverage is meaningful.
func fullRun() bool {
	for _, name := range []string{"test.run", "test.skip"} {
		if f := flag.Lookup(name); f != nil && f.Value.String() != "" {
			return false
		}
	}
	return !flag.Parsed() || flag.Lookup("test.short").Value.String() != "true"
}

// checkOperationCoverage fails the run when a contract operation has no
// successful test call.
func checkOperationCoverage(out io.Writer) bool {
	missing := uncoveredOperations()
	if len(missing) == 0 {
		return true
	}
	_, _ = fmt.Fprintf(out, "FAIL: %d contract operations have no successful test call:\n", len(missing))
	for _, op := range missing {
		_, _ = fmt.Fprintln(out, "  "+op)
	}
	return false
}
