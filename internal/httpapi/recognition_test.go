package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
	"github.com/adirkuhn/moneyharpy/internal/recognize"
)

// testEInvoice stands in for the e-invoice parser: an XML file starting
// with <Invoice is a purchase invoice of Example Shop B.V. over 121.00.
func testEInvoice(xml []byte) (recognize.Extraction, error) {
	if !strings.HasPrefix(string(xml), "<Invoice") {
		return recognize.Extraction{}, errors.New("not an e-invoice")
	}
	sure := recognize.Certain
	return recognize.Extraction{
		Kind:          recognize.Value[recognize.Kind]{V: recognize.KindPurchaseInvoice, Found: true, Confidence: sure},
		Supplier:      recognize.Supplier{Name: "Example Shop B.V.", VATNumber: "NL123456789B01", IBAN: "NL91ABNA0417164300", Country: "NL"},
		Reference:     recognize.Value[string]{V: "EX-2026-7", Found: true, Confidence: sure},
		Date:          recognize.Value[time.Time]{V: adminNow().AddDate(0, 0, -3).Truncate(24 * time.Hour), Found: true, Confidence: sure},
		Currency:      recognize.Value[string]{V: "EUR", Found: true, Confidence: sure},
		PricesInclTax: recognize.Value[bool]{V: false, Found: true, Confidence: sure},
		TotalInclTax:  recognize.Value[money.Amount]{V: 12100, Found: true, Confidence: sure},
		Lines: []recognize.Line{{
			Description: "Consulting", Quantity: recognize.Value[string]{V: "1", Found: true, Confidence: sure},
			Price: recognize.Value[money.Amount]{V: 10000, Found: true, Confidence: sure}, TaxRate: recognize.Value[string]{V: "21", Found: true, Confidence: sure},
		}},
	}, nil
}

type recognitionJSON struct {
	State  string  `json:"state"`
	Source *string `json:"source"`
	Fields []struct {
		Field      string `json:"field"`
		Confidence string `json:"confidence"`
	} `json:"fields"`
}

func TestScanAndRecogniseOverTheAPI(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	_, api, _, _ := purchaseSetup(t, owner)
	typeless := api + "/documents/typeless_documents"

	// An upload is read in the background.
	r := owner.do(http.MethodPost, typeless, map[string]any{"typeless_document": map[string]any{}})
	wantStatus(t, r, http.StatusCreated)
	created := decode[struct {
		ID          string          `json:"id"`
		Recognition recognitionJSON `json:"recognition"`
	}](t, r)
	if created.Recognition.State != "none" || created.Recognition.Source != nil {
		t.Errorf("new typeless recognition = %+v", created.Recognition)
	}
	id := created.ID
	wantStatus(t, owner.attach(typeless+"/"+id+"/attachments", "invoice.xml", "text/xml", []byte("<Invoice/>")), http.StatusOK)
	tl := decode[struct {
		Recognition recognitionJSON `json:"recognition"`
	}](t, owner.do(http.MethodGet, typeless+"/"+id, nil))
	if tl.Recognition.State != "pending" {
		t.Errorf("after the upload = %+v", tl.Recognition)
	}
	list := decode[[]httpapi.IncomingDocumentSummary](t, owner.do(http.MethodGet, api+"/documents?filter=type:all", nil))
	if len(list) != 1 || list[0].RecognitionState != "pending" {
		t.Errorf("incoming list = %+v", list)
	}

	if err := h.recognition.Work(ctx); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	r = owner.do(http.MethodGet, api+"/documents/purchase_invoices/"+id, nil)
	wantStatus(t, r, http.StatusOK)
	inv := decode[struct {
		State       string          `json:"state"`
		Reference   string          `json:"reference"`
		ContactID   string          `json:"contact_id"`
		Recognition recognitionJSON `json:"recognition"`
	}](t, r)
	if inv.State != "new" || inv.Reference != "EX-2026-7" || inv.ContactID == "" || inv.Recognition.State != "done" ||
		inv.Recognition.Source == nil || *inv.Recognition.Source != "e_invoice" || len(inv.Recognition.Fields) < 5 {
		t.Fatalf("purchase invoice = %+v", inv)
	}
	if got := decode[httpapi.IncomingDocumentSummary](t, owner.do(http.MethodGet, api+"/documents/"+id, nil)); got.RecognitionState != "done" {
		t.Errorf("incoming document = %+v", got)
	}

	// Choosing the type it has confirms the type.
	marked := func(field string) bool {
		got := decode[struct {
			Recognition recognitionJSON `json:"recognition"`
		}](t, owner.do(http.MethodGet, api+"/documents/purchase_invoices/"+id, nil))
		for _, f := range got.Recognition.Fields {
			if f.Field == field {
				return true
			}
		}
		return false
	}
	if !marked("document_type") {
		t.Fatal("the type is not marked")
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/documents/"+id+"/set_type", map[string]any{"document_type": "purchase_invoice"}), http.StatusOK)
	if marked("document_type") {
		t.Error("the type is still marked")
	}

	// Read again: pending, 202.
	r = owner.do(http.MethodPost, api+"/documents/"+id+"/recognize", nil)
	wantStatus(t, r, http.StatusAccepted)
	if got := decode[httpapi.IncomingDocumentSummary](t, r); got.RecognitionState != "pending" || got.Type != "PurchaseInvoice" {
		t.Errorf("recognize = %+v", got)
	}
	if err := h.recognition.Work(ctx); err != nil {
		t.Fatal(err)
	}
	// Saving a field, even unchanged, confirms it: it is no longer marked.
	if !marked("reference") || !marked("date") {
		t.Fatal("the reference and date are not marked")
	}
	wantStatus(t, owner.do(http.MethodPatch, api+"/documents/purchase_invoices/"+id, map[string]any{"purchase_invoice": map[string]any{"reference": "EX-2026-7"}}), http.StatusOK)
	if marked("reference") || !marked("date") {
		t.Error("want only the reference confirmed")
	}

	// A document without attachments cannot be read.
	other := decode[struct {
		ID string `json:"id"`
	}](t, owner.do(http.MethodPost, typeless, map[string]any{"typeless_document": map[string]any{}}))
	wantStatus(t, owner.do(http.MethodPost, api+"/documents/"+other.ID+"/recognize", nil), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPost, api+"/documents/1/recognize", nil), http.StatusNotFound)
}

