package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/taxonomy"
)

func (s *server) bookkeepingSettings(w http.ResponseWriter, r *http.Request, admin resource.ID, st bookkeeping.Settings) {
	boxes, err := s.deps.Bookkeeping.Boxes(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := BookkeepingSettings{
		VatDocumentsPeriod: DeclarationPeriod(st.VATPeriod), IntraCommunityTransactionsDeclarationPeriod: DeclarationPeriod(st.ICPPeriod),
		ChartScheme: st.Scheme, ChartSchemes: []ChartScheme{}, TaxReportBoxes: make([]TaxReportBox, len(boxes)),
	}
	if st.LockedUntil != nil {
		if day, err := time.Parse(time.DateOnly, *st.LockedUntil); err == nil {
			out.PeriodLockedUntil = &openapi_types.Date{Time: day}
		}
	}
	for _, sc := range taxonomy.All() {
		out.ChartSchemes = append(out.ChartSchemes, ChartScheme{Id: sc.ID, Country: sc.Country, Name: sc.Name, Version: sc.Version, Source: sc.Source})
	}
	for i, b := range boxes {
		out.TaxReportBoxes[i] = TaxReportBox{Code: b.Code, Name: b.Name}
	}
	writeJSON(w, http.StatusOK, out)
}

// GetBookkeepingSettings implements ServerInterface.
func (s *server) GetBookkeepingSettings(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	st, err := s.deps.Bookkeeping.Settings(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.bookkeepingSettings(w, r, admin, st)
}

// UpdateBookkeepingSettings implements ServerInterface.
func (s *server) UpdateBookkeepingSettings(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[BookkeepingSettingsRequest](w, r)
	if !ok {
		return
	}
	in := body.BookkeepingSettings
	st, err := s.deps.Bookkeeping.UpdateSettings(r.Context(), admin, user(r).ID, bookkeeping.SettingsChanges{
		VATPeriod: (*string)(in.VatDocumentsPeriod), ICPPeriod: (*string)(in.IntraCommunityTransactionsDeclarationPeriod),
		Scheme: in.ChartScheme, LockedUntil: in.PeriodLockedUntil,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.bookkeepingSettings(w, r, admin, st)
}

// taxRateJSON is the TaxRate response. The generated type models
// percentage as a union of string and number; Moneybird sends a string.
type taxRateJSON struct {
	ID               string    `json:"id"`
	AdministrationID string    `json:"administration_id"`
	Name             string    `json:"name"`
	Percentage       string    `json:"percentage"`
	TaxRateType      string    `json:"tax_rate_type"`
	ShowTax          bool      `json:"show_tax"`
	Active           bool      `json:"active"`
	Country          *string   `json:"country"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	URI              string    `json:"uri"`
	TaxReportBoxes   []string  `json:"tax_report_boxes"`
	Position         int       `json:"position"`
}

func toTaxRate(t bookkeeping.TaxRate) taxRateJSON {
	boxes := t.ReportBoxes
	if boxes == nil {
		boxes = []string{}
	}
	return taxRateJSON{
		ID: t.ID.String(), AdministrationID: t.AdministrationID.String(), Name: t.Name, Percentage: t.Percentage,
		TaxRateType: t.Type, ShowTax: t.ShowTax, Active: t.Active, Country: emptyNil(t.Country),
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, URI: t.Ref().URI(), TaxReportBoxes: boxes, Position: t.Position,
	}
}

func emptyNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// parseTaxRateFilter reads Moneybird's filter syntax: key:value pairs
// separated by commas.
func parseTaxRateFilter(raw string) (bookkeeping.TaxRateFilter, error) {
	var f bookkeeping.TaxRateFilter
	if raw == "" {
		return f, nil
	}
	parseBool := func(v string) (*bool, error) {
		b, err := strconv.ParseBool(v)
		return &b, err
	}
	parseTime := func(v string) (*time.Time, error) {
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05 MST", time.DateTime, time.DateOnly} {
			if t, err := time.Parse(layout, v); err == nil {
				return &t, nil
			}
		}
		return nil, fmt.Errorf("%q is not a time", v)
	}
	for part := range strings.SplitSeq(raw, ",") {
		key, value, ok := strings.Cut(part, ":")
		if !ok {
			return f, fmt.Errorf("filter %q is not key:value", part)
		}
		var err error
		switch key {
		case "name":
			f.Name = &value
		case "partial_name":
			f.PartialName = &value
		case "percentage":
			f.Percentage = &value
		case "tax_rate_type":
			f.Type = &value
		case "country":
			f.Country = &value
		case "show_tax":
			f.ShowTax, err = parseBool(value)
		case "active":
			f.Active, err = parseBool(value)
		case "created_after":
			f.CreatedAfter, err = parseTime(value)
		case "updated_after":
			f.UpdatedAfter, err = parseTime(value)
		default:
			return f, fmt.Errorf("unknown filter %q", key)
		}
		if err != nil {
			return f, fmt.Errorf("filter %s: %w", key, err)
		}
	}
	return f, nil
}

// ListTaxRates implements ServerInterface.
func (s *server) ListTaxRates(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListTaxRatesParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	f, err := parseTaxRateFilter(deref(params.Filter))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := s.deps.Bookkeeping.TaxRates(r.Context(), admin, user(r).ID, f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	page, perPage := pageNumbers(params.Page, params.PerPage)
	page, perPage = max(page, 1), cmpPerPage(perPage, bookkeeping.TaxRatesPerPage)
	out := []taxRateJSON{}
	for _, t := range list[min((page-1)*perPage, len(list)):min(page*perPage, len(list))] {
		out = append(out, toTaxRate(t))
	}
	s.writeLinks(w, r, page, perPage, len(list))
	writeJSON(w, http.StatusOK, out)
}

func toTaxRateChanges(body TaxRateRequest) bookkeeping.TaxRateChanges {
	in := body.TaxRate
	return bookkeeping.TaxRateChanges{
		Name: in.Name, Percentage: in.Percentage, Type: (*string)(in.TaxRateType), ShowTax: in.ShowTax, Active: in.Active,
		Country: in.Country, ReportBoxes: in.TaxReportBoxes,
	}
}

// CreateTaxRate implements ServerInterface.
func (s *server) CreateTaxRate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[TaxRateRequest](w, r)
	if !ok {
		return
	}
	t, err := s.deps.Bookkeeping.CreateTaxRate(r.Context(), admin, user(r).ID, toTaxRateChanges(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toTaxRate(t))
}

// GetTaxRate implements ServerInterface.
func (s *server) GetTaxRate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	t, err := s.deps.Bookkeeping.TaxRate(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toTaxRate(t))
}

// UpdateTaxRate implements ServerInterface.
func (s *server) UpdateTaxRate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[TaxRateRequest](w, r)
	if !ok {
		return
	}
	t, err := s.deps.Bookkeeping.UpdateTaxRate(r.Context(), admin, rid, user(r).ID, toTaxRateChanges(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toTaxRate(t))
}

// ReorderTaxRates implements ServerInterface.
func (s *server) ReorderTaxRates(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[TaxRateOrderRequest](w, r)
	if !ok {
		return
	}
	ids := make([]resource.ID, len(body.Ids))
	for i, raw := range body.Ids {
		if ids[i], ok = parseID(w, raw); !ok {
			return
		}
	}
	if err := s.deps.Bookkeeping.ReorderTaxRates(r.Context(), admin, user(r).ID, string(body.TaxRateType), ids); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type taxonomyItemJSON struct {
	TaxonomyVersion string  `json:"taxonomy_version"`
	Code            string  `json:"code"`
	Name            string  `json:"name"`
	NameEnglish     *string `json:"name_english"`
	Reference       *string `json:"reference"`
	Description     *string `json:"description,omitempty"`
	Level           int     `json:"level,omitempty"`
	Side            string  `json:"side,omitempty"`
}

func toTaxonomyItem(scheme *taxonomy.Scheme, it taxonomy.Item, full bool) *taxonomyItemJSON {
	out := &taxonomyItemJSON{TaxonomyVersion: scheme.Version, Code: it.Code, Name: it.Name, Reference: emptyNil(it.Reference)}
	if full {
		out.Description, out.Level, out.Side = emptyNil(it.Description), it.Level, "profit_and_loss"
		if it.Side() == taxonomy.BalanceSheet {
			out.Side = "balance_sheet"
		}
	}
	return out
}

// SearchTaxonomyItems implements ServerInterface.
func (s *server) SearchTaxonomyItems(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params SearchTaxonomyItemsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	scheme, err := s.deps.Bookkeeping.Scheme(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []*taxonomyItemJSON{}
	if scheme != nil {
		side := taxonomy.AnySide
		if params.AccountType != nil {
			side = bookkeeping.SideOf(string(*params.AccountType))
		}
		limit := 25
		if params.Limit != nil {
			limit = *params.Limit
		}
		for _, it := range scheme.Search(deref(params.Query), side, limit) {
			out = append(out, toTaxonomyItem(scheme, it, true))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// ledgerAccountJSON is the LedgerAccount response. The generated type models
// taxonomy_item and parent_id as unions.
type ledgerAccountJSON struct {
	ID                   string            `json:"id"`
	AdministrationID     string            `json:"administration_id"`
	Name                 string            `json:"name"`
	AccountType          string            `json:"account_type"`
	AccountID            *string           `json:"account_id"`
	ParentID             *string           `json:"parent_id"`
	Active               bool              `json:"active"`
	AllowedDocumentTypes []string          `json:"allowed_document_types"`
	FinancialAccountID   *string           `json:"financial_account_id"`
	TaxonomyItem         *taxonomyItemJSON `json:"taxonomy_item"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
	URI                  string            `json:"uri"`
	Description          *string           `json:"description"`
	SystemRole           *string           `json:"system_role"`
	Template             *string           `json:"template"`
}

func toLedgerAccount(scheme *taxonomy.Scheme, l bookkeeping.LedgerAccount) ledgerAccountJSON {
	out := ledgerAccountJSON{
		ID: l.ID.String(), AdministrationID: l.AdministrationID.String(), Name: l.Name, AccountType: l.Type,
		AccountID: emptyNil(l.Code), Active: l.Active, AllowedDocumentTypes: l.AllowedDocumentTypes,
		CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt, URI: l.Ref().URI(), Description: emptyNil(l.Description),
		SystemRole: emptyNil(l.System), Template: emptyNil(l.Template),
	}
	if out.AllowedDocumentTypes == nil {
		out.AllowedDocumentTypes = []string{}
	}
	if l.ParentID != nil {
		out.ParentID = ptr(l.ParentID.String())
	}
	if scheme != nil && l.Taxonomy != "" {
		if it, ok := scheme.Item(l.Taxonomy); ok {
			out.TaxonomyItem = toTaxonomyItem(scheme, it, false)
		}
	}
	return out
}

func (s *server) ledgerAccounts(w http.ResponseWriter, r *http.Request, admin resource.ID, status int, list []bookkeeping.LedgerAccount) {
	scheme, err := s.deps.Bookkeeping.Scheme(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]ledgerAccountJSON, len(list))
	for i, l := range list {
		out[i] = toLedgerAccount(scheme, l)
	}
	writeJSON(w, status, out)
}

func (s *server) ledgerAccount(w http.ResponseWriter, r *http.Request, admin resource.ID, status int, l bookkeeping.LedgerAccount) {
	scheme, err := s.deps.Bookkeeping.Scheme(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, toLedgerAccount(scheme, l))
}

// ListLedgerAccounts implements ServerInterface.
func (s *server) ListLedgerAccounts(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	list, err := s.deps.Bookkeeping.LedgerAccounts(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.ledgerAccounts(w, r, admin, http.StatusOK, list)
}

func toLedgerAccountChanges(w http.ResponseWriter, body LedgerAccountRequest) (bookkeeping.LedgerAccountChanges, bool) {
	in := body.LedgerAccount
	c := bookkeeping.LedgerAccountChanges{
		Name: in.Name, Type: (*string)(in.AccountType), Code: in.AccountId, Active: in.Active,
		Description: in.Description, Taxonomy: body.RgsCode,
	}
	if in.AllowedDocumentTypes != nil {
		types := make([]string, len(*in.AllowedDocumentTypes))
		for i, t := range *in.AllowedDocumentTypes {
			types[i] = string(t)
		}
		c.AllowedDocumentTypes = &types
	}
	if in.ParentId != nil {
		parent := resource.ID(0)
		if *in.ParentId != "" {
			var ok bool
			if parent, ok = parseID(w, *in.ParentId); !ok {
				return c, false
			}
		}
		c.ParentID = &parent
	}
	return c, true
}

// CreateLedgerAccount implements ServerInterface.
func (s *server) CreateLedgerAccount(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[LedgerAccountRequest](w, r)
	if !ok {
		return
	}
	c, ok := toLedgerAccountChanges(w, body)
	if !ok {
		return
	}
	l, err := s.deps.Bookkeeping.CreateLedgerAccount(r.Context(), admin, user(r).ID, c)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.ledgerAccount(w, r, admin, http.StatusCreated, l)
}

// GetLedgerAccount implements ServerInterface.
func (s *server) GetLedgerAccount(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	l, err := s.deps.Bookkeeping.LedgerAccount(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.ledgerAccount(w, r, admin, http.StatusOK, l)
}

// UpdateLedgerAccount implements ServerInterface.
func (s *server) UpdateLedgerAccount(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[LedgerAccountRequest](w, r)
	if !ok {
		return
	}
	c, ok := toLedgerAccountChanges(w, body)
	if !ok {
		return
	}
	l, err := s.deps.Bookkeeping.UpdateLedgerAccount(r.Context(), admin, rid, user(r).ID, c)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.ledgerAccount(w, r, admin, http.StatusOK, l)
}

// DeleteLedgerAccount implements ServerInterface.
func (s *server) DeleteLedgerAccount(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	l, kept, err := s.deps.Bookkeeping.DeleteLedgerAccount(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !kept {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.ledgerAccount(w, r, admin, http.StatusOK, l)
}

// UpdateLedgerAccountScheme implements ServerInterface.
func (s *server) UpdateLedgerAccountScheme(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[LedgerAccountSchemeRequest](w, r)
	if !ok {
		return
	}
	codes := make(map[resource.ID]string, len(body.LedgerAccounts))
	for _, in := range body.LedgerAccounts {
		id, ok := parseID(w, in.Id)
		if !ok {
			return
		}
		codes[id] = in.RgsCode
	}
	list, err := s.deps.Bookkeeping.UpdateScheme(r.Context(), admin, user(r).ID, codes)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.ledgerAccounts(w, r, admin, http.StatusOK, list)
}

// ListLedgerAccountTemplates implements ServerInterface.
func (s *server) ListLedgerAccountTemplates(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	list, err := s.deps.Bookkeeping.Templates(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]LedgerAccountTemplate, len(list))
	for i, t := range list {
		out[i] = toLedgerAccountTemplate(t)
	}
	writeJSON(w, http.StatusOK, out)
}

func toLedgerAccountTemplate(t bookkeeping.Template) LedgerAccountTemplate {
	options := func(in []bookkeeping.TemplateOption) []LedgerAccountTemplateOption {
		out := make([]LedgerAccountTemplateOption, len(in))
		for i, o := range in {
			out[i] = LedgerAccountTemplateOption{Key: o.Key, Name: o.Name, RgsCode: o.Taxonomy}
		}
		return out
	}
	out := LedgerAccountTemplate{
		Key: t.Key, Name: t.Name, Description: t.Description, Intro: t.Intro, Warning: t.Warning, NameLabel: t.NameLabel,
		Applied: t.Applied, LedgerAccounts: make([]LedgerAccountTemplateAccount, len(t.Accounts)),
	}
	if t.Choice != nil {
		out.Choice = &LedgerAccountTemplateChoice{Label: t.Choice.Label, Prompt: t.Choice.Prompt, Options: options(t.Choice.Options)}
	}
	if t.Single != nil {
		out.Single = &LedgerAccountTemplateSingle{
			AccountType: AccountType(t.Single.Type), NameLabel: t.Single.NameLabel, OptionsLabel: t.Single.OptionsLabel,
			OptionsHelp: t.Single.OptionsHelp, Parent: t.Single.Parent, Options: options(t.Single.Options),
		}
	}
	for i, a := range t.Accounts {
		choices := a.Choices
		if choices == nil {
			choices = []string{}
		}
		out.LedgerAccounts[i] = LedgerAccountTemplateAccount{
			Key: a.Key, Parent: emptyNil(a.Parent), Name: a.Name, AccountType: AccountType(a.Type), AccountId: a.Code, RgsCode: a.Taxonomy,
			Description: a.Description, Exists: a.Exists, Selected: a.Selected, Choices: choices,
		}
	}
	return out
}

// ApplyLedgerAccountTemplate implements ServerInterface. The body is
// optional: without it, all of the theme's missing ledger accounts are added.
func (s *server) ApplyLedgerAccountTemplate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, key string) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	var body LedgerAccountTemplateRequest
	raw, err := io.ReadAll(r.Body)
	if err != nil || len(bytes.TrimSpace(raw)) > 0 && json.Unmarshal(raw, &body) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in := bookkeeping.TemplateInput{}
	if body.LedgerAccounts != nil {
		in.Accounts = *body.LedgerAccounts
	}
	if body.Name != nil {
		in.Name = *body.Name
	}
	if body.Option != nil {
		in.Option = *body.Option
	}
	if body.ParentId != nil && *body.ParentId != "" {
		parent, ok := parseID(w, *body.ParentId)
		if !ok {
			return
		}
		in.ParentID = &parent
	}
	added, err := s.deps.Bookkeeping.ApplyTemplate(r.Context(), admin, user(r).ID, key, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.ledgerAccounts(w, r, admin, http.StatusCreated, added)
}

// GetDetailUsage implements ServerInterface.
func (s *server) GetDetailUsage(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetDetailUsageParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	u, err := s.deps.Bookkeeping.DetailUsage(r.Context(), admin, user(r).ID, string(params.Kind))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	counts := func(in []bookkeeping.UsageCount) []UsageCount {
		out := make([]UsageCount, len(in))
		for i, c := range in {
			out[i] = UsageCount{Id: c.ID.String(), Count: c.Count}
		}
		return out
	}
	writeJSON(w, http.StatusOK, DetailUsage{TaxRates: counts(u.TaxRates), LedgerAccounts: counts(u.LedgerAccounts)})
}
