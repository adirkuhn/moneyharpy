package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/downloads"
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/subscriptions"
)

type subscriptionProductJSON struct {
	StartDate string       `json:"start_date"`
	EndDate   *string      `json:"end_date"`
	ProductID string       `json:"product_id"`
	Product   *productJSON `json:"product,omitempty"`
	Amount    string       `json:"amount"`
	Discount  string       `json:"discount"`
}

type subscriptionJSON struct {
	ID                   string                    `json:"id"`
	AdministrationID     string                    `json:"administration_id"`
	StartDate            string                    `json:"start_date"`
	EndDate              *string                   `json:"end_date"`
	Frequency            int                       `json:"frequency"`
	FrequencyType        string                    `json:"frequency_type"`
	Reference            *string                   `json:"reference"`
	CancelledAt          *time.Time                `json:"cancelled_at"`
	ProductID            string                    `json:"product_id"`
	Product              *productJSON              `json:"product,omitempty"`
	ContactID            string                    `json:"contact_id"`
	Contact              *contactJSON              `json:"contact,omitempty"`
	ContactPersonID      *string                   `json:"contact_person_id"`
	ContactPerson        *contactPersonJSON        `json:"contact_person"`
	SubscriptionProducts []subscriptionProductJSON `json:"subscription_products"`
	RecurringID          *string                   `json:"recurring_sales_invoice_id"`
	URI                  string                    `json:"uri"`
}

// toSubscriptions answers with subscriptions, their contacts and products.
func (s *server) toSubscriptions(r *http.Request, admin resource.ID, list []subscriptions.Subscription) ([]subscriptionJSON, error) {
	ctx, u := r.Context(), user(r).ID
	contactsByID := map[resource.ID]*contacts.Contact{}
	productsByID := map[resource.ID]*productJSON{}
	product := func(id resource.ID) (*productJSON, error) {
		if p, ok := productsByID[id]; ok {
			return p, nil
		}
		got, err := s.deps.Products.Product(ctx, admin, id, u)
		if errors.Is(err, products.ErrNotFound) {
			productsByID[id] = nil
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		out := toProduct(got)
		productsByID[id] = &out
		return &out, nil
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	out := make([]subscriptionJSON, len(list))
	for i, sub := range list {
		c, ok := contactsByID[sub.ContactID]
		if !ok {
			got, err := s.deps.Contacts.Contact(ctx, admin, sub.ContactID, u)
			if err != nil && !contacts.IsNotFound(err) {
				return nil, err
			}
			if err == nil {
				c = &got
			}
			contactsByID[sub.ContactID] = c
		}
		current := sub.ItemAt(today)
		o := subscriptionJSON{
			ID: sub.ID.String(), AdministrationID: admin.String(), StartDate: sub.StartDate.Format(time.DateOnly), EndDate: dateText(sub.EndDate),
			Frequency: sub.Frequency, FrequencyType: sub.FrequencyType, Reference: nz(sub.Reference), CancelledAt: sub.CancelledAt,
			ProductID: current.ProductID.String(), ContactID: sub.ContactID.String(), ContactPersonID: idString(sub.ContactPersonID),
			SubscriptionProducts: []subscriptionProductJSON{}, RecurringID: idString(sub.RecurringID), URI: sub.Ref().URI(),
		}
		if c != nil {
			cj := s.toContact(*c)
			o.Contact = &cj
			for _, p := range c.People {
				if sub.ContactPersonID != nil && p.ID == *sub.ContactPersonID {
					cp := toContactPerson(p)
					o.ContactPerson = &cp
				}
			}
		}
		var err error
		if o.Product, err = product(current.ProductID); err != nil {
			return nil, err
		}
		for _, it := range sub.Items {
			p, err := product(it.ProductID)
			if err != nil {
				return nil, err
			}
			o.SubscriptionProducts = append(o.SubscriptionProducts, subscriptionProductJSON{
				StartDate: it.StartDate.Format(time.DateOnly), EndDate: dateText(it.EndDate), ProductID: it.ProductID.String(), Product: p,
				Amount: it.Amount, Discount: percentText(it.Discount),
			})
		}
		out[i] = o
	}
	return out, nil
}

func (s *server) writeSubscription(w http.ResponseWriter, r *http.Request, admin resource.ID, status int, sub subscriptions.Subscription, err error) {
	if err != nil {
		s.failSubscription(w, r, err)
		return
	}
	out, err := s.toSubscriptions(r, admin, []subscriptions.Subscription{sub})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, out[0])
}

// symbolic writes Moneybird's error with its symbolic codes.
func symbolic(w http.ResponseWriter, status int, message string, codes map[string]any) {
	body := map[string]any{"error": message}
	if codes != nil {
		body["symbolic"] = codes
	}
	writeJSON(w, status, body)
}

// failSubscription answers the errors of subscriptions as Moneybird does.
func (s *server) failSubscription(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, subscriptions.ErrNotFound):
		symbolic(w, http.StatusNotFound, "Record not found for model name: Subscription", nil)
	case errors.Is(err, subscriptions.ErrCancelled):
		symbolic(w, http.StatusBadRequest, "Subscription is already cancelled", map[string]any{"subscription": "cancelled"})
	case errors.Is(err, subscriptions.ErrProductNotFound):
		symbolic(w, http.StatusNotFound, "Product not found", map[string]any{"product_id": "not_found"})
	case contacts.IsNotFound(err):
		symbolic(w, http.StatusNotFound, "Record not found for model name: Contact", nil)
	default:
		s.fail(w, r, err)
	}
}

