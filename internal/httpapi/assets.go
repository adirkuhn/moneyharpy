package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/assets"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/webhooks"
)

type valueChangeJSON struct {
	Type             string  `json:"type"`
	Date             string  `json:"date"`
	Amount           string  `json:"amount"`
	Description      *string `json:"description"`
	ExternallyBooked bool    `json:"externally_booked"`
	AssetID          string  `json:"asset_id"`
}

type reserveJSON struct {
	ID         string  `json:"id"`
	Amount     string  `json:"amount"`
	Date       string  `json:"date"`
	AssetID    *string `json:"asset_id,omitempty"`
	DisposalID *string `json:"disposal_id,omitempty"`
}

type disposalJSON struct {
	Date                    string       `json:"date"`
	Reason                  string       `json:"reason"`
	ExternallyBooked        bool         `json:"externally_booked"`
	AssetID                 string       `json:"asset_id"`
	ReinvestmentReserveSale *reserveJSON `json:"reinvestment_reserve_sale"`
}

type sourceJSON struct {
	ID                            string  `json:"id"`
	AssetID                       string  `json:"asset_id"`
	DetailID                      *string `json:"detail_id,omitempty"`
	GeneralJournalDocumentEntryID *string `json:"general_journal_document_entry_id,omitempty"`
}

type planJSON struct {
	LifespanInYears     float64 `json:"lifespan_in_years"`
	ResidualValue       string  `json:"residual_value"`
	Active              bool    `json:"active"`
	MonthlyDepreciation string  `json:"monthly_depreciation"`
}

type assetJSON struct {
	ID                          string            `json:"id"`
	AdministrationID            string            `json:"administration_id"`
	LedgerAccountID             string            `json:"ledger_account_id"`
	Name                        string            `json:"name"`
	PurchaseDate                string            `json:"purchase_date"`
	PurchaseValue               string            `json:"purchase_value"`
	CalculatedPurchaseValue     string            `json:"calculated_purchase_value"`
	CurrentValue                string            `json:"current_value"`
	ValueChangePlan             *planJSON         `json:"value_change_plan"`
	Disposal                    *disposalJSON     `json:"disposal"`
	ValueChanges                []valueChangeJSON `json:"value_changes"`
	Sources                     []sourceJSON      `json:"sources"`
	ReinvestmentReservePurchase *reserveJSON      `json:"reinvestment_reserve_purchase"`
	URI                         string            `json:"uri"`
	CreatedAt                   time.Time         `json:"created_at"`
	UpdatedAt                   time.Time         `json:"updated_at"`
}

func toValueChange(a assets.Asset, c assets.ValueChange) valueChangeJSON {
	return valueChangeJSON{
		Type: c.Type, Date: c.Date.Format(time.DateOnly), Amount: c.Amount.String(), Description: c.Description,
		ExternallyBooked: c.ExternallyBooked, AssetID: a.ID.String(),
	}
}

func toReservePurchase(a assets.Asset, r assets.ReserveBooking) reserveJSON {
	id := a.ID.String()
	return reserveJSON{ID: r.ID.String(), Amount: r.Amount.String(), Date: a.PurchaseDate.Format(time.DateOnly), AssetID: &id}
}

func toDisposal(a assets.Asset, d assets.Disposal) disposalJSON {
	out := disposalJSON{Date: d.Date.Format(time.DateOnly), Reason: d.Reason, ExternallyBooked: d.ExternallyBooked, AssetID: a.ID.String()}
	if s := d.ReserveSale; s != nil {
		sale := toReserveSale(d, *s)
		out.ReinvestmentReserveSale = &sale
	}
	return out
}

func toReserveSale(d assets.Disposal, s assets.ReserveBooking) reserveJSON {
	id := d.ID.String()
	return reserveJSON{ID: s.ID.String(), Amount: s.Amount.String(), Date: d.Date.Format(time.DateOnly), DisposalID: &id}
}

func toSource(a assets.Asset, s assets.Source) sourceJSON {
	return sourceJSON{ID: s.ID.String(), AssetID: a.ID.String(), DetailID: idString(s.DetailID), GeneralJournalDocumentEntryID: idString(s.EntryID)}
}

