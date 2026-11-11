package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
)

type assetValueChange struct {
	Type             string  `json:"type"`
	Date             string  `json:"date"`
	Amount           string  `json:"amount"`
	Description      *string `json:"description"`
	ExternallyBooked bool    `json:"externally_booked"`
	AssetID          string  `json:"asset_id"`
}

type assetJSON struct {
	ID                      string `json:"id"`
	LedgerAccountID         string `json:"ledger_account_id"`
	Name                    string `json:"name"`
	PurchaseValue           string `json:"purchase_value"`
	CalculatedPurchaseValue string `json:"calculated_purchase_value"`
	CurrentValue            string `json:"current_value"`
	URI                     string `json:"uri"`
	ValueChangePlan         *struct {
		LifespanInYears     float64 `json:"lifespan_in_years"`
		ResidualValue       string  `json:"residual_value"`
		Active              bool    `json:"active"`
		MonthlyDepreciation string  `json:"monthly_depreciation"`
	} `json:"value_change_plan"`
	Disposal *struct {
		Reason                  string `json:"reason"`
		ReinvestmentReserveSale *struct {
			ID     string `json:"id"`
			Amount string `json:"amount"`
		} `json:"reinvestment_reserve_sale"`
	} `json:"disposal"`
	ValueChanges []assetValueChange `json:"value_changes"`
	Sources      []struct {
		ID       string  `json:"id"`
		DetailID *string `json:"detail_id"`
	} `json:"sources"`
	ReinvestmentReservePurchase *struct {
		Amount string `json:"amount"`
	} `json:"reinvestment_reserve_purchase"`
}

func assetInput(name, ledger, date string, plan map[string]any) map[string]any {
	asset := map[string]any{"name": name, "ledger_account_id": ledger, "purchase_date": date, "purchase_value": 1200}
	if plan != nil {
		asset["value_change_plan_attributes"] = plan
	}
	return map[string]any{"asset": asset}
}

