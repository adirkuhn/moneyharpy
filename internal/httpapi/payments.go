package httpapi

import (
	"net/http"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

type paymentJSON struct {
	ID                    string    `json:"id"`
	AdministrationID      string    `json:"administration_id"`
	InvoiceType           string    `json:"invoice_type"`
	InvoiceID             string    `json:"invoice_id"`
	FinancialAccountID    *string   `json:"financial_account_id"`
	FinancialMutationID   *string   `json:"financial_mutation_id"`
	LedgerAccountID       *string   `json:"ledger_account_id"`
	CreditInvoiceID       *string   `json:"credit_invoice_id"`
	LinkedPaymentID       *string   `json:"linked_payment_id"`
	PaymentTransactionID  *string   `json:"payment_transaction_id"`
	ManualPaymentAction   *string   `json:"manual_payment_action"`
	Price                 string    `json:"price"`
	PriceBase             string    `json:"price_base"`
	PaymentDate           string    `json:"payment_date"`
	TransactionIdentifier *string   `json:"transaction_identifier"`
	UserID                string    `json:"user_id"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func toPayment(p sales.Payment) paymentJSON {
	return paymentJSON{
		ID: p.ID.String(), AdministrationID: p.AdministrationID.String(), InvoiceType: p.InvoiceType, InvoiceID: p.InvoiceID.String(),
		FinancialAccountID: idString(p.FinancialAccountID), FinancialMutationID: idString(p.FinancialMutationID), LedgerAccountID: idString(p.LedgerAccountID),
		CreditInvoiceID: idString(p.CreditInvoiceID), LinkedPaymentID: idString(p.LinkedPaymentID), ManualPaymentAction: nz(p.ManualPaymentAction),
		Price: p.Price.String(), PriceBase: p.PriceBase.String(), PaymentDate: p.PaymentDate.Format(time.DateOnly),
		TransactionIdentifier: nz(p.TransactionIdentifier), UserID: p.UserID.String(), CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

type paymentBody struct {
	Payment struct {
		PaymentDate           string      `json:"payment_date"`
		Price                 *flexString `json:"price"`
		PriceBase             *flexString `json:"price_base"`
		FinancialAccountID    *flexString `json:"financial_account_id"`
		FinancialMutationID   *flexString `json:"financial_mutation_id"`
		TransactionIdentifier string      `json:"transaction_identifier"`
		ManualPaymentAction   string      `json:"manual_payment_action"`
		LedgerAccountID       *flexString `json:"ledger_account_id"`
		InvoiceID             *flexString `json:"invoice_id"`
	} `json:"payment"`
}

func (s *server) registerPayment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, status int) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[paymentBody](w, r)
	if !ok {
		return
	}
	in := body.Payment
	p := sales.PaymentInput{PaymentDate: in.PaymentDate, Price: in.Price.ptr(), PriceBase: in.PriceBase.ptr(),
		TransactionIdentifier: in.TransactionIdentifier, ManualPaymentAction: in.ManualPaymentAction}
	for _, f := range []struct {
		src *flexString
		dst **resource.ID
	}{{in.FinancialAccountID, &p.FinancialAccountID}, {in.FinancialMutationID, &p.FinancialMutationID}, {in.LedgerAccountID, &p.LedgerAccountID}, {in.InvoiceID, &p.InvoiceID}} {
		if f.src == nil || *f.src == "" {
			continue
		}
		if *f.dst, ok = flexID(w, f.src); !ok {
			return
		}
	}
	// Linking a bank transaction changes the bank's books too.
	if p.FinancialMutationID != nil && !s.requireAlso(w, r, admin, administration.Bank) {
		return
	}
	payment, err := s.deps.Sales.RegisterPayment(r.Context(), admin, rid, user(r).ID, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, toPayment(payment))
}

// CreateSalesInvoicePayment implements ServerInterface.
func (s *server) CreateSalesInvoicePayment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.registerPayment(w, r, administrationID, id, http.StatusCreated)
}

// RegisterSalesInvoicePayment implements ServerInterface.
func (s *server) RegisterSalesInvoicePayment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.registerPayment(w, r, administrationID, id, http.StatusOK)
}

// DeleteSalesInvoicePayment implements ServerInterface.
func (s *server) DeleteSalesInvoicePayment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, paymentID Identifier) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	pid, ok := parseID(w, paymentID)
	if !ok {
		return
	}
	if err := s.deps.Sales.DeletePayment(r.Context(), admin, rid, pid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SettleSalesCreditInvoice implements ServerInterface.
func (s *server) SettleSalesCreditInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	inv, err := s.deps.Sales.SettleCredit(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeSalesInvoice(w, r, admin, http.StatusOK, inv)
}

// MarkSalesInvoiceAsDubious implements ServerInterface.
func (s *server) MarkSalesInvoiceAsDubious(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	var body struct {
		DubiousDate string `json:"dubious_date"`
	}
	if r.ContentLength != 0 {
		if body, ok = decodeBody[struct {
			DubiousDate string `json:"dubious_date"`
		}](w, r); !ok {
			return
		}
	}
	inv, err := s.deps.Sales.MarkDubious(r.Context(), admin, rid, user(r).ID, body.DubiousDate)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeSalesInvoice(w, r, admin, http.StatusOK, inv)
}

// MarkSalesInvoiceAsUncollectible implements ServerInterface.
func (s *server) MarkSalesInvoiceAsUncollectible(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	type request struct {
		UncollectibleDate string `json:"uncollectible_date"`
		BookMethod        string `json:"book_method"`
	}
	var body request
	if r.ContentLength != 0 {
		if body, ok = decodeBody[request](w, r); !ok {
			return
		}
	}
	inv, err := s.deps.Sales.MarkUncollectible(r.Context(), admin, rid, user(r).ID, body.UncollectibleDate, body.BookMethod)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeSalesInvoice(w, r, admin, http.StatusOK, inv)
}

// SendSalesInvoiceReminders implements ServerInterface.
func (s *server) SendSalesInvoiceReminders(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		Reminders []struct {
			ContactID       flexString   `json:"contact_id"`
			WorkflowID      flexString   `json:"workflow_id"`
			DocumentStyleID flexString   `json:"document_style_id"`
			IdentityID      flexString   `json:"identity_id"`
			InvoiceIDs      []flexString `json:"sales_invoice_ids"`
			ReminderText    string       `json:"reminder_text"`
			DeliveryMethod  string       `json:"delivery_method"`
			EmailAddress    string       `json:"email_address"`
		} `json:"sales_invoice_reminders"`
	}](w, r)
	if !ok {
		return
	}
	in := make([]sales.ReminderInput, len(body.Reminders))
	for i, rem := range body.Reminders {
		in[i] = sales.ReminderInput{ReminderText: rem.ReminderText, DeliveryMethod: rem.DeliveryMethod, EmailAddress: rem.EmailAddress}
		for _, f := range []struct {
			src flexString
			dst *resource.ID
		}{{rem.ContactID, &in[i].ContactID}, {rem.WorkflowID, &in[i].WorkflowID}, {rem.DocumentStyleID, &in[i].DocumentStyleID}, {rem.IdentityID, &in[i].IdentityID}} {
			id, ok := flexID(w, &f.src)
			if !ok {
				return
			}
			*f.dst = *id
		}
		for j := range rem.InvoiceIDs {
			id, ok := flexID(w, &rem.InvoiceIDs[j])
			if !ok {
				return
			}
			in[i].InvoiceIDs = append(in[i].InvoiceIDs, *id)
		}
	}
	list, err := s.deps.Sales.SendReminders(r.Context(), admin, user(r).ID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toSalesInvoices(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
