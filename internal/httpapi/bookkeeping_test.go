package httpapi_test

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
)

type taxRate struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Percentage     string   `json:"percentage"`
	TaxRateType    string   `json:"tax_rate_type"`
	Active         bool     `json:"active"`
	URI            string   `json:"uri"`
	TaxReportBoxes []string `json:"tax_report_boxes"`
	Position       int      `json:"position"`
}

type ledgerAccount struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	AccountType  string  `json:"account_type"`
	AccountID    *string `json:"account_id"`
	ParentID     *string `json:"parent_id"`
	Active       bool    `json:"active"`
	URI          string  `json:"uri"`
	SystemRole   *string `json:"system_role"`
	TaxonomyItem *struct {
		Code            string `json:"code"`
		TaxonomyVersion string `json:"taxonomy_version"`
	} `json:"taxonomy_item"`
}

func TestBookkeepingSettings(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	base := "/api/v2/" + a.Id + "/bookkeeping_settings"

	st := decode[httpapi.BookkeepingSettings](t, owner.do(http.MethodGet, base, nil))
	if st.VatDocumentsPeriod != "quarter" || st.ChartScheme != "nl-rgs-3.8" || len(st.ChartSchemes) == 0 || len(st.TaxReportBoxes) == 0 || st.PeriodLockedUntil != nil {
		t.Fatalf("settings = %+v", st)
	}
	day := adminNow().AddDate(0, 0, -1).Format(time.DateOnly)
	r := owner.do(http.MethodPatch, base, map[string]any{"bookkeeping_settings": map[string]any{"vat_documents_period": "month", "period_locked_until": day}})
	wantStatus(t, r, http.StatusOK)
	if st = decode[httpapi.BookkeepingSettings](t, r); st.VatDocumentsPeriod != "month" || st.PeriodLockedUntil.String() != day {
		t.Errorf("updated = %+v", st)
	}
	future := adminNow().AddDate(0, 0, 2).Format(time.DateOnly)
	wantStatus(t, owner.do(http.MethodPatch, base, map[string]any{"bookkeeping_settings": map[string]any{"period_locked_until": future}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPatch, base, map[string]any{"bookkeeping_settings": map[string]any{"vat_documents_period": "week"}}), http.StatusBadRequest)
	wantStatus(t, stranger.do(http.MethodGet, base, nil), http.StatusNotFound)
}

func TestTaxRates(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	base := "/api/v2/" + a.Id + "/tax_rates"

	all := decode[[]taxRate](t, owner.do(http.MethodGet, base+".json", nil))
	if len(all) == 0 || all[0].TaxRateType != "sales_invoice" || all[0].Percentage != "21" {
		t.Fatalf("list = %+v", all)
	}
	paged := owner.do(http.MethodGet, base+"?per_page=1&page=2", nil)
	wantStatus(t, paged, http.StatusOK)
	if got := decode[[]taxRate](t, paged); len(got) != 1 || got[0].ID != all[1].ID || paged.header.Get("X-Total-Count") != strconv.Itoa(len(all)) || !strings.Contains(paged.header.Get("Link"), `rel="prev"`) {
		t.Errorf("page 2 = %+v, total %s", got, paged.header.Get("X-Total-Count"))
	}
	if got := decode[[]taxRate](t, owner.do(http.MethodGet, base+"?per_page=100&page=9", nil)); len(got) != 0 {
		t.Errorf("past the end = %+v", got)
	}
	sales := decode[[]taxRate](t, owner.do(http.MethodGet, base+"?filter="+url.QueryEscape("tax_rate_type:sales_invoice,active:true"), nil))
	for _, r := range sales {
		if r.TaxRateType != "sales_invoice" {
			t.Errorf("filter matched %+v", r)
		}
	}
	wantStatus(t, owner.do(http.MethodGet, base+"?filter=colour:red", nil), http.StatusBadRequest)

	r := owner.do(http.MethodPost, base, map[string]any{"tax_rate": map[string]any{
		"name": "Reduced 9.5%", "percentage": "9,5", "tax_rate_type": "sales_invoice", "tax_report_boxes": []string{"NL/1b"},
	}})
	wantStatus(t, r, http.StatusCreated)
	created := decode[taxRate](t, r)
	if created.Percentage != "9.5" || created.URI != "/"+a.Id+"/tax_rates/"+created.ID || created.Position != len(sales)+1 {
		t.Errorf("created = %+v", created)
	}
	r = owner.do(http.MethodPatch, base+"/"+created.ID, map[string]any{"tax_rate": map[string]any{"active": false}})
	wantStatus(t, r, http.StatusOK)
	if got := decode[taxRate](t, r); got.Active {
		t.Errorf("updated = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodPost, base, map[string]any{"tax_rate": map[string]any{"name": "", "percentage": "200", "tax_rate_type": "sales_invoice"}}), http.StatusUnprocessableEntity)

	ids := []string{created.ID}
	for _, r := range sales {
		ids = append(ids, r.ID)
	}
	wantStatus(t, owner.do(http.MethodPut, base+"/order", map[string]any{"tax_rate_type": "sales_invoice", "ids": ids}), http.StatusNoContent)
	if got := decode[taxRate](t, owner.do(http.MethodGet, base+"/"+created.ID, nil)); got.Position != 1 {
		t.Errorf("position after reorder = %d", got.Position)
	}
	wantStatus(t, stranger.do(http.MethodGet, base, nil), http.StatusNotFound)
	wantStatus(t, stranger.do(http.MethodGet, base+"/"+created.ID, nil), http.StatusNotFound)
}

func TestLedgerAccounts(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	base := "/api/v2/" + a.Id + "/ledger_accounts"

	all := decode[[]ledgerAccount](t, owner.do(http.MethodGet, base+".json", nil))
	i := slices.IndexFunc(all, func(l ledgerAccount) bool { return l.AccountID != nil && *l.AccountID == "4400" })
	if i < 0 || all[i].TaxonomyItem == nil || all[i].TaxonomyItem.TaxonomyVersion != "3.8" {
		t.Fatalf("office costs = %+v", all)
	}
	office := all[i]

	r := owner.do(http.MethodPost, base, map[string]any{
		"ledger_account": map[string]any{"name": "Hosting", "account_type": "expenses", "account_id": "4460", "parent_id": office.ID},
		"rgs_code":       "WBedKanSof",
	})
	wantStatus(t, r, http.StatusCreated)
	hosting := decode[ledgerAccount](t, r)
	if *hosting.ParentID != office.ID || hosting.TaxonomyItem.Code != "WBedKanSof" || hosting.URI != "/"+a.Id+"/ledger_accounts/"+hosting.ID {
		t.Errorf("created = %+v", hosting)
	}
	wantStatus(t, owner.do(http.MethodPost, base, map[string]any{"ledger_account": map[string]any{"name": "No code", "account_type": "expenses"}}), http.StatusUnprocessableEntity)

	items := decode[[]httpapi.TaxonomyItem](t, owner.do(http.MethodGet, "/api/v2/"+a.Id+"/taxonomy_items?query=WBedKan&account_type=expenses&limit=5", nil))
	if len(items) == 0 || len(items) > 5 {
		t.Errorf("taxonomy items = %+v", items)
	}

	r = owner.do(http.MethodPut, base+"/scheme", map[string]any{"ledger_accounts": []map[string]string{{"id": hosting.ID, "rgs_code": "WBedKanKan"}}})
	wantStatus(t, r, http.StatusOK)
	r = owner.do(http.MethodPatch, base+"/"+hosting.ID, map[string]any{"ledger_account": map[string]any{"name": "Web hosting", "parent_id": ""}})
	wantStatus(t, r, http.StatusOK)
	if got := decode[ledgerAccount](t, r); got.Name != "Web hosting" || got.ParentID != nil || got.TaxonomyItem.Code != "WBedKanKan" {
		t.Errorf("updated = %+v", got)
	}

	r = owner.do(http.MethodDelete, base+"/"+hosting.ID, nil)
	wantStatus(t, r, http.StatusOK)
	if decode[ledgerAccount](t, r).Active {
		t.Error("first delete did not deactivate")
	}
	wantStatus(t, owner.do(http.MethodDelete, base+"/"+hosting.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, base+"/"+hosting.ID, nil), http.StatusNotFound)

	system := all[slices.IndexFunc(all, func(l ledgerAccount) bool { return l.SystemRole != nil })]
	wantStatus(t, owner.do(http.MethodDelete, base+"/"+system.ID, nil), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodDelete, base+"/"+office.ID, nil), http.StatusUnprocessableEntity)
	wantStatus(t, stranger.do(http.MethodGet, base, nil), http.StatusNotFound)
}

func TestLedgerAccountTemplates(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	a := owner.createAdministration("Harpy")
	base := "/api/v2/" + a.Id + "/ledger_account_templates"

	list := decode[[]httpapi.LedgerAccountTemplate](t, owner.do(http.MethodGet, base, nil))
	if len(list) == 0 || list[0].Applied || len(list[0].LedgerAccounts) == 0 {
		t.Fatalf("templates = %+v", list)
	}
	r := owner.do(http.MethodPost, base+"/"+list[0].Key, nil)
	wantStatus(t, r, http.StatusCreated)
	if added := decode[[]ledgerAccount](t, r); len(added) != len(list[0].LedgerAccounts) {
		t.Errorf("added %d, want %d", len(added), len(list[0].LedgerAccounts))
	}
	if again := decode[[]ledgerAccount](t, owner.do(http.MethodPost, base+"/"+list[0].Key, nil)); len(again) != 0 {
		t.Errorf("second apply added %d", len(again))
	}
	if got := decode[[]httpapi.LedgerAccountTemplate](t, owner.do(http.MethodGet, base, nil)); !got[0].Applied {
		t.Error("template not marked applied")
	}
	wantStatus(t, owner.do(http.MethodPost, base+"/spaceship", nil), http.StatusNotFound)
}
