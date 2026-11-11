package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// flexString takes a JSON string or number as text, for Moneybird fields
// that accept both, such as amounts and ids.
type flexString string

// UnmarshalJSON implements json.Unmarshaler.
func (f *flexString) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	var n json.Number
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := d.Decode(&n); err != nil {
		return fmt.Errorf("want a string or a number: %w", err)
	}
	*f = flexString(n.String())
	return nil
}

func (f *flexString) ptr() *string {
	if f == nil {
		return nil
	}
	s := string(*f)
	return &s
}

// flexID parses an id sent as a string or a number; it writes 400 when it
// is not one.
func flexID(w http.ResponseWriter, f *flexString) (*resource.ID, bool) {
	if f == nil {
		return nil, true
	}
	id, ok := parseID(w, string(*f))
	return &id, ok
}

func dateString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.DateOnly)
	return &s
}

// GetBankSettings implements ServerInterface.
func (s *server) GetBankSettings(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	st, err := s.deps.Banking.Settings(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, BankSettings{AutomaticBooking: st.AutoLink})
}

// UpdateBankSettings implements ServerInterface.
func (s *server) UpdateBankSettings(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[BankSettingsRequest](w, r)
	if !ok {
		return
	}
	st, err := s.deps.Banking.Settings(r.Context(), admin, user(r).ID)
	if err == nil && body.BankSettings.AutomaticBooking != nil {
		st, err = s.deps.Banking.SetAutoLink(r.Context(), admin, user(r).ID, *body.BankSettings.AutomaticBooking)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, BankSettings{AutomaticBooking: st.AutoLink})
}