func TestRecognitionSettingsAndCheck(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	admin, member := h.client(), h.client()
	admin.register()
	member.register()

	r := admin.do(http.MethodPost, "/api/instance/settings/test_recognition", nil)
	wantStatus(t, r, http.StatusOK)
	got := decode[httpapi.RecognitionCheck](t, r)
	if got.OcrAvailable || got.Ai != httpapi.RecognitionCheckAiOff || got.Message == nil {
		t.Errorf("check = %+v", got)
	}
	wantStatus(t, member.do(http.MethodPost, "/api/instance/settings/test_recognition", nil), http.StatusForbidden)

	wantStatus(t, admin.do(http.MethodPatch, "/api/instance/settings", map[string]any{
		"recognition_ai_enabled": true, "recognition_ai_connections": []any{map[string]any{"provider": "anthropic"}},
	}), http.StatusUnprocessableEntity)
	const key, nvidiaKey = "ai-key-value", "nvapi-placeholder"
	r = admin.do(http.MethodPatch, "/api/instance/settings", map[string]any{
		"recognition_ai_enabled": true,
		"recognition_ai_connections": []any{
			map[string]any{"name": "NVIDIA free", "provider": "nvidia", "api_key": nvidiaKey},
			map[string]any{"provider": "anthropic", "model": "claude-opus-5-5", "api_key": key, "enabled": false},
		},
	})
	wantStatus(t, r, http.StatusOK)
	if strings.Contains(string(r.body), key) || strings.Contains(string(r.body), nvidiaKey) {
		t.Error("settings response contains an API key")
	}
	s := decode[httpapi.InstanceSettings](t, admin.do(http.MethodGet, "/api/instance/settings", nil))
	cs := s.RecognitionAiConnections
	if !s.RecognitionAiEnabled || len(cs) != 2 ||
		cs[0].Name != "NVIDIA free" || cs[0].Provider != httpapi.Nvidia || !cs[0].Enabled || !cs[0].ApiKeySet || cs[0].Id == "" ||
		cs[1].Name != "Anthropic" || cs[1].Model != "claude-opus-5-5" || cs[1].Enabled || !cs[1].ApiKeySet {
		t.Fatalf("settings = %+v", s)
	}
	// Reordered by id, the keys stay with their connections.
	r = admin.do(http.MethodPatch, "/api/instance/settings", map[string]any{
		"recognition_ai_connections": []any{
			map[string]any{"id": cs[1].Id, "name": "Claude", "provider": "anthropic", "enabled": true},
			map[string]any{"id": cs[0].Id, "name": "NVIDIA free", "provider": "nvidia", "api_key": "", "enabled": false},
		},
	})
	wantStatus(t, r, http.StatusOK)
	got2 := decode[httpapi.InstanceSettings](t, r).RecognitionAiConnections
	if len(got2) != 2 || got2[0].Id != cs[1].Id || !got2[0].ApiKeySet || !got2[0].Enabled || got2[1].Id != cs[0].Id || got2[1].ApiKeySet || got2[1].Enabled {
		t.Errorf("reordered = %+v", got2)
	}
	// This server has no AI models wired in the tests.
	if got := decode[httpapi.RecognitionCheck](t, admin.do(http.MethodPost, "/api/instance/settings/test_recognition", nil)); got.Ai != httpapi.RecognitionCheckAiUnavailable ||
		len(got.AiConnections) != 2 || got.AiConnections[0].State != httpapi.RecognitionConnectionCheckStateUnavailable ||
		got.AiConnections[1].State != httpapi.RecognitionConnectionCheckStateDisabled {
		t.Errorf("check with AI on = %+v", got)
	}
}
