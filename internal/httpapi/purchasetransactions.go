package httpapi

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

type purchaseTransactionJSON struct {
	ID                         string       `json:"id"`
	AdministrationID           string       `json:"administration_id"`
	FinancialAccountID         string       `json:"financial_account_id"`
	PurchaseTransactionBatchID *string      `json:"purchase_transaction_batch_id"`
	PayableType                *string      `json:"payable_type"`
	PayableID                  *string      `json:"payable_id"`
	Contact                    *contactJSON `json:"contact"`
	Amount                     string       `json:"amount"`
	Date                       *string      `json:"date"`
	Description                string       `json:"description"`
	EndToEndID                 string       `json:"end_to_end_id"`
	SepaIBAN                   string       `json:"sepa_iban"`
	SepaIBANAccountName        string       `json:"sepa_iban_account_name"`
	SepaBIC                    string       `json:"sepa_bic"`
	SourceSepaIBAN             *string      `json:"source_sepa_iban"`
	SourceSepaIBANAccountName  *string      `json:"source_sepa_iban_account_name"`
	PaymentInstrumentID        *string      `json:"payment_instrument_id"`
	PaymentMethod              *string      `json:"payment_method"`
	State                      string       `json:"state"`
	CreatedAt                  time.Time    `json:"created_at"`
	UpdatedAt                  time.Time    `json:"updated_at"`
	URI                        string       `json:"uri"`
}

func (s *server) toPurchaseTransactions(r *http.Request, admin resource.ID, list []purchases.Transaction) ([]purchaseTransactionJSON, error) {
	ids := make([]resource.ID, 0, len(list))
	for _, t := range list {
		if t.ContactID != nil {
			ids = append(ids, *t.ContactID)
		}
	}
	byID, err := s.contactsOf(r, admin, ids)
	if err != nil {
		return nil, err
	}
	out := make([]purchaseTransactionJSON, len(list))
	for i, t := range list {
		o := purchaseTransactionJSON{
			ID: t.ID.String(), AdministrationID: t.AdministrationID.String(), FinancialAccountID: t.FinancialAccountID.String(),
			PurchaseTransactionBatchID: idString(t.BatchID), PayableType: nz(t.PayableType), PayableID: idString(t.PayableID), Amount: t.Amount.String(),
			Date: dateText(t.Date), Description: t.Description, EndToEndID: t.EndToEndID, SepaIBAN: t.IBAN, SepaIBANAccountName: t.IBANName,
			SepaBIC: t.BIC, SourceSepaIBAN: nz(t.SourceIBAN), SourceSepaIBANAccountName: nz(t.SourceName), PaymentMethod: ptr("sepa_credit_transfer"),
			State: t.State(), CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, URI: t.Ref().URI(),
		}
		if t.ContactID != nil {
			if c := byID[*t.ContactID]; c != nil {
				cj := s.toContact(*c)
				o.Contact = &cj
			}
		}
		out[i] = o
	}
	return out, nil
}

