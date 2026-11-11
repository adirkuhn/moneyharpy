package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/projects"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

type productJSON struct {
	ID               string    `json:"id"`
	AdministrationID string    `json:"administration_id"`
	Description      *string   `json:"description"`
	Title            *string   `json:"title"`
	Identifier       *string   `json:"identifier"`
	Price            string    `json:"price"`
	Currency         string    `json:"currency"`
	Frequency        *int      `json:"frequency"`
	FrequencyType    *string   `json:"frequency_type"`
	TaxRateID        *string   `json:"tax_rate_id"`
	LedgerAccountID  string    `json:"ledger_account_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Active           bool      `json:"active"`
	URI              string    `json:"uri"`
}

func toProduct(p products.Product) productJSON {
	out := productJSON{
		ID: p.ID.String(), AdministrationID: p.AdministrationID.String(), Description: nz(p.Description), Title: nz(p.Title),
		Identifier: nz(p.Identifier), Price: p.Price.String(), Currency: p.Currency, FrequencyType: nz(p.FrequencyType),
		TaxRateID: idString(p.TaxRateID), LedgerAccountID: p.LedgerAccountID.String(), CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		Active: p.Active, URI: p.Ref().URI(),
	}
	if p.FrequencyType != "" {
		n := p.Frequency
		out.Frequency = &n
	}
	return out
}

func (s *server) writeProduct(w http.ResponseWriter, r *http.Request, status int, p products.Product, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, toProduct(p))
}

type productBody struct {
	Product struct {
		Title               *string     `json:"title"`
		Description         *string     `json:"description"`
		CheckoutDescription *string     `json:"checkout_description"`
		Price               *flexString `json:"price"`
		DocumentStyleID     *flexString `json:"document_style_id"`
		LedgerAccountID     *flexString `json:"ledger_account_id"`
		TaxRateID           *flexString `json:"tax_rate_id"`
		WorkflowID          *flexString `json:"workflow_id"`
		Currency            *string     `json:"currency"`
		CheckoutType        *string     `json:"checkout_type"`
		FrequencyType       *string     `json:"frequency_type"`
		Frequency           *flexString `json:"frequency"`
		ProductType         *string     `json:"product_type"`
		VatRateType         *string     `json:"vat_rate_type"`
		MaxAmountPerOrder   *flexString `json:"max_amount_per_order"`
		Identifier          *string     `json:"identifier"`
		FrequencyPreset     *string     `json:"frequency_preset"`
		Active              *flexBool   `json:"active"`
	} `json:"product"`
}

func readProductChanges(w http.ResponseWriter, r *http.Request) (products.Changes, bool) {
	body, ok := decodeBody[productBody](w, r)
	if !ok {
		return products.Changes{}, false
	}
	in := body.Product
	c := products.Changes{
		Title: in.Title, Description: in.Description, CheckoutDescription: in.CheckoutDescription, Identifier: in.Identifier, Price: in.Price.ptr(),
		Currency: in.Currency, FrequencyType: in.FrequencyType, FrequencyPreset: in.FrequencyPreset, ProductType: in.ProductType,
		VatRateType: in.VatRateType, CheckoutType: in.CheckoutType, Active: in.Active.ptr(),
	}
	for _, f := range []struct {
		src *flexString
		dst **resource.ID
	}{{in.LedgerAccountID, &c.LedgerAccountID}, {in.TaxRateID, &c.TaxRateID}, {in.DocumentStyleID, &c.DocumentStyleID}, {in.WorkflowID, &c.WorkflowID}} {
		if *f.dst, ok = flexOptionalID(w, f.src); !ok {
			return c, false
		}
	}
	if c.Frequency, ok = flexInt(w, in.Frequency, "frequency"); !ok {
		return c, false
	}
	if c.MaxAmountPerOrder, ok = flexInt(w, in.MaxAmountPerOrder, "max_amount_per_order"); !ok {
		return c, false
	}
	return c, true
}

// ListProducts implements ServerInterface.
func (s *server) ListProducts(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListProductsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q := products.Query{
		Search: deref(params.Query), Currency: deref(params.Currency), IncludeInactive: deref(params.Active) == ListProductsParamsActiveFalse,
	}
	if params.ForCheckout != nil {
		checkout := *params.ForCheckout == ListProductsParamsForCheckoutTrue
		q.ForCheckout = &checkout
	}
	if params.LedgerAccountId != nil {
		id, ok := parseID(w, *params.LedgerAccountId)
		if !ok {
			return
		}
		q.LedgerAccountID = &id
	}
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	list, total, err := s.deps.Products.List(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]productJSON, len(list))
	for i, p := range list {
		out[i] = toProduct(p)
	}
	s.writeLinks(w, r, max(q.Page, 1), pageSize(q.PerPage, products.DefaultPerPage, products.MaxPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

// pageSize is the page size a list used: the asked one within bounds, or
// the default.
func pageSize(asked, fallback, most int) int {
	if asked <= 0 {
		return fallback
	}
	return min(asked, most)
}

// CreateProduct implements ServerInterface.
func (s *server) CreateProduct(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	c, ok := readProductChanges(w, r)
	if !ok {
		return
	}
	p, err := s.deps.Products.Create(r.Context(), admin, user(r).ID, c)
	s.writeProduct(w, r, http.StatusCreated, p, err)
}

// GetProductByIdentifier implements ServerInterface.
func (s *server) GetProductByIdentifier(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, identifier string) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	p, err := s.deps.Products.ByIdentifier(r.Context(), admin, user(r).ID, identifier)
	s.writeProduct(w, r, http.StatusOK, p, err)
}

// GetProduct implements ServerInterface.
func (s *server) GetProduct(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	p, err := s.deps.Products.Product(r.Context(), admin, rid, user(r).ID)
	s.writeProduct(w, r, http.StatusOK, p, err)
}

// UpdateProduct implements ServerInterface.
func (s *server) UpdateProduct(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	c, ok := readProductChanges(w, r)
	if !ok {
		return
	}
	p, err := s.deps.Products.Update(r.Context(), admin, rid, user(r).ID, c)
	s.writeProduct(w, r, http.StatusOK, p, err)
}

// DeleteProduct implements ServerInterface: a product lines name is kept,
// deactivated, as the X-Kept-As header tells.
func (s *server) DeleteProduct(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	kept, err := s.deps.Products.Delete(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if kept {
		w.Header().Set("X-Kept-As", "deactivated")
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateProductSalesLink implements ServerInterface. MoneyHarpy has no
// online checkout, so it answers as Moneybird does for a product that is
// not valid for online sales.
func (s *server) CreateProductSalesLink(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, params CreateProductSalesLinkParams) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	var contact *resource.ID
	if params.ContactId != nil {
		cid, ok := parseID(w, *params.ContactId)
		if !ok {
			return
		}
		contact = &cid
	}
	link, err := s.deps.Products.SalesLink(r.Context(), admin, rid, user(r).ID, contact)
	if errors.Is(err, products.ErrNotForOnlineSales) {
		writeError(w, http.StatusNotFound, "Product not valid for online sales")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

type projectJSON struct {
	ID               string    `json:"id"`
	AdministrationID string    `json:"administration_id"`
	Name             string    `json:"name"`
	State            string    `json:"state"`
	Budget           *int      `json:"budget"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	URI              string    `json:"uri"`
}

