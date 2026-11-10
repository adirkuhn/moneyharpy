package httpapi

import (
	"io"
	"net/http"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// TestStorage implements ServerInterface. The storage's reason is shown
// because only instance administrators can reach this and need it to fix setup.
func (s *server) TestStorage(w http.ResponseWriter, r *http.Request) {
	if !requireInstanceAdmin(w, r) {
		return
	}
	if err := s.deps.Storage.Check(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toInvoiceSettings(v invoicing.SettingsView) InvoiceSettings {
	return InvoiceSettings{
		InvoiceIdTemplate: v.InvoiceIDTemplate, InvoiceSequenceId: v.InvoiceSequenceID, InvoiceIdExample: v.InvoiceIDExample,
		EstimateIdTemplate: v.EstimateIDTemplate, EstimateSequenceId: v.EstimateSequenceID, EstimateIdExample: v.EstimateIDExample,
		InvoiceBccEmailAddress: v.InvoiceBCC, InvoiceRemindersBccEnabled: v.InvoiceRemindersBCC,
		EstimateBccEmailAddress: v.EstimateBCC, OrderBccEmailAddress: v.OrderBCC,
		DefaultEmailUbl: v.DefaultEmailUBL, PaymentsWithoutProof: v.PaymentsWithoutProof,
	}
}

// GetInvoiceSettings implements ServerInterface.
func (s *server) GetInvoiceSettings(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	v, err := s.deps.Invoicing.Settings(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toInvoiceSettings(v))
}

// UpdateInvoiceSettings implements ServerInterface.
func (s *server) UpdateInvoiceSettings(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[InvoiceSettingsRequest](w, r)
	if !ok {
		return
	}
	in := body.InvoiceSettings
	v, err := s.deps.Invoicing.UpdateSettings(r.Context(), admin, user(r).ID, invoicing.SettingsChanges{
		InvoiceIDTemplate: in.InvoiceIdTemplate, InvoiceSequenceID: in.InvoiceSequenceId,
		EstimateIDTemplate: in.EstimateIdTemplate, EstimateSequenceID: in.EstimateSequenceId,
		InvoiceBCC: in.InvoiceBccEmailAddress, InvoiceRemindersBCC: in.InvoiceRemindersBccEnabled,
		EstimateBCC: in.EstimateBccEmailAddress, OrderBCC: in.OrderBccEmailAddress,
		DefaultEmailUBL: in.DefaultEmailUbl, PaymentsWithoutProof: in.PaymentsWithoutProof,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toInvoiceSettings(v))
}

func toWorkflow(wf invoicing.Workflow) Workflow {
	admin := wf.AdministrationID.String()
	steps := make([]WorkflowStepResponse, len(wf.Steps))
	for i, st := range wf.Steps {
		var text *string
		if st.ReminderText != "" {
			text = &st.ReminderText
		}
		steps[i] = WorkflowStepResponse{
			Id: ptr(st.ID.String()), AdministrationId: &admin, StepOrder: ptr(st.StepOrder), DueInterval: ptr(st.DueInterval),
			ReminderDelay: ptr(st.ReminderDelay), ReminderText: text, ReminderAutoSend: ptr(st.ReminderAutoSend),
			CreatedAt: ptr(st.CreatedAt), UpdatedAt: ptr(st.UpdatedAt),
		}
	}
	return Workflow{
		Id: ptr(wf.ID.String()), AdministrationId: &admin, Type: ptr(string(wf.Type)), Name: &wf.Name, Default: &wf.Default,
		Currency: &wf.Currency, Language: &wf.Language, Active: &wf.Active, PricesAreInclTax: &wf.PricesAreInclTax,
		NumberOfCollections: ptr(0), ShowQrCode: &wf.ShowQRCode, Steps: &steps,
		CreatedAt: ptr(wf.CreatedAt), UpdatedAt: ptr(wf.UpdatedAt), Uri: wf.Ref().URI(),
	}
}

func toWorkflowChanges(body WorkflowRequest) invoicing.WorkflowChanges {
	in := body.Workflow
	c := invoicing.WorkflowChanges{
		Name: in.Name, Currency: in.Currency, Language: (*string)(in.Language), Active: in.Active, Default: in.Default,
		PricesAreInclTax: in.PricesAreInclTax, ShowQRCode: in.ShowQrCode,
	}
	if in.StepsAttributes != nil {
		steps := make([]invoicing.StepInput, len(*in.StepsAttributes))
		for i, st := range *in.StepsAttributes {
			steps[i] = invoicing.StepInput{
				DueInterval: deref(st.DueInterval), ReminderDelay: deref(st.ReminderDelay),
				ReminderText: deref(st.ReminderText), ReminderAutoSend: deref(st.ReminderAutoSend),
			}
		}
		c.Steps = &steps
	}
	return c
}

// ListWorkflows implements ServerInterface.
func (s *server) ListWorkflows(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	list, err := s.deps.Invoicing.Workflows(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]Workflow, len(list))
	for i, wf := range list {
		out[i] = toWorkflow(wf)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateWorkflow implements ServerInterface.
func (s *server) CreateWorkflow(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[WorkflowRequest](w, r)
	if !ok {
		return
	}
	t := invoicing.WorkflowType("")
	if body.Workflow.Type != nil {
		t = invoicing.WorkflowType(*body.Workflow.Type)
	}
	wf, err := s.deps.Invoicing.CreateWorkflow(r.Context(), admin, user(r).ID, t, toWorkflowChanges(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toWorkflow(wf))
}

// GetWorkflow implements ServerInterface.
func (s *server) GetWorkflow(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	wf, err := s.deps.Invoicing.Workflow(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toWorkflow(wf))
}

// UpdateWorkflow implements ServerInterface.
func (s *server) UpdateWorkflow(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[WorkflowRequest](w, r)
	if !ok {
		return
	}
	wf, err := s.deps.Invoicing.UpdateWorkflow(r.Context(), admin, rid, user(r).ID, toWorkflowChanges(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toWorkflow(wf))
}

// DeleteWorkflow implements ServerInterface.
func (s *server) DeleteWorkflow(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Invoicing.DeleteWorkflow(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// documentStyleLine is a layout entry in Moneybird's JSON shape: options that
// are off are left out.
type documentStyleLine struct {
	Field string `json:"field"`
	Bold  bool   `json:"bold,omitempty"`
	Label bool   `json:"label,omitempty"`
}

// documentStyleJSON is the DocumentStyle response. The generated type models
// the layout lines as maps of unions, so this mirrors it with plain fields;
// the contract tests check that both agree.
type documentStyleJSON struct {
	ID                     string              `json:"id"`
	AdministrationID       string              `json:"administration_id"`
	Name                   string              `json:"name"`
	IdentityID             string              `json:"identity_id"`
	Default                bool                `json:"default"`
	LogoHash               *string             `json:"logo_hash"`
	LogoContainerFullWidth bool                `json:"logo_container_full_width"`
	LogoDisplayWidth       int                 `json:"logo_display_width"`
	LogoPosition           string              `json:"logo_position"`
	BackgroundHash         *string             `json:"background_hash"`
	PaperSize              string              `json:"paper_size"`
	AddressPosition        string              `json:"address_position"`
	FontSize               int                 `json:"font_size"`
	FontFamily             string              `json:"font_family"`
	PrintOnStationery      bool                `json:"print_on_stationery"`
	CustomCSS              *string             `json:"custom_css"`
	InvoiceSenderAddress   []documentStyleLine `json:"invoice_sender_address"`
	InvoiceMetadataLeft    []documentStyleLine `json:"invoice_metadata_left"`
	InvoiceMetadataRight   []documentStyleLine `json:"invoice_metadata_right"`
	EstimateSenderAddress  []documentStyleLine `json:"estimate_sender_address"`
	EstimateMetadataLeft   []documentStyleLine `json:"estimate_metadata_left"`
	EstimateMetadataRight  []documentStyleLine `json:"estimate_metadata_right"`
	CreatedAt              time.Time           `json:"created_at"`
	UpdatedAt              time.Time           `json:"updated_at"`
	URI                    string              `json:"uri"`
}

func lines(in []invoicing.Line) []documentStyleLine {
	out := make([]documentStyleLine, len(in))
	for i, l := range in {
		out[i] = documentStyleLine(l)
	}
	return out
}

func toDocumentStyle(d invoicing.DocumentStyle) documentStyleJSON {
	hash := func(f *invoicing.StoredFile) *string {
		if f == nil {
			return nil
		}
		return &f.Hash
	}
	return documentStyleJSON{
		ID: d.ID.String(), AdministrationID: d.AdministrationID.String(), Name: d.Name, IdentityID: d.IdentityID.String(),
		Default: d.Default, LogoHash: hash(d.Logo), LogoContainerFullWidth: d.LogoContainerFullWidth,
		LogoDisplayWidth: d.LogoDisplayWidth, LogoPosition: d.LogoPosition, BackgroundHash: hash(d.Background),
		PaperSize: d.PaperSize, AddressPosition: d.AddressPosition, FontSize: d.FontSize, FontFamily: d.FontFamily,
		PrintOnStationery: d.PrintOnStationery, CustomCSS: nullable(d.CustomCSS),
		InvoiceSenderAddress: lines(d.InvoiceSenderAddress), InvoiceMetadataLeft: lines(d.InvoiceMetadataLeft),
		InvoiceMetadataRight: lines(d.InvoiceMetadataRight), EstimateSenderAddress: lines(d.EstimateSenderAddress),
		EstimateMetadataLeft: lines(d.EstimateMetadataLeft), EstimateMetadataRight: lines(d.EstimateMetadataRight),
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, URI: d.Ref().URI(),
	}
}

func toLines(in *[]DocumentStyleLine) *[]invoicing.Line {
	if in == nil {
		return nil
	}
	out := make([]invoicing.Line, len(*in))
	for i, l := range *in {
		out[i] = invoicing.Line{Field: l.Field, Bold: deref(l.Bold), Label: deref(l.Label)}
	}
	return &out
}

func toDocumentStyleChanges(body DocumentStyleRequest) invoicing.DocumentStyleChanges {
	in := body.DocumentStyle
	c := invoicing.DocumentStyleChanges{
		Name: in.Name, Default: in.Default, LogoContainerFullWidth: in.LogoContainerFullWidth,
		LogoDisplayWidth: in.LogoDisplayWidth, LogoPosition: (*string)(in.LogoPosition),
		PrintOnStationery: in.PrintOnStationery, PaperSize: (*string)(in.PaperSize),
		AddressPosition: (*string)(in.AddressPosition), FontSize: in.FontSize, FontFamily: in.FontFamily, CustomCSS: in.CustomCss,
		InvoiceSenderAddress: toLines(in.InvoiceSenderAddress), InvoiceMetadataLeft: toLines(in.InvoiceMetadataLeft),
		InvoiceMetadataRight: toLines(in.InvoiceMetadataRight), EstimateSenderAddress: toLines(in.EstimateSenderAddress),
		EstimateMetadataLeft: toLines(in.EstimateMetadataLeft), EstimateMetadataRight: toLines(in.EstimateMetadataRight),
	}
	if in.IdentityId != nil {
		// The contract guarantees digits; an overflow becomes 0, which no
		// identity has, so validation reports it.
		id, _ := resource.ParseID(*in.IdentityId)
		c.IdentityID = &id
	}
	return c
}

// ListDocumentStyles implements ServerInterface.
func (s *server) ListDocumentStyles(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	list, err := s.deps.Invoicing.DocumentStyles(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]documentStyleJSON, len(list))
	for i, d := range list {
		out[i] = toDocumentStyle(d)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateDocumentStyle implements ServerInterface.
func (s *server) CreateDocumentStyle(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[DocumentStyleRequest](w, r)
	if !ok {
		return
	}
	d, err := s.deps.Invoicing.CreateDocumentStyle(r.Context(), admin, user(r).ID, toDocumentStyleChanges(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toDocumentStyle(d))
}

// GetDocumentStyle implements ServerInterface.
func (s *server) GetDocumentStyle(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	d, err := s.deps.Invoicing.DocumentStyle(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toDocumentStyle(d))
}

// UpdateDocumentStyle implements ServerInterface.
func (s *server) UpdateDocumentStyle(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[DocumentStyleRequest](w, r)
	if !ok {
		return
	}
	d, err := s.deps.Invoicing.UpdateDocumentStyle(r.Context(), admin, rid, user(r).ID, toDocumentStyleChanges(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toDocumentStyle(d))
}

// DeleteDocumentStyle implements ServerInterface.
func (s *server) DeleteDocumentStyle(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Invoicing.DeleteDocumentStyle(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetDocumentStyleFile implements ServerInterface.
func (s *server) GetDocumentStyleFile(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, file string) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, contentType, err := s.deps.Invoicing.OpenFile(r.Context(), admin, rid, user(r).ID, invoicing.FileKind(file))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer func() { _ = body.Close() }()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.WriteHeader(http.StatusOK)
	// The client went away if this fails; there is nobody left to tell.
	_, _ = io.Copy(w, body)
}

// UploadDocumentStyleFile implements ServerInterface.
func (s *server) UploadDocumentStyleFile(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, file string) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "The file is too large")
		return
	}
	d, err := s.deps.Invoicing.UploadFile(r.Context(), admin, rid, user(r).ID, invoicing.FileKind(file), data)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toDocumentStyle(d))
}

// RemoveDocumentStyleFile implements ServerInterface.
func (s *server) RemoveDocumentStyleFile(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, file string) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	d, err := s.deps.Invoicing.RemoveFile(r.Context(), admin, rid, user(r).ID, invoicing.FileKind(file))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toDocumentStyle(d))
}

// GetCustomField implements ServerInterface.
func (s *server) GetCustomField(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	f, err := s.deps.Administrations.CustomField(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toCustomField(f))
}

// UpdateCustomField implements ServerInterface.
func (s *server) UpdateCustomField(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[CustomFieldUpdateRequest](w, r)
	if !ok {
		return
	}
	f, err := s.deps.Administrations.UpdateCustomField(r.Context(), admin, rid, user(r).ID, administration.CustomFieldChanges{Name: body.CustomField.Name})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toCustomField(f))
}

// DeleteCustomField implements ServerInterface.
func (s *server) DeleteCustomField(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Administrations.DeleteCustomField(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ReorderCustomFields implements ServerInterface.
func (s *server) ReorderCustomFields(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[CustomFieldOrderRequest](w, r)
	if !ok {
		return
	}
	in := body.CustomFieldsOrder
	ids := make([]resource.ID, len(in.Ids))
	for i, raw := range in.Ids {
		ids[i], _ = resource.ParseID(raw) // 0 matches no field, which validation reports.
	}
	if err := s.deps.Administrations.ReorderCustomFields(r.Context(), admin, user(r).ID, administration.Source(in.Source), ids); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func ptr[T any](v T) *T { return &v }