func toAsset(a assets.Asset) assetJSON {
	out := assetJSON{
		ID: a.ID.String(), AdministrationID: a.AdministrationID.String(), LedgerAccountID: a.LedgerAccountID.String(), Name: a.Name,
		PurchaseDate: a.PurchaseDate.Format(time.DateOnly), PurchaseValue: a.PurchaseValue.String(),
		CalculatedPurchaseValue: a.CalculatedPurchaseValue().String(), CurrentValue: a.CurrentValue().String(),
		ValueChanges: []valueChangeJSON{}, Sources: []sourceJSON{}, URI: a.Ref().URI(), CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
	if p := a.Plan; p != nil {
		out.ValueChangePlan = &planJSON{
			LifespanInYears: p.LifespanYears(), ResidualValue: p.ResidualValue.String(), Active: a.PlanActive(), MonthlyDepreciation: a.MonthlyDepreciation().String(),
		}
	}
	if d := a.Disposal; d != nil {
		disposal := toDisposal(a, *d)
		out.Disposal = &disposal
	}
	for _, c := range a.ValueChanges {
		out.ValueChanges = append(out.ValueChanges, toValueChange(a, c))
	}
	for _, s := range a.Sources {
		out.Sources = append(out.Sources, toSource(a, s))
	}
	if r := a.ReservePurchase; r != nil {
		purchase := toReservePurchase(a, *r)
		out.ReinvestmentReservePurchase = &purchase
	}
	return out
}

func (s *server) writeAsset(w http.ResponseWriter, r *http.Request, status int, a assets.Asset, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, toAsset(a))
}