func toProject(p projects.Project) projectJSON {
	return projectJSON{
		ID: p.ID.String(), AdministrationID: p.AdministrationID.String(), Name: p.Name, State: p.State, Budget: p.Budget,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, URI: p.Ref().URI(),
	}
}

func (s *server) writeProject(w http.ResponseWriter, r *http.Request, status int, p projects.Project, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, toProject(p))
}

// presence keeps a field's raw value and whether it was sent at all, so
// null can mean "remove".
type presence struct {
	sent bool
	raw  json.RawMessage
}

// UnmarshalJSON implements json.Unmarshaler; it is called for null too.
func (p *presence) UnmarshalJSON(b []byte) error {
	p.sent, p.raw = true, append(json.RawMessage(nil), b...)
	return nil
}

func readProjectChanges(w http.ResponseWriter, r *http.Request) (projects.Changes, bool) {
	body, ok := decodeBody[struct {
		Project struct {
			Name   *string  `json:"name"`
			Budget presence `json:"budget"`
			State  *string  `json:"state"`
		} `json:"project"`
	}](w, r)
	if !ok {
		return projects.Changes{}, false
	}
	in := body.Project
	c := projects.Changes{Name: in.Name, State: in.State}
	if in.Budget.sent {
		budget := ""
		if raw := bytes.TrimSpace(in.Budget.raw); !bytes.Equal(raw, []byte("null")) {
			var f flexString
			if err := json.Unmarshal(raw, &f); err != nil {
				writeError(w, http.StatusBadRequest, "budget is not a number")
				return c, false
			}
			budget = string(f)
		}
		c.Budget = &budget
	}
	return c, true
}

// ListProjects implements ServerInterface.
func (s *server) ListProjects(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListProjectsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, err := projects.ParseFilter(deref((*string)(params.Filter)))
	if err != nil {
		writeError(w, http.StatusBadRequest, filterProblem(err))
		return
	}
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	list, total, err := s.deps.Projects.List(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]projectJSON, len(list))
	for i, p := range list {
		out[i] = toProject(p)
	}
	s.writeLinks(w, r, max(q.Page, 1), pageSize(q.PerPage, projects.DefaultPerPage, projects.MaxPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

// CreateProject implements ServerInterface.
func (s *server) CreateProject(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	c, ok := readProjectChanges(w, r)
	if !ok {
		return
	}
	p, err := s.deps.Projects.Create(r.Context(), admin, user(r).ID, c)
	s.writeProject(w, r, http.StatusCreated, p, err)
}

// GetProject implements ServerInterface.
func (s *server) GetProject(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	p, err := s.deps.Projects.Project(r.Context(), admin, rid, user(r).ID)
	s.writeProject(w, r, http.StatusOK, p, err)
}

// UpdateProject implements ServerInterface.
func (s *server) UpdateProject(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	c, ok := readProjectChanges(w, r)
	if !ok {
		return
	}
	p, err := s.deps.Projects.Update(r.Context(), admin, rid, user(r).ID, c)
	s.writeProject(w, r, http.StatusOK, p, err)
}

// DeleteProject implements ServerInterface: a project something is booked
// on is kept, archived, as the X-Kept-As header tells.
func (s *server) DeleteProject(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	kept, err := s.deps.Projects.Delete(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if kept {
		w.Header().Set("X-Kept-As", "archived")
	}
	w.WriteHeader(http.StatusNoContent)
}
