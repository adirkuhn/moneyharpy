package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// attachableTypes maps record kinds to the attachable_type Moneybird
// reports; its journal entries are documents.
var attachableTypes = map[string]string{
	journal.GeneralJournalDocument: "Document", journal.SalesInvoice: "SalesInvoice",
	purchases.PurchaseInvoice: "Document", purchases.Receipt: "Document", purchases.GeneralDocument: "Document", purchases.TypelessDocument: "Document",
}

type attachmentJSON struct {
	ID               string    `json:"id"`
	AdministrationID string    `json:"administration_id"`
	AttachableID     string    `json:"attachable_id"`
	AttachableType   string    `json:"attachable_type"`
	Filename         string    `json:"filename"`
	ContentType      string    `json:"content_type"`
	Size             int64     `json:"size"`
	Rotation         *int      `json:"rotation"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func toAttachment(a attachments.Attachment) attachmentJSON {
	kind := attachableTypes[a.EntityType]
	if kind == "" {
		kind = a.EntityType
	}
	return attachmentJSON{
		ID: a.ID.String(), AdministrationID: a.AdministrationID.String(), AttachableID: a.EntityID.String(), AttachableType: kind,
		Filename: a.Filename, ContentType: a.ContentType, Size: a.Size, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

type journalRowJSON struct {
	ID               string    `json:"id"`
	AdministrationID string    `json:"administration_id"`
	LedgerAccountID  string    `json:"ledger_account_id"`
	ProjectID        *string   `json:"project_id"`
	ContactID        *string   `json:"contact_id"`
	Debit            string    `json:"debit"`
	Credit           string    `json:"credit"`
	RowOrder         int       `json:"row_order"`
	Description      *string   `json:"description"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type journalDocumentJSON struct {
	ID               string           `json:"id"`
	AdministrationID string           `json:"administration_id"`
	Reference        string           `json:"reference"`
	Date             string           `json:"date"`
	JournalType      *string          `json:"journal_type"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	Version          int64            `json:"version"`
	Rows             []journalRowJSON `json:"general_journal_document_entries"`
	Notes            []noteJSON       `json:"notes"`
	Attachments      []attachmentJSON `json:"attachments"`
	Events           []any            `json:"events"`
	URI              string           `json:"uri"`
}

func toJournalDocument(d journal.Document) journalDocumentJSON {
	out := journalDocumentJSON{
		ID: d.ID.String(), AdministrationID: d.AdministrationID.String(), Reference: d.Reference, Date: d.Date.Format(time.DateOnly),
		JournalType: nz(d.JournalType), CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, Version: d.Version(), Rows: []journalRowJSON{},
		Notes: []noteJSON{}, Attachments: []attachmentJSON{}, Events: []any{}, URI: d.Ref().URI(),
	}
	for _, r := range d.Rows {
		out.Rows = append(out.Rows, journalRowJSON{
			ID: r.ID.String(), AdministrationID: d.AdministrationID.String(), LedgerAccountID: r.LedgerAccountID.String(), ProjectID: idString(r.ProjectID),
			ContactID: idString(r.ContactID), Debit: r.Debit.String(), Credit: r.Credit.String(), RowOrder: r.RowOrder, Description: nz(r.Description),
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	for _, n := range d.Notes {
		out.Notes = append(out.Notes, toNote(n))
	}
	for _, a := range d.Attachments {
		out.Attachments = append(out.Attachments, toAttachment(a))
	}
	return out
}

func toJournalDocuments(list []journal.Document) []journalDocumentJSON {
	out := make([]journalDocumentJSON, len(list))
	for i, d := range list {
		out[i] = toJournalDocument(d)
	}
	return out
}

type journalRowBody struct {
	ID              *flexString `json:"id"`
	LedgerAccountID *flexString `json:"ledger_account_id"`
	ContactID       *flexString `json:"contact_id"`
	ProjectID       *flexString `json:"project_id"`
	TaxRateID       *flexString `json:"tax_rate_id"`
	Debit           *flexString `json:"debit"`
	Credit          *flexString `json:"credit"`
	Description     *string     `json:"description"`
	RowOrder        *flexString `json:"row_order"`
	Destroy         *flexBool   `json:"_destroy"`
}

// journalRows reads rows sent as an array or as an object keyed by position.
type journalRows []journalRowBody

// UnmarshalJSON implements json.Unmarshaler.
func (j *journalRows) UnmarshalJSON(b []byte) error {
	type plain journalRows
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte("[")) {
		return json.Unmarshal(b, (*plain)(j))
	}
	var keyed map[string]journalRowBody
	if err := json.Unmarshal(b, &keyed); err != nil {
		return err
	}
	keys := make([]string, 0, len(keyed))
	for k := range keyed {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, c int) bool {
		x, errX := strconv.Atoi(keys[a])
		y, errY := strconv.Atoi(keys[c])
		if errX == nil && errY == nil {
			return x < y
		}
		return keys[a] < keys[c]
	})
	for _, k := range keys {
		*j = append(*j, keyed[k])
	}
	return nil
}

type journalDocumentBody struct {
	Document struct {
		Reference   *string      `json:"reference"`
		Date        *string      `json:"date"`
		JournalType *string      `json:"journal_type"`
		Rows        *journalRows `json:"general_journal_document_entries_attributes"`
	} `json:"general_journal_document"`
}

func toDocumentChanges(w http.ResponseWriter, body journalDocumentBody) (journal.DocumentChanges, bool) {
	in := body.Document
	c := journal.DocumentChanges{Reference: in.Reference, Date: in.Date, JournalType: in.JournalType}
	if in.Rows == nil {
		return c, true
	}
	c.Rows = []journal.RowInput{}
	for _, r := range *in.Rows {
		row := journal.RowInput{Debit: r.Debit.ptr(), Credit: r.Credit.ptr(), Description: r.Description, Destroy: deref(r.Destroy.ptr())}
		var ok bool
		if r.ID != nil && *r.ID != "" {
			if row.ID, ok = flexID(w, r.ID); !ok {
				return c, false
			}
		}
		if row.LedgerAccountID, ok = flexOptionalID(w, r.LedgerAccountID); !ok {
			return c, false
		}
		if row.ContactID, ok = flexOptionalID(w, r.ContactID); !ok {
			return c, false
		}
		if row.ProjectID, ok = flexOptionalID(w, r.ProjectID); !ok {
			return c, false
		}
		if r.RowOrder != nil {
			n, err := strconv.Atoi(string(*r.RowOrder))
			if err != nil {
				writeError(w, http.StatusBadRequest, "row_order is not a number")
				return c, false
			}
			row.RowOrder = &n
		}
		c.Rows = append(c.Rows, row)
	}
	return c, true
}

func (s *server) today(r *http.Request, admin resource.ID) (time.Time, error) {
	a, err := s.deps.Administrations.Get(r.Context(), admin, user(r).ID)
	if err != nil {
		return time.Time{}, err
	}
	now := time.Now()
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		now = now.In(loc)
	}
	return now, nil
}

func (s *server) documentQuery(w http.ResponseWriter, r *http.Request, admin resource.ID, filter *DocumentFilter) (journal.DocumentQuery, bool) {
	now, err := s.today(r, admin)
	if err != nil {
		s.fail(w, r, err)
		return journal.DocumentQuery{}, false
	}
	q, err := journal.ParseDocumentFilter(deref((*string)(filter)), now)
	if err != nil {
		var verr *resource.ValidationError
		if errors.As(err, &verr) {
			writeError(w, http.StatusBadRequest, strings.Join(verr.Fields["filter"], "; "))
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return q, false
	}
	return q, true
}

// ListGeneralJournalDocuments implements ServerInterface.
func (s *server) ListGeneralJournalDocuments(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListGeneralJournalDocumentsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	filter := params.Filter
	if params.NewFilter != nil {
		filter = (*DocumentFilter)(params.NewFilter)
	}
	q, ok := s.documentQuery(w, r, admin, filter)
	if !ok {
		return
	}
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	q.Ascending = params.Sort != nil && *params.Sort == "date_asc"
	list, total, err := s.deps.Journal.Documents(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeLinks(w, r, max(q.Page, 1), cmpPerPage(q.PerPage, 50), total)
	writeJSON(w, http.StatusOK, toJournalDocuments(list))
}

func cmpPerPage(n, fallback int) int {
	if n <= 0 {
		return fallback
	}
	return min(n, journal.MaxPerPage)
}

// CreateGeneralJournalDocument implements ServerInterface.
func (s *server) CreateGeneralJournalDocument(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[journalDocumentBody](w, r)
	if !ok {
		return
	}
	c, ok := toDocumentChanges(w, body)
	if !ok {
		return
	}
	d, err := s.deps.Journal.CreateDocument(r.Context(), admin, user(r).ID, c)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toJournalDocument(d))
}

// ListGeneralJournalDocumentVersions implements ServerInterface.
func (s *server) ListGeneralJournalDocumentVersions(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListGeneralJournalDocumentVersionsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := s.documentQuery(w, r, admin, params.Filter)
	if !ok {
		return
	}
	list, err := s.deps.Journal.DocumentVersions(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]SynchronizationItem, len(list))
	for i, d := range list {
		out[i] = SynchronizationItem{Id: d.ID.String(), Version: int(d.Version())}
	}
	writeJSON(w, http.StatusOK, out)
}

// FetchGeneralJournalDocuments implements ServerInterface.
func (s *server) FetchGeneralJournalDocuments(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
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
	list, err := s.deps.Journal.DocumentsByID(r.Context(), admin, user(r).ID, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toJournalDocuments(list))
}

// GetGeneralJournalDocument implements ServerInterface.
func (s *server) GetGeneralJournalDocument(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	d, err := s.deps.Journal.Document(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toJournalDocument(d))
}

// UpdateGeneralJournalDocument implements ServerInterface.
func (s *server) UpdateGeneralJournalDocument(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[journalDocumentBody](w, r)
	if !ok {
		return
	}
	c, ok := toDocumentChanges(w, body)
	if !ok {
		return
	}
	d, err := s.deps.Journal.UpdateDocument(r.Context(), admin, rid, user(r).ID, c)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toJournalDocument(d))
}

// DeleteGeneralJournalDocument implements ServerInterface.
func (s *server) DeleteGeneralJournalDocument(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Journal.DeleteDocument(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	// Moneybird answers with the status code as the body.
	writeJSON(w, http.StatusOK, http.StatusOK)
}

func decodeNote(w http.ResponseWriter, r *http.Request) (notes.Input, bool) {
	body, ok := decodeBody[struct {
		Note struct {
			Note       string      `json:"note"`
			Todo       *flexBool   `json:"todo"`
			AssigneeID *flexString `json:"assignee_id"`
		} `json:"note"`
	}](w, r)
	if !ok {
		return notes.Input{}, false
	}
	in := notes.Input{Note: body.Note.Note, Todo: deref(body.Note.Todo.ptr())}
	if body.Note.AssigneeID != nil && *body.Note.AssigneeID != "" {
		id, ok := flexID(w, body.Note.AssigneeID)
		if !ok {
			return in, false
		}
		in.AssigneeID = id
	}
	return in, true
}

// CreateGeneralJournalDocumentNote implements ServerInterface.
func (s *server) CreateGeneralJournalDocumentNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	in, ok := decodeNote(w, r)
	if !ok {
		return
	}
	n, err := s.deps.Journal.AddDocumentNote(r.Context(), admin, rid, user(r).ID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toNote(n))
}

// DeleteGeneralJournalDocumentNote implements ServerInterface.
func (s *server) DeleteGeneralJournalDocumentNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, noteID Identifier) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	note, ok := parseID(w, noteID)
	if !ok {
		return
	}
	if err := s.deps.Journal.DeleteDocumentNote(r.Context(), admin, rid, note, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readUpload reads the file of a multipart form.
func readUpload(w http.ResponseWriter, r *http.Request) (string, string, []byte, bool) {
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"file": {"can't be blank"}}))
		return "", "", nil, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, attachments.MaxSize+1))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "The file is too large")
		return "", "", nil, false
	}
	return header.Filename, header.Header.Get("Content-Type"), data, true
}

// CreateGeneralJournalDocumentAttachment implements ServerInterface.
func (s *server) CreateGeneralJournalDocumentAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	name, contentType, data, ok := readUpload(w, r)
	if !ok {
		return
	}
	a, err := s.deps.Journal.AddDocumentAttachment(r.Context(), admin, rid, user(r).ID, name, contentType, data)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAttachment(a))
}

// DeleteGeneralJournalDocumentAttachment implements ServerInterface.
func (s *server) DeleteGeneralJournalDocumentAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	att, ok := parseID(w, attachmentID)
	if !ok {
		return
	}
	if err := s.deps.Journal.DeleteDocumentAttachment(r.Context(), admin, rid, att, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DownloadGeneralJournalDocumentAttachment implements ServerInterface.
func (s *server) DownloadGeneralJournalDocumentAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	s.redirectToAttachment(w, r, fileJournalAttachment, administrationID, id, attachmentID)
}

// writeFile answers with an attachment's contents.
func writeFile(w http.ResponseWriter, a attachments.Attachment, body io.ReadCloser) {
	defer func() { _ = body.Close() }()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": a.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.FormatInt(a.Size, 10))
	w.WriteHeader(http.StatusOK)
	// The client went away if this fails; there is nobody left to tell.
	_, _ = io.Copy(w, body)
}

type journalEntryJSON struct {
	ID                 string  `json:"id"`
	Date               string  `json:"date"`
	DocumentType       string  `json:"document_type"`
	DocumentID         string  `json:"document_id"`
	LedgerAccountID    string  `json:"ledger_account_id"`
	ProjectID          *string `json:"project_id"`
	ContactID          *string `json:"contact_id"`
	TaxRateID          *string `json:"tax_rate_id"`
	FinancialAccountID *string `json:"financial_account_id"`
	Amount             string  `json:"amount"`
	Description        *string `json:"description"`
}

// optionalSubject reads a report filter that is an id or "null".
func optionalSubject(w http.ResponseWriter, v *string) (*resource.ID, bool, bool) {
	if v == nil {
		return nil, false, true
	}
	if *v == "null" {
		return nil, true, true
	}
	id, ok := parseID(w, *v)
	return &id, false, ok
}

// GetJournalEntriesReport implements ServerInterface.
func (s *server) GetJournalEntriesReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetJournalEntriesReportParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	var q journal.EntryQuery
	if params.LedgerAccountId != nil {
		id, ok := parseID(w, *params.LedgerAccountId)
		if !ok {
			return
		}
		q.LedgerAccountID = &id
	}
	if q.ContactID, q.NoContact, ok = optionalSubject(w, params.ContactId); !ok {
		return
	}
	if q.ProjectID, q.NoProject, ok = optionalSubject(w, params.ProjectId); !ok {
		return
	}
	if params.AccountType != nil {
		q.AccountType = string(*params.AccountType)
	}
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	list, total, err := s.deps.Journal.Entries(r.Context(), admin, user(r).ID, deref((*string)(params.Period)), q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]journalEntryJSON, len(list))
	for i, e := range list {
		out[i] = journalEntryJSON{
			ID: e.ID.String(), Date: e.Date.Format(time.DateOnly), DocumentType: e.DocumentType, DocumentID: e.DocumentID.String(),
			LedgerAccountID: e.LedgerAccountID.String(), ProjectID: idString(e.ProjectID), ContactID: idString(e.ContactID),
			TaxRateID: idString(e.TaxRateID), FinancialAccountID: idString(e.FinancialAccountID), Amount: e.Amount.String(), Description: nz(e.Description),
		}
	}
	s.writeLinks(w, r, max(q.Page, 1), cmpPerPage(q.PerPage, journal.DefaultPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

type ledgerValue struct {
	LedgerAccountID string `json:"ledger_account_id"`
	Value           string `json:"value"`
}

type ledgerValues struct {
	LedgerAccounts []ledgerValue `json:"ledger_accounts"`
}

type ledgerPeriodJSON struct {
	Period       string       `json:"period,omitempty"`
	DebitSums    ledgerValues `json:"debit_sums"`
	CreditSums   ledgerValues `json:"credit_sums"`
	StartBalance ledgerValues `json:"start_balance"`
	FinalBalance ledgerValues `json:"final_balance"`
}

func toLedgerPeriod(p *money.Period, sums []journal.AccountSum) ledgerPeriodJSON {
	out := ledgerPeriodJSON{
		DebitSums: ledgerValues{[]ledgerValue{}}, CreditSums: ledgerValues{[]ledgerValue{}},
		StartBalance: ledgerValues{[]ledgerValue{}}, FinalBalance: ledgerValues{[]ledgerValue{}},
	}
	if p != nil {
		out.Period = fmt.Sprintf("%s..%s", p.From.Format("20060102"), p.To.Format("20060102"))
	}
	for _, a := range sums {
		id := a.LedgerAccountID.String()
		out.DebitSums.LedgerAccounts = append(out.DebitSums.LedgerAccounts, ledgerValue{id, a.Debit.String()})
		out.CreditSums.LedgerAccounts = append(out.CreditSums.LedgerAccounts, ledgerValue{id, a.Credit.String()})
		out.StartBalance.LedgerAccounts = append(out.StartBalance.LedgerAccounts, ledgerValue{id, a.Start.String()})
		out.FinalBalance.LedgerAccounts = append(out.FinalBalance.LedgerAccounts, ledgerValue{id, a.Final.String()})
	}
	return out
}

// GetGeneralLedgerReport implements ServerInterface.
func (s *server) GetGeneralLedgerReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetGeneralLedgerReportParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	report, err := s.deps.Journal.GeneralLedger(r.Context(), admin, user(r).ID, deref((*string)(params.Period)), string(deref(params.Granularity)))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type reportJSON struct {
		ledgerPeriodJSON
		Periods []ledgerPeriodJSON `json:"periods,omitempty"`
	}
	out := reportJSON{ledgerPeriodJSON: toLedgerPeriod(nil, report.Sums)}
	for _, p := range report.Periods {
		out.Periods = append(out.Periods, toLedgerPeriod(&p.Period, p.Sums))
	}
	writeJSON(w, http.StatusOK, out)
}