// ListAssets implements ServerInterface.
func (s *server) ListAssets(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListAssetsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q := assets.Query{OnlyActive: deref(params.Active) != ListAssetsParamsActiveFalse}
	if params.LedgerAccountId != nil {
		id, ok := parseID(w, *params.LedgerAccountId)
		if !ok {
			return
		}
		q.LedgerAccountID = &id
	}
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	list, total, err := s.deps.Assets.List(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]assetJSON, len(list))
	for i, a := range list {
		out[i] = toAsset(a)
	}
	s.writeLinks(w, r, max(q.Page, 1), pageSize(q.PerPage, assets.DefaultPerPage, assets.MaxPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

type assetBody struct {
	Asset struct {
		Name            *string     `json:"name"`
		LedgerAccountID *flexString `json:"ledger_account_id"`
		PurchaseDate    *string     `json:"purchase_date"`
		PurchaseValue   *flexString `json:"purchase_value"`
		Plan            *struct {
			LifespanInYears *flexString `json:"lifespan_in_years"`
			ResidualValue   *flexString `json:"residual_value"`
		} `json:"value_change_plan_attributes"`
	} `json:"asset"`
}

// readAssetInput reads an asset as sent, answering 400 with Moneybird's
// messages for values that are not what they should be.
func readAssetInput(w http.ResponseWriter, r *http.Request) (assets.Input, bool) {
	body, ok := decodeBody[assetBody](w, r)
	if !ok {
		return assets.Input{}, false
	}
	b := body.Asset
	in := assets.Input{Name: b.Name}
	var problems []string
	if b.LedgerAccountID != nil {
		id, err := resource.ParseID(string(*b.LedgerAccountID))
		if err != nil {
			problems = append(problems, "asset[ledger_account_id] is invalid")
		}
		in.LedgerAccountID = &id
	}
	if b.PurchaseDate != nil {
		d, err := time.Parse(time.DateOnly, *b.PurchaseDate)
		if err != nil {
			problems = append(problems, "asset[purchase_date] is invalid")
		}
		in.PurchaseDate = &d
	}
	if b.PurchaseValue != nil {
		v, err := money.ParseAmount(string(*b.PurchaseValue))
		if err != nil {
			problems = append(problems, "asset[purchase_value] does not have a valid value")
		}
		in.PurchaseValue = &v
	}
	if p := b.Plan; p != nil {
		in.Plan = &assets.PlanInput{}
		if p.LifespanInYears != nil {
			y, err := strconv.ParseFloat(strings.ReplaceAll(string(*p.LifespanInYears), ",", "."), 64)
			if err != nil {
				problems = append(problems, "asset[value_change_plan_attributes][lifespan_in_years] does not have a valid value")
			}
			in.Plan.LifespanYears = &y
		}
		if p.ResidualValue != nil {
			v, err := money.ParseAmount(string(*p.ResidualValue))
			if err != nil {
				problems = append(problems, "asset[value_change_plan_attributes][residual_value] does not have a valid value")
			}
			in.Plan.ResidualValue = &v
		}
	}
	if len(problems) > 0 {
		writeError(w, http.StatusBadRequest, strings.Join(problems, ", "))
		return in, false
	}
	return in, true
}

// CreateAsset implements ServerInterface.
func (s *server) CreateAsset(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	in, ok := readAssetInput(w, r)
	if !ok {
		return
	}
	a, err := s.deps.Assets.Create(r.Context(), admin, user(r).ID, in)
	s.writeAsset(w, r, http.StatusCreated, a, err)
}

// GetAsset implements ServerInterface.
func (s *server) GetAsset(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	a, err := s.deps.Assets.Asset(r.Context(), admin, rid, user(r).ID)
	s.writeAsset(w, r, http.StatusOK, a, err)
}

// UpdateAsset implements ServerInterface.
func (s *server) UpdateAsset(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	in, ok := readAssetInput(w, r)
	if !ok {
		return
	}
	a, err := s.deps.Assets.Update(r.Context(), admin, rid, user(r).ID, in)
	s.writeAsset(w, r, http.StatusOK, a, err)
}

// DeleteAsset implements ServerInterface.
func (s *server) DeleteAsset(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	s.noContent(w, r, s.deps.Assets.Delete(r.Context(), admin, rid, user(r).ID))
}

func (s *server) noContent(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateAssetSource implements ServerInterface.
func (s *server) CreateAssetSource(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		DetailID *flexString `json:"detail_id"`
		EntryID  *flexString `json:"general_journal_document_entry_id"`
	}](w, r)
	if !ok {
		return
	}
	var in assets.SourceInput
	for _, f := range []struct {
		name string
		src  *flexString
		dst  **resource.ID
	}{{"detail_id", body.DetailID, &in.DetailID}, {"general_journal_document_entry_id", body.EntryID, &in.EntryID}} {
		if f.src == nil {
			continue
		}
		v, err := resource.ParseID(string(*f.src))
		if err != nil {
			writeError(w, http.StatusBadRequest, f.name+" is invalid")
			return
		}
		*f.dst = &v
	}
	if (in.DetailID == nil) == (in.EntryID == nil) {
		writeError(w, http.StatusBadRequest, "detail_id, general_journal_document_entry_id are missing, exactly one parameter must be provided")
		return
	}
	src, err := s.deps.Assets.AddSource(r.Context(), admin, rid, user(r).ID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toSource(assets.Asset{ID: rid}, src))
}

// DeleteAssetSource implements ServerInterface.
func (s *server) DeleteAssetSource(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, sourceID Identifier) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	src, ok := parseID(w, sourceID)
	if !ok {
		return
	}
	s.noContent(w, r, s.deps.Assets.DeleteSource(r.Context(), admin, rid, src, user(r).ID))
}

// readDay reads a date field, answering 400 as Moneybird does.
func readDay(w http.ResponseWriter, s string) (time.Time, bool) {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Date is invalid")
		return d, false
	}
	return d, true
}

func (s *server) createValueChange(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, kind string) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		Date             string     `json:"date"`
		Amount           flexString `json:"amount"`
		Description      string     `json:"description"`
		ExternallyBooked flexBool   `json:"externally_booked"`
	}](w, r)
	if !ok {
		return
	}
	day, ok := readDay(w, body.Date)
	if !ok {
		return
	}
	amount, err := money.ParseAmount(string(body.Amount))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Amount does not have a valid value")
		return
	}
	in := assets.ChangeInput{Date: day, Amount: amount, Description: body.Description, ExternallyBooked: bool(body.ExternallyBooked)}
	c, err := s.deps.Assets.AddValueChange(r.Context(), admin, rid, user(r).ID, kind, in)
	s.writeValueChange(w, r, rid, c, err)
}