type financialAccountJSON struct {
	ID               string     `json:"id"`
	AdministrationID string     `json:"administration_id"`
	Type             string     `json:"type"`
	Name             string     `json:"name"`
	Identifier       string     `json:"identifier"`
	Currency         string     `json:"currency"`
	Provider         *string    `json:"provider"`
	MoneybirdAccount bool       `json:"moneybird_account"`
	Active           bool       `json:"active"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	URI              string     `json:"uri"`
	Position         int        `json:"position"`
	LedgerAccountID  string     `json:"ledger_account_id"`
	LastImportAt     *time.Time `json:"last_import_at"`
}

func toFinancialAccount(a banking.Account) financialAccountJSON {
	return financialAccountJSON{
		ID: a.ID.String(), AdministrationID: a.AdministrationID.String(), Type: a.Type, Name: a.Name, Identifier: a.Identifier,
		Currency: a.Currency, Provider: emptyNil(a.Provider), Active: a.Active, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
		URI: a.Ref().URI(), Position: a.Position, LedgerAccountID: a.LedgerAccountID.String(), LastImportAt: a.LastImportAt,
	}
}

// ListFinancialAccounts implements ServerInterface.
func (s *server) ListFinancialAccounts(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	list, err := s.deps.Banking.Accounts(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]financialAccountJSON, len(list))
	for i, a := range list {
		out[i] = toFinancialAccount(a)
	}
	writeJSON(w, http.StatusOK, out)
}

func toAccountChanges(body FinancialAccountRequest) banking.AccountChanges {
	in := body.FinancialAccount
	return banking.AccountChanges{Type: (*string)(in.Type), Name: in.Name, Identifier: in.Identifier, Currency: in.Currency, Active: in.Active}
}

// CreateFinancialAccount implements ServerInterface.
func (s *server) CreateFinancialAccount(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[FinancialAccountRequest](w, r)
	if !ok {
		return
	}
	a, err := s.deps.Banking.CreateAccount(r.Context(), admin, user(r).ID, toAccountChanges(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toFinancialAccount(a))
}

// GetFinancialAccount implements ServerInterface.
func (s *server) GetFinancialAccount(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	a, err := s.deps.Banking.Account(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFinancialAccount(a))
}

// UpdateFinancialAccount implements ServerInterface.
func (s *server) UpdateFinancialAccount(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[FinancialAccountRequest](w, r)
	if !ok {
		return
	}
	a, err := s.deps.Banking.UpdateAccount(r.Context(), admin, rid, user(r).ID, toAccountChanges(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFinancialAccount(a))
}

// ReorderFinancialAccounts implements ServerInterface.
func (s *server) ReorderFinancialAccounts(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[IdsRequest](w, r)
	if !ok {
		return
	}
	ids := make([]resource.ID, len(body.Ids))
	for i, raw := range body.Ids {
		if ids[i], ok = parseID(w, raw); !ok {
			return
		}
	}
	if err := s.deps.Banking.ReorderAccounts(r.Context(), admin, user(r).ID, ids); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type importResultJSON struct {
	Format              string                   `json:"format"`
	Imported            int                      `json:"imported"`
	Duplicates          int                      `json:"duplicates"`
	Locked              int                      `json:"locked"`
	AutoBooked          int                      `json:"auto_booked"`
	FinancialStatements []financialStatementJSON `json:"financial_statements"`
}

// ImportFinancialStatement implements ServerInterface.
func (s *server) ImportFinancialStatement(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "The file is too large")
		return
	}
	res, err := s.deps.Banking.Import(r.Context(), admin, rid, user(r).ID, data)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := importResultJSON{Format: res.Format, Imported: res.Imported, Duplicates: res.Duplicates, Locked: res.Locked, AutoBooked: res.AutoBooked, FinancialStatements: []financialStatementJSON{}}
	for _, st := range res.Statements {
		out.FinancialStatements = append(out.FinancialStatements, toFinancialStatement(st))
	}
	writeJSON(w, http.StatusCreated, out)
}

type bookingJSON struct {
	ID                  string    `json:"id"`
	AdministrationID    string    `json:"administration_id"`
	FinancialMutationID string    `json:"financial_mutation_id"`
	LedgerAccountID     string    `json:"ledger_account_id"`
	ProjectID           *string   `json:"project_id"`
	Description         *string   `json:"description"`
	Price               string    `json:"price"`
	BookingRuleID       *string   `json:"booking_rule_id"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type financialMutationJSON struct {
	ID                    string        `json:"id"`
	AdministrationID      string        `json:"administration_id"`
	FinancialAccountID    string        `json:"financial_account_id"`
	FinancialStatementID  string        `json:"financial_statement_id"`
	Date                  string        `json:"date"`
	ValutationDate        *string       `json:"valutation_date"`
	Message               *string       `json:"message"`
	Amount                string        `json:"amount"`
	AmountOpen            string        `json:"amount_open"`
	OriginalAmount        *string       `json:"original_amount"`
	Currency              string        `json:"currency"`
	Code                  *string       `json:"code"`
	ContraAccountName     *string       `json:"contra_account_name"`
	ContraAccountNumber   string        `json:"contra_account_number"`
	BatchReference        *string       `json:"batch_reference"`
	AccountServicerTxID   *string       `json:"account_servicer_transaction_id"`
	State                 string        `json:"state"`
	SettlementState       string        `json:"settlement_state"`
	ProcessedAt           *time.Time    `json:"processed_at"`
	SepaFields            *struct{}     `json:"sepa_fields"`
	Payments              []paymentJSON `json:"payments"`
	LedgerAccountBookings []bookingJSON `json:"ledger_account_bookings"`
	Version               int64         `json:"version"`
	CreatedAt             time.Time     `json:"created_at"`
	UpdatedAt             time.Time     `json:"updated_at"`
	URI                   string        `json:"uri"`
}

func toFinancialMutation(m banking.Mutation) financialMutationJSON {
	out := financialMutationJSON{
		ID: m.ID.String(), AdministrationID: m.AdministrationID.String(), FinancialAccountID: m.AccountID.String(),
		FinancialStatementID: m.StatementID.String(), Date: m.Date.Format(time.DateOnly), ValutationDate: dateString(m.ValueDate),
		Message: emptyNil(m.Message), Amount: m.Amount.String(), AmountOpen: m.Open().String(), Currency: m.Currency, Code: emptyNil(m.Code),
		ContraAccountName: emptyNil(m.ContraName), ContraAccountNumber: m.ContraAccount, BatchReference: emptyNil(m.BatchReference),
		AccountServicerTxID: emptyNil(m.TransactionID), State: m.State, SettlementState: "settled", ProcessedAt: m.ProcessedAt,
		Payments: make([]paymentJSON, len(m.Payments)), LedgerAccountBookings: make([]bookingJSON, len(m.Bookings)), Version: m.Version(),
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, URI: m.Ref().URI(),
	}
	for i, p := range m.Payments {
		out.Payments[i] = paymentJSON{
			ID: p.ID.String(), AdministrationID: m.AdministrationID.String(), InvoiceType: p.InvoiceType, InvoiceID: p.InvoiceID.String(),
			FinancialAccountID: ptr(m.AccountID.String()), FinancialMutationID: ptr(m.ID.String()), ManualPaymentAction: emptyNil(p.ManualPaymentAction),
			Price: p.Price.String(), PriceBase: p.PriceBase.String(), PaymentDate: p.PaymentDate.Format(time.DateOnly),
			TransactionIdentifier: emptyNil(p.TransactionIdentifier), UserID: p.UserID.String(), CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		}
	}
	for i, b := range m.Bookings {
		out.LedgerAccountBookings[i] = bookingJSON{
			ID: b.ID.String(), AdministrationID: b.AdministrationID.String(), FinancialMutationID: b.MutationID.String(),
			LedgerAccountID: b.LedgerAccountID.String(), Description: emptyNil(b.Description), Price: b.Price.String(),
			CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
		}
		if b.RuleID != nil {
			out.LedgerAccountBookings[i].BookingRuleID = ptr(b.RuleID.String())
		}
	}
	return out
}

func toFinancialMutations(list []banking.Mutation) []financialMutationJSON {
	out := make([]financialMutationJSON, len(list))
	for i, m := range list {
		out[i] = toFinancialMutation(m)
	}
	return out
}

type financialStatementJSON struct {
	ID                 string                  `json:"id"`
	AdministrationID   string                  `json:"administration_id"`
	FinancialAccountID string                  `json:"financial_account_id"`
	Reference          string                  `json:"reference"`
	OfficialDate       *string                 `json:"official_date"`
	OfficialBalance    *string                 `json:"official_balance"`
	ImporterService    *string                 `json:"importer_service"`
	FinancialMutations []financialMutationJSON `json:"financial_mutations"`
	URI                string                  `json:"uri"`
}

func toFinancialStatement(st banking.Statement) financialStatementJSON {
	out := financialStatementJSON{
		ID: st.ID.String(), AdministrationID: st.AdministrationID.String(), FinancialAccountID: st.AccountID.String(), Reference: st.Reference,
		OfficialDate: dateString(st.OfficialDate), ImporterService: emptyNil(st.Importer), FinancialMutations: toFinancialMutations(st.Mutations), URI: st.Ref().URI(),
	}
	if st.OfficialBalance != nil {
		out.OfficialBalance = ptr(st.OfficialBalance.String())
	}
	return out
}

// statementBody is FinancialStatementRequest with Moneybird's flexible
// numbers and ids.
type statementBody struct {
	FinancialStatement struct {
		FinancialAccountID *flexString     `json:"financial_account_id"`
		Reference          *string         `json:"reference"`
		OfficialDate       *string         `json:"official_date"`
		OfficialBalance    *flexString     `json:"official_balance"`
		ImporterKey        *string         `json:"importer_key"`
		Manual             bool            `json:"manual"`
		Mutations          []mutationInput `json:"financial_mutations_attributes"`
	} `json:"financial_statement"`
}

type mutationInput struct {
	ID             *flexString `json:"id"`
	Destroy        bool        `json:"_destroy"`
	Date           *string     `json:"date"`
	ValueDate      *string     `json:"valutation_date"`
	Message        *string     `json:"message"`
	Amount         *flexString `json:"amount"`
	Code           *string     `json:"code"`
	ContraName     *string     `json:"contra_account_name"`
	ContraAccount  *string     `json:"contra_account_number"`
	BatchReference *string     `json:"batch_reference"`
	Offset         *int        `json:"offset"`
	TransactionID  *string     `json:"account_servicer_transaction_id"`
}

func toStatementInput(w http.ResponseWriter, body statementBody) (banking.StatementInput, bool) {
	in := body.FinancialStatement
	account, ok := flexID(w, in.FinancialAccountID)
	if !ok {
		return banking.StatementInput{}, false
	}
	out := banking.StatementInput{AccountID: account, Reference: in.Reference, OfficialDate: in.OfficialDate, OfficialBalance: in.OfficialBalance.ptr(), Manual: in.Manual}
	for _, m := range in.Mutations {
		id, ok := flexID(w, m.ID)
		if !ok {
			return banking.StatementInput{}, false
		}
		out.Mutations = append(out.Mutations, banking.MutationInput{
			ID: id, Destroy: m.Destroy, Date: m.Date, ValueDate: m.ValueDate, Message: m.Message, Amount: m.Amount.ptr(), Code: m.Code,
			ContraName: m.ContraName, ContraAccount: m.ContraAccount, BatchReference: m.BatchReference, Offset: m.Offset, TransactionID: m.TransactionID,
		})
	}
	return out, true
}

// CreateFinancialStatement implements ServerInterface.
func (s *server) CreateFinancialStatement(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[statementBody](w, r)
	if !ok {
		return
	}
	in, ok := toStatementInput(w, body)
	if !ok {
		return
	}
	st, err := s.deps.Banking.CreateStatement(r.Context(), admin, user(r).ID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toFinancialStatement(st))
}

// GetFinancialStatement implements ServerInterface.
func (s *server) GetFinancialStatement(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	st, err := s.deps.Banking.Statement(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFinancialStatement(st))
}

// UpdateFinancialStatement implements ServerInterface.
func (s *server) UpdateFinancialStatement(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[statementBody](w, r)
	if !ok {
		return
	}
	in, ok := toStatementInput(w, body)
	if !ok {
		return
	}
	st, err := s.deps.Banking.UpdateStatement(r.Context(), admin, rid, user(r).ID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFinancialStatement(st))
}

// DeleteFinancialStatement implements ServerInterface.
func (s *server) DeleteFinancialStatement(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Banking.DeleteStatement(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListFinancialMutations implements ServerInterface.
func (s *server) ListFinancialMutations(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListFinancialMutationsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	a, err := s.deps.Administrations.Get(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	today := time.Now()
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		today = today.In(loc)
	}
	f, err := banking.ParseMutationFilter(deref(params.Filter), today)
	if err != nil {
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "invalid fields: "))
		return
	}
	list, err := s.deps.Banking.Mutations(r.Context(), admin, user(r).ID, f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFinancialMutations(list))
}

// GetFinancialMutation implements ServerInterface.
func (s *server) GetFinancialMutation(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	m, err := s.deps.Banking.Mutation(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFinancialMutation(m))
}

// ListFinancialMutationVersions implements ServerInterface.
func (s *server) ListFinancialMutationVersions(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	list, err := s.deps.Banking.MutationVersions(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]SynchronizationItem, len(list))
	for i, v := range list {
		out[i] = SynchronizationItem{Id: v.ID.String(), Version: int(v.Version)}
	}
	writeJSON(w, http.StatusOK, out)
}