type purchaseTransactionBatchJSON struct {
	ID                 string    `json:"id"`
	AdministrationID   string    `json:"administration_id"`
	FinancialAccountID string    `json:"financial_account_id"`
	Identifier         string    `json:"identifier"`
	State              string    `json:"state"`
	ExecutionDate      string    `json:"execution_date"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	URI                string    `json:"uri"`
}

func toPurchaseTransactionBatch(b purchases.Batch) purchaseTransactionBatchJSON {
	return purchaseTransactionBatchJSON{
		ID: b.ID.String(), AdministrationID: b.AdministrationID.String(), FinancialAccountID: b.FinancialAccountID.String(), Identifier: b.Identifier,
		State: b.State, ExecutionDate: b.ExecutionDate.Format(time.DateOnly), CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt, URI: b.Ref().URI(),
	}
}

// ListPurchaseTransactions implements ServerInterface.
func (s *server) ListPurchaseTransactions(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListPurchaseTransactionsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	now, err := s.today(r, admin)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	q, err := purchases.ParseTransactionFilter(deref((*string)(params.Filter)), now)
	if err != nil {
		writeError(w, http.StatusBadRequest, filterProblem(err))
		return
	}
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	list, total, err := s.deps.Purchases.Transactions(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toPurchaseTransactions(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeLinks(w, r, max(q.Page, 1), cmpPerPage(q.PerPage, purchases.DefaultPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

func (s *server) writePurchaseTransaction(w http.ResponseWriter, r *http.Request, admin resource.ID, status int, t purchases.Transaction, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toPurchaseTransactions(r, admin, []purchases.Transaction{t})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, out[0])
}

// CreatePurchaseTransaction implements ServerInterface.
func (s *server) CreatePurchaseTransaction(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		Transaction struct {
			PayableID          flexString `json:"payable_id"`
			FinancialAccountID flexString `json:"financial_account_id"`
			Date               string     `json:"date"`
		} `json:"purchase_transaction"`
	}](w, r)
	if !ok {
		return
	}
	payable, ok := flexID(w, &body.Transaction.PayableID)
	if !ok {
		return
	}
	account, ok := flexID(w, &body.Transaction.FinancialAccountID)
	if !ok {
		return
	}
	t, err := s.deps.Purchases.Pay(r.Context(), admin, user(r).ID, purchases.PayInput{DocumentID: *payable, FinancialAccountID: *account, Date: body.Transaction.Date})
	s.writePurchaseTransaction(w, r, admin, http.StatusCreated, t, err)
}

// GetPurchaseTransaction implements ServerInterface.
func (s *server) GetPurchaseTransaction(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	t, err := s.deps.Purchases.Transaction(r.Context(), admin, rid, user(r).ID)
	s.writePurchaseTransaction(w, r, admin, http.StatusOK, t, err)
}

// DeletePurchaseTransaction implements ServerInterface.
func (s *server) DeletePurchaseTransaction(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Purchases.DeleteTransaction(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeletePurchaseTransactionDeprecated implements ServerInterface.
func (s *server) DeletePurchaseTransactionDeprecated(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.DeletePurchaseTransaction(w, r, administrationID, id)
}

// CreatePurchaseTransactionBatch implements ServerInterface.
func (s *server) CreatePurchaseTransactionBatch(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		Batch struct {
			FinancialAccountID flexString   `json:"financial_account_id"`
			TransactionIDs     []flexString `json:"purchase_transaction_ids"`
			ExecutionDate      string       `json:"execution_date"`
		} `json:"purchase_transaction_batch"`
	}](w, r)
	if !ok {
		return
	}
	account, ok := flexID(w, &body.Batch.FinancialAccountID)
	if !ok {
		return
	}
	in := purchases.BatchInput{FinancialAccountID: *account, ExecutionDate: body.Batch.ExecutionDate}
	for i := range body.Batch.TransactionIDs {
		id, ok := flexID(w, &body.Batch.TransactionIDs[i])
		if !ok {
			return
		}
		in.TransactionIDs = append(in.TransactionIDs, *id)
	}
	b, err := s.deps.Purchases.CreateBatch(r.Context(), admin, user(r).ID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toPurchaseTransactionBatch(b))
}

// GetPurchaseTransactionBatch implements ServerInterface.
func (s *server) GetPurchaseTransactionBatch(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	b, err := s.deps.Purchases.Batch(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toPurchaseTransactionBatch(b))
}

// DownloadPurchaseTransactionBatch implements ServerInterface.
func (s *server) DownloadPurchaseTransactionBatch(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	file, b, err := s.deps.Purchases.DownloadBatch(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeDocument(w, "text/xml", b.Identifier+".xml", file)
}

// CreateSepaCreditTransfer implements ServerInterface.
func (s *server) CreateSepaCreditTransfer(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "File is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, 10<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "The file could not be read")
		return
	}
	in := purchases.ImportInput{File: data}
	for _, f := range []struct {
		name string
		dst  **resource.ID
	}{{"financial_account_id", &in.FinancialAccountID}, {"ledger_account_id", &in.LedgerAccountID}} {
		if v := strings.TrimSpace(r.FormValue(f.name)); v != "" {
			id, ok := parseID(w, v)
			if !ok {
				return
			}
			*f.dst = &id
		}
	}
	batches, err := s.deps.Purchases.ImportCreditTransfer(r.Context(), admin, user(r).ID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]purchaseTransactionBatchJSON, len(batches))
	for i, b := range batches {
		out[i] = toPurchaseTransactionBatch(b)
	}
	writeJSON(w, http.StatusCreated, out)
}