type subscriptionBody struct {
	Subscription struct {
		StartDate        *string     `json:"start_date"`
		ProductID        *flexString `json:"product_id"`
		Amount           *string     `json:"amount"`
		Discount         *flexString `json:"discount"`
		ContactID        *flexString `json:"contact_id"`
		ContactPersonID  *flexString `json:"contact_person_id"`
		EndDate          *string     `json:"end_date"`
		Reference        *string     `json:"reference"`
		DocumentStyleID  *flexString `json:"document_style_id"`
		Frequency        *int        `json:"frequency"`
		FrequencyType    *string     `json:"frequency_type"`
		Mergeable        *flexBool   `json:"mergeable"`
		PricesAreInclTax *flexBool   `json:"prices_are_incl_tax"`
	} `json:"subscription"`
}

// nonZero drops a zero id: an id that is cleared on update is no id on
// create.
func nonZero(id *resource.ID) *resource.ID {
	if id == nil || *id == 0 {
		return nil
	}
	return id
}

// ListSubscriptions implements ServerInterface.
func (s *server) ListSubscriptions(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListSubscriptionsParams) {
	admin, contact, ok := parseIDs(w, administrationID, params.ContactId)
	if !ok {
		return
	}
	list, err := s.deps.Subscriptions.List(r.Context(), admin, user(r).ID, contact)
	if err != nil {
		s.failSubscription(w, r, err)
		return
	}
	out, err := s.toSubscriptions(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateSubscription implements ServerInterface.
func (s *server) CreateSubscription(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[subscriptionBody](w, r)
	if !ok {
		return
	}
	in := body.Subscription
	c := subscriptions.Input{
		StartDate: in.StartDate, EndDate: in.EndDate, Amount: in.Amount, Discount: in.Discount.ptr(), Reference: in.Reference,
		Frequency: in.Frequency, FrequencyType: in.FrequencyType, Mergeable: in.Mergeable.ptr(), PricesAreInclTax: in.PricesAreInclTax.ptr(),
	}
	for _, f := range []struct {
		src *flexString
		dst **resource.ID
	}{{in.ProductID, &c.ProductID}, {in.ContactID, &c.ContactID}, {in.ContactPersonID, &c.ContactPersonID}, {in.DocumentStyleID, &c.DocumentStyleID}} {
		id, ok := flexOptionalID(w, f.src)
		if !ok {
			return
		}
		*f.dst = nonZero(id)
	}
	sub, err := s.deps.Subscriptions.Create(r.Context(), admin, user(r).ID, c)
	s.writeSubscription(w, r, admin, http.StatusCreated, sub, err)
}

// GetSubscription implements ServerInterface.
func (s *server) GetSubscription(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, sid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	sub, err := s.deps.Subscriptions.Subscription(r.Context(), admin, sid, user(r).ID)
	s.writeSubscription(w, r, admin, http.StatusOK, sub, err)
}

// UpdateSubscription implements ServerInterface.
func (s *server) UpdateSubscription(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, sid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[subscriptionBody](w, r)
	if !ok {
		return
	}
	in := body.Subscription
	c := subscriptions.Changes{
		Reference: in.Reference, Mergeable: in.Mergeable.ptr(), PricesAreInclTax: in.PricesAreInclTax.ptr(), StartDate: in.StartDate,
		Amount: in.Amount, Discount: in.Discount.ptr(),
	}
	for _, f := range []struct {
		src *flexString
		dst **resource.ID
	}{{in.ProductID, &c.ProductID}, {in.ContactPersonID, &c.ContactPersonID}, {in.DocumentStyleID, &c.DocumentStyleID}} {
		if *f.dst, ok = flexOptionalID(w, f.src); !ok {
			return
		}
	}
	c.ProductID, c.DocumentStyleID = nonZero(c.ProductID), nonZero(c.DocumentStyleID)
	sub, err := s.deps.Subscriptions.Update(r.Context(), admin, sid, user(r).ID, c)
	s.writeSubscription(w, r, admin, http.StatusOK, sub, err)
}

// CancelSubscription implements ServerInterface.
func (s *server) CancelSubscription(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, sid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	var body struct {
		Subscription struct {
			EndDate *string `json:"end_date"`
		} `json:"subscription"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	sub, err := s.deps.Subscriptions.Cancel(r.Context(), admin, sid, user(r).ID, body.Subscription.EndDate)
	s.writeSubscription(w, r, admin, http.StatusOK, sub, err)
}

type additionalChargeJSON struct {
	ID               string  `json:"id"`
	AdministrationID string  `json:"administration_id"`
	ContactID        *string `json:"contact_id"`
	SubscriptionID   *string `json:"subscription_id"`
	ProductID        string  `json:"product_id"`
	DetailID         *string `json:"detail_id"`
	Amount           string  `json:"amount"`
	Price            string  `json:"price"`
	Period           string  `json:"period"`
	Description      string  `json:"description"`
	Identifier       *string `json:"identifier"`
	SalesInvoiceID   *string `json:"sales_invoice_id"`
}

func toAdditionalCharge(c subscriptions.Charge) additionalChargeJSON {
	return additionalChargeJSON{
		ID: c.ID.String(), AdministrationID: c.AdministrationID.String(), ContactID: idString(c.ContactID), SubscriptionID: idString(c.SubscriptionID),
		ProductID: c.ProductID.String(), DetailID: idString(c.DetailID), Amount: c.Amount, Price: c.Price.String(), Period: c.Period.Monthly(),
		Description: c.Description, Identifier: nz(c.Identifier), SalesInvoiceID: idString(c.InvoiceID),
	}
}

func (s *server) listCharges(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, includeBilled *string, owner func(resource.ID) subscriptions.Owner) {
	admin, oid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	list, err := s.deps.Subscriptions.Charges(r.Context(), admin, user(r).ID, owner(oid), deref(includeBilled) == "true")
	if err != nil {
		s.failSubscription(w, r, err)
		return
	}
	out := make([]additionalChargeJSON, len(list))
	for i, c := range list {
		out[i] = toAdditionalCharge(c)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) addCharge(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, owner func(resource.ID) subscriptions.Owner) {
	admin, oid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		ProductID   flexString  `json:"product_id"`
		Amount      flexString  `json:"amount"`
		Price       *flexString `json:"price"`
		Period      string      `json:"period"`
		Description string      `json:"description"`
		Identifier  *string     `json:"identifier"`
	}](w, r)
	if !ok {
		return
	}
	product, err := resource.ParseID(string(body.ProductID))
	if err != nil {
		symbolic(w, http.StatusNotFound, "Product not found", map[string]any{"product_id": "not_found"})
		return
	}
	c, err := s.deps.Subscriptions.AddCharge(r.Context(), admin, user(r).ID, owner(oid), subscriptions.ChargeInput{
		ProductID: product, Amount: string(body.Amount), Price: body.Price.ptr(), Period: body.Period, Description: body.Description, Identifier: body.Identifier,
	})
	if err != nil {
		s.failSubscription(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAdditionalCharge(c))
}

func ofSubscription(id resource.ID) subscriptions.Owner {
	return subscriptions.Owner{SubscriptionID: &id}
}
func ofContact(id resource.ID) subscriptions.Owner { return subscriptions.Owner{ContactID: &id} }

// ListSubscriptionAdditionalCharges implements ServerInterface.
func (s *server) ListSubscriptionAdditionalCharges(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, params ListSubscriptionAdditionalChargesParams) {
	s.listCharges(w, r, administrationID, id, (*string)(params.IncludeBilled), ofSubscription)
}

// CreateSubscriptionAdditionalCharge implements ServerInterface.
func (s *server) CreateSubscriptionAdditionalCharge(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.addCharge(w, r, administrationID, id, ofSubscription)
}

// ListContactAdditionalCharges implements ServerInterface.
func (s *server) ListContactAdditionalCharges(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, params ListContactAdditionalChargesParams) {
	s.listCharges(w, r, administrationID, id, (*string)(params.IncludeBilled), ofContact)
}

// CreateContactAdditionalCharge implements ServerInterface.
func (s *server) CreateContactAdditionalCharge(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.addCharge(w, r, administrationID, id, ofContact)
}

// CreateSubscriptionOneOffSalesInvoice implements ServerInterface.
func (s *server) CreateSubscriptionOneOffSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, sid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		Invoice struct {
			Details     *salesDetails `json:"details_attributes"`
			InvoiceDate *string       `json:"invoice_date"`
		} `json:"sales_invoice"`
	}](w, r)
	if !ok {
		return
	}
	details, ok := toDetailInputs(w, body.Invoice.Details)
	if !ok {
		return
	}
	inv, err := s.deps.Subscriptions.ScheduleOneOff(r.Context(), admin, sid, user(r).ID, details, body.Invoice.InvoiceDate)
	if err != nil {
		s.failSubscription(w, r, err)
		return
	}
	s.writeSalesInvoice(w, r, admin, http.StatusCreated, inv)
}

// ListSubscriptionTemplates implements ServerInterface: MoneyHarpy has no
// online checkout, so there are no templates.
func (s *server) ListSubscriptionTemplates(w http.ResponseWriter, r *http.Request, _ AdministrationId, params ListSubscriptionTemplatesParams) {
	page, perPage := pageNumbers(params.Page, params.PerPage)
	s.writeLinks(w, r, max(page, 1), pageSize(perPage, products.DefaultPerPage, products.MaxPerPage), 0)
	writeJSON(w, http.StatusOK, []any{})
}

func noSubscriptionTemplate(w http.ResponseWriter) {
	symbolic(w, http.StatusNotFound, "Record not found for model name: SubscriptionTemplate", nil)
}

// GetSubscriptionTemplateCheckoutIdentifier implements ServerInterface.
func (s *server) GetSubscriptionTemplateCheckoutIdentifier(w http.ResponseWriter, _ *http.Request, _ AdministrationId, _ Id, _ GetSubscriptionTemplateCheckoutIdentifierParams) {
	w.Header().Set("X-Deprecated-Endpoint", "Use POST subscription_templates/{id}/sales_link")
	noSubscriptionTemplate(w)
}

// CreateSubscriptionTemplateSalesLink implements ServerInterface.
func (s *server) CreateSubscriptionTemplateSalesLink(w http.ResponseWriter, _ *http.Request, _ AdministrationId, _ Id, _ CreateSubscriptionTemplateSalesLinkParams) {
	noSubscriptionTemplate(w)
}

// GetContactPaymentsMandate implements ServerInterface: no contact has a
// Moneybird Payments mandate, so it answers 404 with the status as body.
func (s *server) GetContactPaymentsMandate(w http.ResponseWriter, _ *http.Request, _ AdministrationId, _ ContactId) {
	writeJSON(w, http.StatusNotFound, http.StatusNotFound)
}

// paymentsDisabled answers as Moneybird does when Moneybird Payments is
// disabled for the administration: 403 with the status as body.
func paymentsDisabled(w http.ResponseWriter) {
	writeJSON(w, http.StatusForbidden, http.StatusForbidden)
}

// RequestContactPaymentsMandate implements ServerInterface.
func (s *server) RequestContactPaymentsMandate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, _ ContactId) {
	if admin, ok := parseID(w, administrationID); ok && s.requireAlso(w, r, admin, administration.Settings) {
		paymentsDisabled(w)
	}
}

// RequestContactPaymentsMandateURL implements ServerInterface.
func (s *server) RequestContactPaymentsMandateURL(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, _ ContactId) {
	if admin, ok := parseID(w, administrationID); ok && s.requireAlso(w, r, admin, administration.Settings) {
		paymentsDisabled(w)
	}
}

// DeleteContactPaymentsMandate implements ServerInterface.
func (s *server) DeleteContactPaymentsMandate(w http.ResponseWriter, _ *http.Request, _ AdministrationId, _ ContactId) {
	paymentsDisabled(w)
}

// portalContact checks the contact a portal link is for.
func (s *server) portalContact(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, contactID ContactId) (resource.ID, resource.ID, bool) {
	admin, contact, ok := parseIDs(w, administrationID, contactID)
	if !ok {
		return 0, 0, false
	}
	if _, err := s.deps.Contacts.Contact(r.Context(), admin, contact, user(r).ID); err != nil {
		s.failSubscription(w, r, err)
		return 0, 0, false
	}
	return admin, contact, true
}

func (s *server) appURL(path string) string { return strings.TrimSuffix(s.deps.BaseURL, "/") + path }

// GetCustomerContactPortalLink implements ServerInterface: MoneyHarpy has
// no customer portal, so the link opens the contact in the app.
func (s *server) GetCustomerContactPortalLink(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, contactID ContactId) {
	admin, contact, ok := s.portalContact(w, r, administrationID, contactID)
	if !ok {
		return
	}
	ref := resource.Ref{ID: contact, Type: resource.TypeContact, AdministrationID: admin}
	writeJSON(w, http.StatusOK, s.appURL(ref.URI()))
}

// GetCustomerContactPortalInvoicesLink implements ServerInterface: the link
// opens the contact's sales invoices in the app.
func (s *server) GetCustomerContactPortalInvoicesLink(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, contactID ContactId) {
	admin, contact, ok := s.portalContact(w, r, administrationID, contactID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.appURL("/"+admin.String()+"/sales_invoices?filter=contact_id:"+contact.String()))
}

// GetCustomerContactPortalSubscriptionLink implements ServerInterface:
// Moneybird links only to subscriptions sold through a template, which
// MoneyHarpy has none of, so it answers 404 as Moneybird does for others.
func (s *server) GetCustomerContactPortalSubscriptionLink(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, contactID ContactId, id Id) {
	if _, _, ok := s.portalContact(w, r, administrationID, contactID); !ok {
		return
	}
	if _, ok := parseID(w, id); ok {
		s.failSubscription(w, r, subscriptions.ErrNotFound)
	}
}

// GetVerifications implements ServerInterface: MoneyHarpy verifies nothing
// yet, so it answers as Moneybird does when no verifications are present.
func (s *server) GetVerifications(w http.ResponseWriter, _ *http.Request, _ AdministrationId) {
	writeJSON(w, http.StatusOK, struct{}{})
}

type downloadJSON struct {
	ID          string    `json:"id"`
	UserID      *string   `json:"user_id"`
	Type        string    `json:"download_type"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Failed      bool      `json:"failed"`
	Downloaded  bool      `json:"downloaded"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func toDownload(d downloads.Download) downloadJSON {
	return downloadJSON{
		ID: d.ID.String(), UserID: idString(d.UserID), Type: d.Type, Filename: d.Filename, ContentType: d.ContentType, Failed: d.Failed,
		Downloaded: d.Downloaded, Size: d.Size, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

// permitted reports whether the request's user, and connection, holds one
// of the permissions in the administration.
func (s *server) permitted(r *http.Request, admin resource.ID, perms []administration.Permission) bool {
	if g, ok := grantFrom(r.Context()); ok && !g.Allows(perms...) {
		return false
	}
	return s.allowed(r.Context(), admin, user(r).ID, perms) == nil
}

func flexQueryBool(v *string) *bool {
	if v == nil {
		return nil
	}
	b := *v == "true"
	return &b
}

// ListDownloads implements ServerInterface: only downloads of the types the
// user may open are listed.
func (s *server) ListDownloads(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListDownloadsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q := downloads.Query{
		Type: deref(params.DownloadType), Downloaded: flexQueryBool((*string)(params.Downloaded)), Failed: flexQueryBool((*string)(params.Failed)), Types: []string{},
	}
	for _, t := range downloads.Types() {
		if perms, _ := downloads.Permissions(t); s.permitted(r, admin, perms) {
			q.Types = append(q.Types, t)
		}
	}
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	list, total, err := s.deps.Downloads.List(r.Context(), admin, user(r).ID, q)
	if errors.Is(err, downloads.ErrUnknownType) {
		symbolic(w, http.StatusBadRequest, "download_type does not have a valid value", nil)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]downloadJSON, len(list))
	for i, d := range list {
		out[i] = toDownload(d)
	}
	s.writeLinks(w, r, max(q.Page, 1), pageSize(q.PerPage, downloads.DefaultPerPage, downloads.MaxPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

// DownloadDownload implements ServerInterface: it checks the permission
// the download's type needs and redirects to a link to the file.
func (s *server) DownloadDownload(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, did, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	d, err := s.deps.Downloads.Download(r.Context(), admin, did, user(r).ID)
	if errors.Is(err, downloads.ErrNotFound) {
		symbolic(w, http.StatusNotFound, "Record not found for model name: Download", nil)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	perms, _ := downloads.Permissions(d.Type)
	if !s.permitted(r, admin, perms) {
		names := make([]string, len(perms))
		for i, p := range perms {
			names[i] = string(p)
		}
		symbolic(w, http.StatusUnauthorized, "No access to "+strings.Join(names, " or "), nil)
		return
	}
	_, body, err := s.deps.Downloads.Open(r.Context(), admin, did, user(r).ID)
	if errors.Is(err, downloads.ErrFailed) {
		symbolic(w, http.StatusNotFound, "Record not found for model name: Download", nil)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	_ = body.Close()
	s.redirectToFile(w, fileLink{Kind: fileDownload, Admin: admin, ID: did, User: user(r).ID})
}