// FetchFinancialMutations implements ServerInterface.
func (s *server) FetchFinancialMutations(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		IDs []flexString `json:"ids"`
	}](w, r)
	if !ok {
		return
	}
	ids := make([]resource.ID, len(body.IDs))
	for i := range body.IDs {
		id, ok := flexID(w, &body.IDs[i])
		if !ok {
			return
		}
		ids[i] = *id
	}
	list, err := s.deps.Banking.MutationsByID(r.Context(), admin, user(r).ID, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFinancialMutations(list))
}

// LinkFinancialMutationBooking implements ServerInterface.
func (s *server) LinkFinancialMutationBooking(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		BookingType string      `json:"booking_type"`
		BookingID   *flexString `json:"booking_id"`
		PriceBase   *flexString `json:"price_base"`
		Price       *flexString `json:"price"`
		Description string      `json:"description"`
	}](w, r)
	if !ok {
		return
	}
	booking, ok := flexID(w, body.BookingID)
	if !ok {
		return
	}
	price := body.PriceBase.ptr()
	if price == nil {
		price = body.Price.ptr()
	}
	m, err := s.deps.Banking.LinkBooking(r.Context(), admin, rid, user(r).ID, banking.Link{Type: body.BookingType, BookingID: booking, Price: price, Description: body.Description})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFinancialMutation(m))
}

