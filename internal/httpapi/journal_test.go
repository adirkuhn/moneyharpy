package httpapi_test

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"testing"
)

type journalDocument struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	URI     string `json:"uri"`
	Rows    []struct {
		ID              string  `json:"id"`
		LedgerAccountID string  `json:"ledger_account_id"`
		ContactID       *string `json:"contact_id"`
		Debit           string  `json:"debit"`
		Credit          string  `json:"credit"`
	} `json:"general_journal_document_entries"`
	Notes       []struct{ ID string } `json:"notes"`
	Attachments []struct {
		ID             string `json:"id"`
		Filename       string `json:"filename"`
		AttachableType string `json:"attachable_type"`
	} `json:"attachments"`
}

// attach sends a file as multipart/form-data, checked against the contract.
func (c *client) attach(path, filename, contentType string, data []byte) response {
	c.t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, _ := form.CreatePart(header)
	_, _ = part.Write(data)
	_ = form.Close()
	req, err := http.NewRequest(http.MethodPost, c.base+path, &body)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	c.checkContract(req, res, raw)
	return response{status: res.StatusCode, header: res.Header, body: raw}
}

func TestJournalEntriesLifecycle(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	docs := api + "/documents/general_journal_documents"
	rent, deposits := category(t, owner, a.Id, "Rent"), category(t, owner, a.Id, "Private deposits")
	customer := decode[contact](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{"company_name": "Example Shop B.V."}}))

	unbalanced := owner.do(http.MethodPost, docs, map[string]any{"general_journal_document": map[string]any{
		"reference": "Rent", "date": "2026-11-05",
		"general_journal_document_entries_attributes": []map[string]any{{"ledger_account_id": rent, "debit": "400"}, {"ledger_account_id": deposits, "credit": "300"}},
	}})
	wantStatus(t, unbalanced, http.StatusUnprocessableEntity)

	created := owner.do(http.MethodPost, docs, map[string]any{"general_journal_document": map[string]any{
		"reference": "Rent paid privately", "date": "2026-11-05",
		"general_journal_document_entries_attributes": map[string]any{
			"0": map[string]any{"ledger_account_id": rent, "debit": 400, "contact_id": customer.ID},
			"1": map[string]any{"ledger_account_id": deposits, "credit": "400,00"},
		},
	}})
	wantStatus(t, created, http.StatusCreated)
	d := decode[journalDocument](t, created)
	if len(d.Rows) != 2 || d.Rows[0].Debit != "400.0" || d.Rows[1].Credit != "400.0" || d.Rows[0].ContactID == nil || d.URI != "/"+a.Id+"/documents/general_journal_documents/"+d.ID {
		t.Fatalf("created = %+v", d)
	}

	list := owner.do(http.MethodGet, docs+"?filter=period:202611..202611,contact_id:"+customer.ID, nil)
	if got := decode[[]journalDocument](t, list); len(got) != 1 || list.header.Get("X-Total-Count") != "1" {
		t.Errorf("filtered = %+v", got)
	}
	if got := decode[[]journalDocument](t, owner.do(http.MethodGet, docs+"?filter=period:prev_year", nil)); len(got) != 0 {
		t.Errorf("last year = %d", len(got))
	}

	updated := decode[journalDocument](t, owner.do(http.MethodPatch, docs+"/"+d.ID, map[string]any{"general_journal_document": map[string]any{
		"general_journal_document_entries_attributes": []map[string]any{{"id": d.Rows[0].ID, "debit": "450"}, {"id": d.Rows[1].ID, "credit": "450"}},
	}}))
	if updated.Rows[0].Debit != "450.0" {
		t.Errorf("updated = %+v", updated.Rows)
	}

	wantStatus(t, owner.do(http.MethodPost, docs+"/"+d.ID+"/notes", map[string]any{"note": map[string]any{"note": "Checked with the bank"}}), http.StatusCreated)
	pdf := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n")
	uploaded := owner.attach(docs+"/"+d.ID+"/attachments", "receipt.pdf", "application/pdf", pdf)
	wantStatus(t, uploaded, http.StatusOK)
	att := decode[struct {
		ID             string `json:"id"`
		AttachableType string `json:"attachable_type"`
	}](t, uploaded)
	if att.AttachableType != "Document" {
		t.Errorf("attachable type = %s", att.AttachableType)
	}
	wantStatus(t, owner.attach(docs+"/"+d.ID+"/attachments", "tool.exe", "application/octet-stream", []byte{0x4d, 0x5a, 0x90, 0x00}), http.StatusUnprocessableEntity)
	download := owner.do(http.MethodGet, docs+"/"+d.ID+"/attachments/"+att.ID+"/download", nil)
	wantStatus(t, download, http.StatusOK)
	if !bytes.Equal(download.body, pdf) || !strings.Contains(download.header.Get("Content-Disposition"), "receipt.pdf") {
		t.Errorf("download = %q %q", download.body, download.header.Get("Content-Disposition"))
	}
	full := decode[journalDocument](t, owner.do(http.MethodGet, docs+"/"+d.ID, nil))
	if len(full.Notes) != 1 || len(full.Attachments) != 1 {
		t.Errorf("notes and attachments = %+v", full)
	}
	wantStatus(t, owner.do(http.MethodDelete, docs+"/"+d.ID+"/attachments/"+att.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodDelete, docs+"/"+d.ID+"/notes/"+full.Notes[0].ID, nil), http.StatusNoContent)

	versions := decode[[]struct {
		ID string `json:"id"`
	}](t, owner.do(http.MethodGet, docs+"/synchronization?filter=period:this_year", nil))
	fetched := decode[[]journalDocument](t, owner.do(http.MethodPost, docs+"/synchronization", map[string]any{"ids": []string{d.ID}}))
	if len(versions) != 1 || len(fetched) != 1 {
		t.Errorf("sync = %+v / %d", versions, len(fetched))
	}

	// A contact in a journal entry stays; the debtors are settled by invoices only.
	wantStatus(t, owner.do(http.MethodDelete, api+"/contacts/"+customer.ID, nil), http.StatusUnprocessableEntity)
	debtors := category(t, owner, a.Id, "Accounts receivable")
	wantStatus(t, owner.do(http.MethodPost, docs, map[string]any{"general_journal_document": map[string]any{
		"reference": "Write off", "date": "2026-11-05",
		"general_journal_document_entries_attributes": []map[string]any{{"ledger_account_id": debtors, "credit": "10"}, {"ledger_account_id": rent, "debit": "10"}},
	}}), http.StatusUnprocessableEntity)

	// Locking the period protects the entry.
	wantStatus(t, owner.do(http.MethodPatch, api+"/bookkeeping_settings", map[string]any{"bookkeeping_settings": map[string]any{"period_locked_until": "2026-11-05"}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodDelete, docs+"/"+d.ID, nil), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPatch, api+"/bookkeeping_settings", map[string]any{"bookkeeping_settings": map[string]any{"period_locked_until": ""}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodDelete, docs+"/"+d.ID, nil), http.StatusNoContent)
}