func (s *server) writeValueChange(w http.ResponseWriter, r *http.Request, asset resource.ID, c assets.ValueChange, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toValueChange(assets.Asset{ID: asset}, c))
}

// CreateAssetManualValueChange implements ServerInterface.
func (s *server) CreateAssetManualValueChange(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.createValueChange(w, r, administrationID, id, assets.Manual)
}

// CreateAssetArbitraryValueChange implements ServerInterface.
func (s *server) CreateAssetArbitraryValueChange(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.createValueChange(w, r, administrationID, id, assets.Arbitrary)
}

func (s *server) dispose(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, kind string) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		Date        string  `json:"date"`
		Description *string `json:"description"`
	}](w, r)
	if !ok {
		return
	}
	day, ok := readDay(w, body.Date)
	if !ok {
		return
	}
	c, err := s.deps.Assets.Dispose(r.Context(), admin, rid, user(r).ID, kind, day, body.Description)
	s.writeValueChange(w, r, rid, c, err)
}

// CreateAssetDivestmentValueChange implements ServerInterface.
func (s *server) CreateAssetDivestmentValueChange(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.dispose(w, r, administrationID, id, assets.Divestment)
}

// CreateAssetFullDepreciationValueChange implements ServerInterface.
func (s *server) CreateAssetFullDepreciationValueChange(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.dispose(w, r, administrationID, id, assets.FullDepreciation)
}

// CreateAssetRetroactiveLinearValueChanges implements ServerInterface. It
// books at once; Moneybird does it in the background, with the same
// answer.
func (s *server) CreateAssetRetroactiveLinearValueChanges(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	s.noContent(w, r, s.deps.Assets.AddRetroactiveLinear(r.Context(), admin, rid, user(r).ID))
}

// CreateAssetDisposal implements ServerInterface.
func (s *server) CreateAssetDisposal(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[AssetDisposalRequest](w, r)
	if !ok {
		return
	}
	day, ok := readDay(w, body.Date.Format(time.DateOnly))
	if !ok {
		return
	}
	d, err := s.deps.Assets.AddDisposal(r.Context(), admin, rid, user(r).ID, day, string(body.Reason))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toDisposal(assets.Asset{ID: rid}, d))
}

// readReserveAmount reads the positive amount of a reinvestment reserve
// booking.
func readReserveAmount(w http.ResponseWriter, r *http.Request) (money.Amount, bool) {
	body, ok := decodeBody[struct {
		Amount *flexString `json:"amount"`
	}](w, r)
	if !ok {
		return 0, false
	}
	if body.Amount == nil {
		writeError(w, http.StatusBadRequest, "Amount is missing, Amount does not have a valid value")
		return 0, false
	}
	a, err := money.ParseAmount(string(*body.Amount))
	if err != nil || a <= 0 {
		writeError(w, http.StatusBadRequest, "Amount does not have a valid value")
		return 0, false
	}
	return a, true
}

// CreateAssetReinvestmentReservePurchase implements ServerInterface.
func (s *server) CreateAssetReinvestmentReservePurchase(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	amount, ok := readReserveAmount(w, r)
	if !ok {
		return
	}
	res, err := s.deps.Assets.AddReservePurchase(r.Context(), admin, rid, user(r).ID, amount)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	a, err := s.deps.Assets.Asset(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toReservePurchase(a, res))
}

// DeleteAssetReinvestmentReservePurchase implements ServerInterface.
func (s *server) DeleteAssetReinvestmentReservePurchase(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	s.noContent(w, r, s.deps.Assets.DeleteReservePurchase(r.Context(), admin, rid, user(r).ID))
}