// UnlinkFinancialMutationBooking implements ServerInterface.
func (s *server) UnlinkFinancialMutationBooking(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		BookingType string     `json:"booking_type"`
		BookingID   flexString `json:"booking_id"`
	}](w, r)
	if !ok {
		return
	}
	booking, ok := flexID(w, &body.BookingID)
	if !ok {
		return
	}
	m, err := s.deps.Banking.UnlinkBooking(r.Context(), admin, rid, user(r).ID, body.BookingType, *booking)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFinancialMutation(m))
}

// GetBankBalance implements ServerInterface.
func (s *server) GetBankBalance(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	list, total, err := s.deps.Banking.Balances(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type accountBalance struct {
		FinancialAccountID string `json:"financial_account_id"`
		Name               string `json:"name"`
		Type               string `json:"type"`
		Balance            string `json:"balance"`
	}
	out := struct {
		Total             string           `json:"total"`
		FinancialAccounts []accountBalance `json:"financial_accounts"`
	}{Total: total.String(), FinancialAccounts: []accountBalance{}}
	for _, b := range list {
		out.FinancialAccounts = append(out.FinancialAccounts, accountBalance{FinancialAccountID: b.Account.ID.String(), Name: b.Account.Name, Type: b.Account.Type, Balance: b.Balance.String()})
	}
	writeJSON(w, http.StatusOK, out)
}

func toBookingRule(r banking.Rule) BookingRule {
	out := BookingRule{
		Id: r.ID.String(), AdministrationId: r.AdministrationID.String(), Name: r.Name, Active: r.Active, Position: r.Position, MatchAll: r.MatchAll,
		Direction: BookingRuleDirection(r.Direction), Conditions: make([]RuleCondition, len(r.Conditions)), LedgerAccountId: r.LedgerAccountID.String(),
		Description: r.Description, Uri: r.Ref().URI(), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if r.AccountID != nil {
		out.FinancialAccountId = ptr(r.AccountID.String())
	}
	for i, c := range r.Conditions {
		out.Conditions[i] = RuleCondition{Field: RuleConditionField(c.Field), Operator: RuleConditionOperator(c.Operator), Value: c.Value}
	}
	return out
}

func toRuleChanges(w http.ResponseWriter, body BookingRuleRequest) (banking.RuleChanges, bool) {
	in := body.BookingRule
	c := banking.RuleChanges{Name: in.Name, Active: in.Active, MatchAll: in.MatchAll, Direction: (*string)(in.Direction), Description: in.Description}
	if in.FinancialAccountId != nil {
		account := resource.ID(0)
		if *in.FinancialAccountId != "" {
			var ok bool
			if account, ok = parseID(w, *in.FinancialAccountId); !ok {
				return c, false
			}
		}
		c.AccountID = &account
	}
	if in.LedgerAccountId != nil {
		id, ok := parseID(w, *in.LedgerAccountId)
		if !ok {
			return c, false
		}
		c.LedgerAccountID = &id
	}
	if in.Conditions != nil {
		conditions := make([]banking.Condition, len(*in.Conditions))
		for i, rc := range *in.Conditions {
			conditions[i] = banking.Condition{Field: string(rc.Field), Operator: string(rc.Operator), Value: rc.Value}
		}
		c.Conditions = &conditions
	}
	return c, true
}

// ListBookingRules implements ServerInterface.
func (s *server) ListBookingRules(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	list, err := s.deps.Banking.Rules(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]BookingRule, len(list))
	for i, rule := range list {
		out[i] = toBookingRule(rule)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateBookingRule implements ServerInterface.
func (s *server) CreateBookingRule(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[BookingRuleRequest](w, r)
	if !ok {
		return
	}
	c, ok := toRuleChanges(w, body)
	if !ok {
		return
	}
	rule, err := s.deps.Banking.CreateRule(r.Context(), admin, user(r).ID, c)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toBookingRule(rule))
}

// GetBookingRule implements ServerInterface.
func (s *server) GetBookingRule(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	rule, err := s.deps.Banking.Rule(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBookingRule(rule))
}

// UpdateBookingRule implements ServerInterface.
func (s *server) UpdateBookingRule(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[BookingRuleRequest](w, r)
	if !ok {
		return
	}
	c, ok := toRuleChanges(w, body)
	if !ok {
		return
	}
	rule, err := s.deps.Banking.UpdateRule(r.Context(), admin, rid, user(r).ID, c)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBookingRule(rule))
}

// DeleteBookingRule implements ServerInterface.
func (s *server) DeleteBookingRule(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Banking.DeleteRule(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