type ledgerValues struct {
	LedgerAccounts []struct {
		LedgerAccountID string `json:"ledger_account_id"`
		Value           string `json:"value"`
	} `json:"ledger_accounts"`
}

func (v ledgerValues) of(id string) string {
	for _, l := range v.LedgerAccounts {
		if l.LedgerAccountID == id {
			return l.Value
		}
	}
	return ""
}

func TestJournalReports(t *testing.T) {
	h := newHarness(t, options{})
	owner, clerk := h.client(), h.client()
	owner.register()
	clerk.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	bank := decode[financialAccount](t, owner.do(http.MethodPost, api+"/financial_accounts", map[string]any{"financial_account": map[string]any{"name": "Main bank", "identifier": "NL91ABNA0417164300"}}))
	data, err := os.ReadFile("../bankfile/testdata/camt053.xml")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, owner.upload(api+"/financial_accounts/"+bank.ID+"/import", "application/octet-stream", data), http.StatusCreated)
	accounts := decode[[]ledgerAccount](t, owner.do(http.MethodGet, api+"/ledger_accounts", nil))
	var bankLedger string
	for _, l := range accounts {
		if l.Name == "Main bank" {
			bankLedger = l.ID
		}
	}

	report := decode[struct {
		DebitSums    ledgerValues `json:"debit_sums"`
		CreditSums   ledgerValues `json:"credit_sums"`
		FinalBalance ledgerValues `json:"final_balance"`
		Periods      []struct {
			Period string `json:"period"`
		} `json:"periods"`
	}](t, owner.do(http.MethodGet, api+"/reports/general_ledger?period=202610..202611&granularity=month", nil))
	if report.DebitSums.of(bankLedger) != "1000.0" || report.CreditSums.of(bankLedger) != "25.5" || report.FinalBalance.of(bankLedger) != "974.5" {
		t.Errorf("bank in the ledger = %+v", report)
	}
	if len(report.Periods) != 2 || report.Periods[1].Period != "20261101..20261130" {
		t.Errorf("periods = %+v", report.Periods)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/reports/general_ledger?period=20261105..20261110", nil), http.StatusUnprocessableEntity)

	entries := owner.do(http.MethodGet, api+"/reports/journal_entries?period=202611&ledger_account_id="+bankLedger, nil)
	got := decode[[]struct {
		DocumentType string `json:"document_type"`
		Amount       string `json:"amount"`
	}](t, entries)
	if len(got) != 2 || got[0].DocumentType != "FinancialMutation" || entries.header.Get("X-Total-Count") != "2" {
		t.Errorf("entries = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/reports/journal_entries?period=202611", nil), http.StatusUnprocessableEntity)

	invite(t, h, owner, clerk, a.Id, "clerk@example.com", map[string]any{"user_type": "employee", "permissions": []string{"sales_invoices"}})
	wantStatus(t, clerk.do(http.MethodGet, api+"/reports/general_ledger", nil), http.StatusForbidden)
	wantStatus(t, clerk.do(http.MethodGet, api+"/documents/general_journal_documents", nil), http.StatusForbidden)
}