// CreateAssetReinvestmentReserveSale implements ServerInterface.
func (s *server) CreateAssetReinvestmentReserveSale(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	amount, ok := readReserveAmount(w, r)
	if !ok {
		return
	}
	res, err := s.deps.Assets.AddReserveSale(r.Context(), admin, rid, user(r).ID, amount)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	a, err := s.deps.Assets.Asset(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toReserveSale(*a.Disposal, res))
}

// DeleteAssetReinvestmentReserveSale implements ServerInterface.
func (s *server) DeleteAssetReinvestmentReserveSale(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	s.noContent(w, r, s.deps.Assets.DeleteReserveSale(r.Context(), admin, rid, user(r).ID))
}

type assetsReportLineJSON struct {
	AssetID                        string  `json:"asset_id"`
	LedgerAccountID                string  `json:"ledger_account_id"`
	Name                           string  `json:"name"`
	PurchaseDate                   string  `json:"purchase_date"`
	Lifespan                       *string `json:"lifespan"`
	PurchaseValue                  string  `json:"purchase_value"`
	CalculatedPurchaseValue        string  `json:"calculated_purchase_value"`
	ResidualValue                  string  `json:"residual_value"`
	TotalValueChangesSincePurchase string  `json:"total_value_changes_since_purchase"`
	ValueAtBegin                   string  `json:"value_at_begin"`
	Investment                     string  `json:"investment"`
	Depreciation                   string  `json:"depreciation"`
	Divestment                     string  `json:"divestment"`
	ValueChanges                   string  `json:"value_changes"`
	ValueAtEnd                     string  `json:"value_at_end"`
	ValueNotInUse                  string  `json:"value_not_in_use"`
}

// GetAssetsReport implements ServerInterface.
func (s *server) GetAssetsReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetAssetsReportParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	lines, err := s.deps.Assets.Report(r.Context(), admin, user(r).ID, deref((*string)(params.Period)))
	switch {
	case errors.Is(err, assets.ErrPeriod):
		writeError(w, http.StatusBadRequest, "Period is invalid")
		return
	case errors.Is(err, assets.ErrLongPeriod):
		writeError(w, http.StatusBadRequest, "Period cannot exceed 12 months")
		return
	case err != nil:
		s.fail(w, r, err)
		return
	}
	out := make([]assetsReportLineJSON, len(lines))
	for i, l := range lines {
		a := l.Asset
		out[i] = assetsReportLineJSON{
			AssetID: a.ID.String(), LedgerAccountID: a.LedgerAccountID.String(), Name: a.Name, PurchaseDate: a.PurchaseDate.Format(time.DateOnly),
			PurchaseValue: a.PurchaseValue.String(), CalculatedPurchaseValue: a.CalculatedPurchaseValue().String(), ResidualValue: l.ResidualValue.String(),
			TotalValueChangesSincePurchase: l.TotalSincePurchase.String(), ValueAtBegin: l.ValueAtBegin.String(), Investment: l.Investment.String(),
			Depreciation: l.Depreciation.String(), Divestment: l.Divestment.String(), ValueChanges: l.ValueChanges.String(),
			ValueAtEnd: l.ValueAtEnd.String(), ValueNotInUse: l.NotInUse.String(),
		}
		if l.LifespanSeconds != nil {
			v := strconv.FormatInt(*l.LifespanSeconds, 10)
			out[i].Lifespan = &v
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// AssetNotifier publishes the value changes MoneyHarpy books or removes by
// itself, such as the monthly depreciation, as webhook events.
type AssetNotifier struct {
	Webhooks *webhooks.Service
	Logger   *slog.Logger
}

// Notify implements assets.Notifier.
func (n AssetNotifier) Notify(ctx context.Context, admin resource.ID, action string, a assets.Asset, c assets.ValueChange) {
	entity, _ := json.Marshal(toValueChange(a, c))
	e := webhooks.Event{EntityType: "Asset", EntityID: a.ID.String(), Action: action, Entity: entity}
	if err := n.Webhooks.Publish(context.WithoutCancel(ctx), admin, e); err != nil && n.Logger != nil {
		n.Logger.ErrorContext(ctx, "publish webhook event", "action", action, "error", err)
	}
}