func TestAssetLifecycle(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	adminID, api, supplier, vat21 := purchaseSetup(t, owner)
	equipment := category(t, owner, adminID, "Equipment and furniture")
	purchase := time.Date(time.Now().Year()-1, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)

	r := owner.do(http.MethodPost, api+"/assets.json", assetInput("Laptop", equipment, purchase, map[string]any{"lifespan_in_years": "1", "residual_value": 0}))
	wantStatus(t, r, http.StatusCreated)
	a := decode[assetJSON](t, r)
	if a.ValueChangePlan == nil || a.ValueChangePlan.MonthlyDepreciation != "100.0" || !a.ValueChangePlan.Active || a.CurrentValue != "1200.0" ||
		a.URI != "/"+adminID+"/assets/"+a.ID || len(a.ValueChanges) != 0 || a.Sources == nil {
		t.Fatalf("created = %+v", a)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/assets", map[string]any{"asset": map[string]any{"ledger_account_id": equipment}}), http.StatusBadRequest)
	r = owner.do(http.MethodPost, api+"/assets", map[string]any{"asset": map[string]any{
		"name": "x", "ledger_account_id": equipment, "purchase_date": purchase, "purchase_value": "1.234",
	}})
	wantStatus(t, r, http.StatusBadRequest)
	r = owner.do(http.MethodPost, api+"/assets", assetInput("No plan", equipment, purchase, nil))
	wantStatus(t, r, http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.body), "value_change_plan") {
		t.Errorf("422 body = %s", r.body)
	}

	r = owner.do(http.MethodPatch, api+"/assets/"+a.ID, map[string]any{"asset": map[string]any{"name": "Work laptop"}})
	wantStatus(t, r, http.StatusOK)
	if got := decode[assetJSON](t, r); got.Name != "Work laptop" {
		t.Errorf("renamed = %+v", got)
	}

	// The purchase invoice that bought it is its source.
	inv := decode[purchaseDocument](t, owner.do(http.MethodPost, api+"/documents/purchase_invoices", map[string]any{"purchase_invoice": map[string]any{
		"contact_id": supplier, "date": purchase,
		"details_attributes": []map[string]any{{"description": "Laptop", "price": "1200", "tax_rate_id": vat21, "ledger_account_id": equipment}},
	}}))
	r = owner.do(http.MethodPost, api+"/assets/"+a.ID+"/sources", map[string]any{"detail_id": inv.Details[0].ID})
	wantStatus(t, r, http.StatusCreated)
	src := decode[struct {
		ID       string `json:"id"`
		AssetID  string `json:"asset_id"`
		DetailID string `json:"detail_id"`
	}](t, r)
	if src.AssetID != a.ID || src.DetailID != inv.Details[0].ID {
		t.Errorf("source = %+v", src)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/assets/"+a.ID+"/sources", map[string]any{"detail_id": "x"}), http.StatusBadRequest)
	wantStatus(t, owner.do(http.MethodPost, api+"/assets/"+a.ID+"/sources", map[string]any{}), http.StatusBadRequest)
	wantStatus(t, owner.do(http.MethodPost, api+"/assets/"+a.ID+"/sources", map[string]any{"general_journal_document_entry_id": "12345"}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodDelete, api+"/assets/"+a.ID+"/sources/"+src.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodDelete, api+"/assets/"+a.ID+"/sources/"+src.ID, nil), http.StatusNotFound)

	r = owner.do(http.MethodPost, api+"/assets/"+a.ID+"/value_changes/manual", map[string]any{
		"date": purchase, "amount": -10, "description": "Scratch", "externally_booked": "true",
	})
	wantStatus(t, r, http.StatusCreated)
	if c := decode[assetValueChange](t, r); c.Type != "manual" || c.Amount != "-10.0" || !c.ExternallyBooked || c.AssetID != a.ID {
		t.Errorf("manual = %+v", c)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/assets/"+a.ID+"/value_changes/manual", map[string]any{"date": "2026-02-30", "amount": 1, "description": "x"}), http.StatusBadRequest)
	wantStatus(t, owner.do(http.MethodPost, api+"/assets/"+a.ID+"/value_changes/arbitrary", map[string]any{"date": "2000-01-01", "amount": -1, "description": "x"}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPost, api+"/assets/"+a.ID+"/value_changes/retroactive_linear_value_changes", nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodPost, api+"/assets/"+a.ID+"/value_changes/retroactive_linear_value_changes", nil), http.StatusUnprocessableEntity)
	r = owner.do(http.MethodGet, api+"/assets/"+a.ID, nil)
	wantStatus(t, r, http.StatusOK)
	a = decode[assetJSON](t, r)
	if len(a.ValueChanges) != 13 || a.CurrentValue != "0.0" || a.ValueChangePlan.Active || a.ValueChanges[1].Type != "linear" {
		t.Fatalf("depreciated = %+v", a)
	}

	r = owner.do(http.MethodGet, api+"/reports/assets.json?period=prev_year", nil)
	wantStatus(t, r, http.StatusOK)
	lines := decode[[]map[string]any](t, r)
	if len(lines) != 1 || lines[0]["investment"] != "1200.0" || lines[0]["value_at_end"] != "0.0" || lines[0]["lifespan"] != "31556952" {
		t.Errorf("report = %+v", lines)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/reports/assets?period=202401..202512", nil), http.StatusBadRequest)

	wantStatus(t, owner.do(http.MethodPost, api+"/assets/"+a.ID+"/disposals", map[string]any{"date": purchase, "reason": "lost"}), http.StatusBadRequest)
	r = owner.do(http.MethodPost, api+"/assets/"+a.ID+"/disposals", map[string]any{"date": time.Now().Format(time.DateOnly), "reason": "sold"})
	wantStatus(t, r, http.StatusCreated)
	wantStatus(t, owner.do(http.MethodPost, api+"/assets/"+a.ID+"/reinvestment_reserve_sale", map[string]any{"amount": -5}), http.StatusBadRequest)
	r = owner.do(http.MethodPost, api+"/assets/"+a.ID+"/reinvestment_reserve_sale", map[string]any{"amount": "300"})
	wantStatus(t, r, http.StatusCreated)
	if sale := decode[map[string]any](t, r); sale["amount"] != "300.0" || sale["disposal_id"] == nil {
		t.Errorf("sale = %v", sale)
	}

	// The list shows active assets unless asked for all.
	r = owner.do(http.MethodGet, api+"/assets", nil)
	wantStatus(t, r, http.StatusOK)
	if got := decode[[]assetJSON](t, r); len(got) != 0 || r.header.Get("X-Total-Count") != "0" {
		t.Errorf("active = %+v", got)
	}
	r = owner.do(http.MethodGet, api+"/assets?active=false&ledger_account_id="+equipment, nil)
	if got := decode[[]assetJSON](t, r); len(got) != 1 || got[0].Disposal == nil || got[0].Disposal.ReinvestmentReserveSale == nil {
		t.Errorf("all = %+v", got)
	}

	van := decode[assetJSON](t, owner.do(http.MethodPost, api+"/assets", assetInput("Van", equipment, purchase, map[string]any{"lifespan_in_years": 5, "residual_value": "200"})))
	wantStatus(t, owner.do(http.MethodPost, api+"/assets/"+van.ID+"/reinvestment_reserve_purchase", map[string]any{"amount": 400}), http.StatusUnprocessableEntity)
	r = owner.do(http.MethodPost, api+"/assets/"+van.ID+"/reinvestment_reserve_purchase", map[string]any{"amount": 300})
	wantStatus(t, r, http.StatusCreated)
	van = decode[assetJSON](t, owner.do(http.MethodGet, api+"/assets/"+van.ID, nil))
	if van.CalculatedPurchaseValue != "900.0" || van.ReinvestmentReservePurchase == nil || van.ReinvestmentReservePurchase.Amount != "300.0" {
		t.Errorf("van = %+v", van)
	}
	wantStatus(t, owner.do(http.MethodDelete, api+"/assets/"+van.ID+"/reinvestment_reserve_purchase", nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodDelete, api+"/assets/"+van.ID+"/reinvestment_reserve_purchase", nil), http.StatusNotFound)
	wantStatus(t, owner.do(http.MethodDelete, api+"/assets/"+van.ID+"/reinvestment_reserve_sale", nil), http.StatusNotFound)
	wantStatus(t, owner.do(http.MethodPost, api+"/assets/"+van.ID+"/disposals", map[string]any{"date": purchase, "reason": "sold"}), http.StatusUnprocessableEntity)
	r = owner.do(http.MethodPost, api+"/assets/"+van.ID+"/value_changes/full_depreciation", map[string]any{"date": purchase, "description": "Stolen"})
	wantStatus(t, r, http.StatusCreated)
	if c := decode[assetValueChange](t, r); c.Type != "full_depreciation" || c.Amount != "-1200.0" {
		t.Errorf("full depreciation = %+v", c)
	}

	// Journal entries show what assets booked.
	r = owner.do(http.MethodGet, api+"/reports/journal_entries?period=prev_year&ledger_account_id="+category(t, owner, adminID, "Depreciation costs"), nil)
	wantStatus(t, r, http.StatusOK)
	if entries := decode[[]map[string]any](t, r); len(entries) == 0 || entries[0]["document_type"] != "Asset" {
		t.Errorf("journal entries = %+v", entries)
	}

	wantStatus(t, owner.do(http.MethodDelete, api+"/assets/"+van.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, api+"/assets/"+van.ID, nil), http.StatusNotFound)
	wantStatus(t, owner.do(http.MethodDelete, api+"/assets/"+van.ID+"/reinvestment_reserve_sale", nil), http.StatusNotFound)
}

func TestAssetsNeedDocumentsPermission(t *testing.T) {
	h := newHarness(t, options{})
	owner, banker := h.client(), h.client()
	owner.register()
	banker.register()
	adminID, api, _, _ := purchaseSetup(t, owner)
	invite(t, h, owner, banker, adminID, "banker@example.com", map[string]any{"user_type": "employee", "permissions": []string{"bank"}})
	wantStatus(t, banker.do(http.MethodGet, api+"/assets", nil), http.StatusForbidden)
	wantStatus(t, banker.do(http.MethodGet, api+"/reports/assets", nil), http.StatusForbidden)
}

func TestAssetWebhookEvents(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	owner := h.client()
	owner.register()
	adminID, api, _, _ := purchaseSetup(t, owner)
	equipment := category(t, owner, adminID, "Equipment and furniture")
	rc := newReceiver(t)
	wantStatus(t, owner.do(http.MethodPatch, "/api/instance/settings", map[string]bool{"webhooks_allow_local": true}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/webhooks", map[string]any{"url": rc.srv.URL, "enabled_events": []string{"company_assets"}}), http.StatusCreated)

	day := time.Now().AddDate(0, -2, 0).Format(time.DateOnly)
	a := decode[assetJSON](t, owner.do(http.MethodPost, api+"/assets", assetInput("Desk", equipment, day, map[string]any{"lifespan_in_years": 5})))
	wantStatus(t, owner.do(http.MethodPatch, api+"/assets/"+a.ID, map[string]any{"asset": map[string]any{"value_change_plan_attributes": map[string]any{"residual_value": 100}}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/assets/"+a.ID+"/value_changes/retroactive_linear_value_changes", nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodPost, api+"/assets/"+a.ID+"/value_changes/arbitrary", map[string]any{"date": day, "amount": -10, "description": "Dent"}), http.StatusCreated)
	wantStatus(t, owner.do(http.MethodPost, api+"/assets/"+a.ID+"/value_changes/divestment", map[string]any{"date": day}), http.StatusCreated)
	wantStatus(t, owner.do(http.MethodDelete, api+"/assets/"+a.ID, nil), http.StatusNoContent)
	if _, err := h.hooks.Deliver(ctx, 50); err != nil {
		t.Fatal(err)
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	var actions []string
	for _, b := range rc.body[1:] {
		var payload map[string]any
		_ = json.Unmarshal(b, &payload)
		actions = append(actions, payload["action"].(string))
	}
	actions = slices.Compact(slices.Sorted(slices.Values(actions)))
	want := []string{
		"company_assets_asset_created", "company_assets_asset_destroyed", "company_assets_asset_updated", "company_assets_disposal_created",
		"company_assets_value_change_plan_created", "company_assets_value_change_plan_deactivated", "company_assets_value_change_plan_updated",
		"company_assets_value_changes_arbitrary_created", "company_assets_value_changes_divestment_created",
		"company_assets_value_changes_linear_created", "company_assets_value_changes_linear_destroyed",
	}
	if !slices.Equal(actions, want) {
		t.Errorf("actions = %v, want %v", actions, want)
	}
}
