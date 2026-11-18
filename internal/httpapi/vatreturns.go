package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/vatreturns"
)

type vatBoxJSON struct {
	Box            string `json:"box"`
	Base           string `json:"base"`
	Tax            string `json:"tax"`
	PeriodBase     string `json:"period_base"`
	PeriodTax      string `json:"period_tax"`
	CorrectionBase string `json:"correction_base"`
	CorrectionTax  string `json:"correction_tax"`
}

type vatCorrectionBoxJSON struct {
	Box  string `json:"box"`
	Base string `json:"base"`
	Tax  string `json:"tax"`
}

type vatCorrectionJSON struct {
	PeriodFrom string                 `json:"period_from"`
	PeriodTo   string                 `json:"period_to"`
	Boxes      []vatCorrectionBoxJSON `json:"boxes"`
}

type vatICPLineJSON struct {
	ContactID string `json:"contact_id"`
	Name      string `json:"name"`
	Country   string `json:"country"`
	VATNumber string `json:"vat_number"`
	Kind      string `json:"kind"`
	Amount    string `json:"amount"`
}

type vatFiguresJSON struct {
	Kind        string              `json:"kind"`
	PeriodFrom  string              `json:"period_from"`
	PeriodTo    string              `json:"period_to"`
	Granularity string              `json:"granularity"`
	Deadline    string              `json:"deadline"`
	Boxes       []vatBoxJSON        `json:"boxes"`
	Corrections []vatCorrectionJSON `json:"corrections"`
	ICPLines    []vatICPLineJSON    `json:"icp_lines"`
	Owed        string              `json:"owed"`
	InputTax    string              `json:"input_tax"`
	Total       string              `json:"total"`
}

type vatPaymentJSON struct {
	ID                  string `json:"id"`
	FinancialMutationID string `json:"financial_mutation_id"`
	FinancialAccountID  string `json:"financial_account_id"`
	PaymentDate         string `json:"payment_date"`
	Price               string `json:"price"`
}

type vatReturnJSON struct {
	vatFiguresJSON
	ID               string           `json:"id"`
	AdministrationID string           `json:"administration_id"`
	URI              string           `json:"uri"`
	SentOn           string           `json:"sent_on"`
	State            string           `json:"state"`
	Paid             string           `json:"paid"`
	Unpaid           string           `json:"unpaid"`
	Payments         []vatPaymentJSON `json:"payments"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

type vatChecksJSON struct {
	OpenTransactions     int    `json:"open_transactions"`
	UntaxedPurchases     string `json:"untaxed_purchases"`
	BookingsWithoutVAT   int    `json:"bookings_without_vat"`
	UnprocessedDocuments int    `json:"unprocessed_documents"`
}

type vatPreviewJSON struct {
	vatFiguresJSON
	Checks vatChecksJSON `json:"checks"`
}

type vatPeriodJSON struct {
	PeriodFrom  string `json:"period_from"`
	PeriodTo    string `json:"period_to"`
	Granularity string `json:"granularity"`
}

// toVatFigures writes a return's amounts in whole euros, each box with its
// own and corrected amounts rounded as the return declares them.
func toVatFigures(r vatreturns.Return) vatFiguresJSON {
	corrected := vatreturns.Plus(func() [][]vatreturns.Box {
		var lists [][]vatreturns.Box
		for _, c := range r.Corrections {
			lists = append(lists, c.Boxes)
		}
		return lists
	}()...)
	find := func(list []vatreturns.Box, code string) vatreturns.Box {
		for _, b := range list {
			if b.Code == code {
				return b
			}
		}
		return vatreturns.Box{Code: code}
	}
	t := r.Totals()
	out := vatFiguresJSON{
		Kind: r.Kind, PeriodFrom: r.Period.From.Format(time.DateOnly), PeriodTo: r.Period.To.Format(time.DateOnly), Granularity: r.Granularity,
		Deadline: r.Deadline().Format(time.DateOnly), Boxes: []vatBoxJSON{}, Corrections: []vatCorrectionJSON{}, ICPLines: []vatICPLineJSON{},
		Owed: t.Owed.String(), InputTax: t.Input.String(), Total: t.Total.String(),
	}
	for _, all := range r.AllBoxes() {
		own, corr := find(r.Boxes, all.Code), find(corrected, all.Code)
		out.Boxes = append(out.Boxes, vatBoxJSON{
			Box: all.Code, Base: vatreturns.RoundBase(all.Base).String(), Tax: vatreturns.RoundTax(all.Code, all.Tax).String(),
			PeriodBase: vatreturns.RoundBase(own.Base).String(), PeriodTax: vatreturns.RoundTax(own.Code, own.Tax).String(),
			CorrectionBase: vatreturns.RoundBase(corr.Base).String(), CorrectionTax: vatreturns.RoundTax(corr.Code, corr.Tax).String(),
		})
	}
	for _, c := range r.Corrections {
		cj := vatCorrectionJSON{PeriodFrom: c.Period.From.Format(time.DateOnly), PeriodTo: c.Period.To.Format(time.DateOnly), Boxes: []vatCorrectionBoxJSON{}}
		for _, b := range c.Boxes {
			cj.Boxes = append(cj.Boxes, vatCorrectionBoxJSON{Box: b.Code, Base: vatreturns.RoundBase(b.Base).String(), Tax: vatreturns.RoundTax(b.Code, b.Tax).String()})
		}
		out.Corrections = append(out.Corrections, cj)
	}
	for _, l := range r.ICP {
		out.ICPLines = append(out.ICPLines, vatICPLineJSON{
			ContactID: l.ContactID.String(), Name: l.Name, Country: l.Country, VATNumber: l.VATNumber, Kind: l.Kind, Amount: vatreturns.RoundBase(l.Amount).String(),
		})
	}
	return out
}

func toVatReturn(r vatreturns.Return) vatReturnJSON {
	out := vatReturnJSON{
		vatFiguresJSON: toVatFigures(r), ID: r.ID.String(), AdministrationID: r.AdministrationID.String(), URI: r.Ref().URI(),
		SentOn: r.SentOn.Format(time.DateOnly), State: r.State(), Paid: r.Paid().String(), Unpaid: r.Unpaid().String(),
		Payments: []vatPaymentJSON{}, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	for _, p := range r.Payments {
		out.Payments = append(out.Payments, vatPaymentJSON{
			ID: p.ID.String(), FinancialMutationID: p.MutationID.String(), FinancialAccountID: p.AccountID.String(),
			PaymentDate: p.Date.Format(time.DateOnly), Price: p.Price.String(),
		})
	}
	return out
}

// vatStart checks the administration and that the user may see sales too,
// as a return covers them.
func (s *server) vatStart(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) (resource.ID, bool) {
	admin, ok := parseID(w, administrationID)
	if !ok || !s.requireAlso(w, r, admin, administration.SalesInvoices) {
		return 0, false
	}
	return admin, true
}

// vatPeriod reads a period written as YYYYMMDD..YYYYMMDD.
func vatPeriod(w http.ResponseWriter, raw string) (money.Period, bool) {
	from, to, _ := strings.Cut(raw, "..")
	f, err1 := time.Parse("20060102", from)
	t, err2 := time.Parse("20060102", to)
	if err1 != nil || err2 != nil || t.Before(f) {
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"period": {"is not a period (YYYYMMDD..YYYYMMDD)"}}))
		return money.Period{}, false
	}
	return money.Period{From: f, To: t}, true
}

// ListVatReturns implements ServerInterface.
func (s *server) ListVatReturns(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := s.vatStart(w, r, administrationID)
	if !ok {
		return
	}
	list, err := s.deps.VatReturns.Returns(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]vatReturnJSON, len(list))
	for i, v := range list {
		out[i] = toVatReturn(v)
	}
	writeJSON(w, http.StatusOK, out)
}

// SubmitVatReturn implements ServerInterface.
func (s *server) SubmitVatReturn(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := s.vatStart(w, r, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		VatReturn struct {
			Kind     string `json:"kind"`
			Period   string `json:"period"`
			ICPLines []struct {
				ContactID flexString `json:"contact_id"`
				Kind      string     `json:"kind"`
			} `json:"icp_lines"`
		} `json:"vat_return"`
	}](w, r)
	if !ok {
		return
	}
	p, ok := vatPeriod(w, body.VatReturn.Period)
	if !ok {
		return
	}
	kinds := map[resource.ID]string{}
	for _, l := range body.VatReturn.ICPLines {
		id, ok := flexID(w, &l.ContactID)
		if !ok {
			return
		}
		kinds[*id] = l.Kind
	}
	v, err := s.deps.VatReturns.Submit(r.Context(), admin, user(r).ID, vatreturns.Filing{Kind: body.VatReturn.Kind, Period: p, ICPKinds: kinds})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toVatReturn(v))
}

// ListVatReturnPeriods implements ServerInterface.
func (s *server) ListVatReturnPeriods(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListVatReturnPeriodsParams) {
	admin, ok := s.vatStart(w, r, administrationID)
	if !ok {
		return
	}
	list, err := s.deps.VatReturns.Periods(r.Context(), admin, user(r).ID, string(params.Kind))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]vatPeriodJSON, len(list))
	for i, c := range list {
		out[i] = vatPeriodJSON{PeriodFrom: c.Period.From.Format(time.DateOnly), PeriodTo: c.Period.To.Format(time.DateOnly), Granularity: c.Granularity}
	}
	writeJSON(w, http.StatusOK, out)
}

// PreviewVatReturn implements ServerInterface.
func (s *server) PreviewVatReturn(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params PreviewVatReturnParams) {
	admin, ok := s.vatStart(w, r, administrationID)
	if !ok {
		return
	}
	p, ok := vatPeriod(w, params.Period)
	if !ok {
		return
	}
	d, err := s.deps.VatReturns.Preview(r.Context(), admin, user(r).ID, string(params.Kind), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, vatPreviewJSON{vatFiguresJSON: toVatFigures(d.Return), Checks: vatChecksJSON{
		OpenTransactions: d.Checks.OpenTransactions, UntaxedPurchases: d.Checks.UntaxedPurchases.String(),
		BookingsWithoutVAT: d.Checks.BookingsWithoutVAT, UnprocessedDocuments: d.Checks.UnprocessedDocuments,
	}})
}

// GetVatReturn implements ServerInterface.
func (s *server) GetVatReturn(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, ok := s.vatStart(w, r, administrationID)
	if !ok {
		return
	}
	rid, ok := parseID(w, id)
	if !ok {
		return
	}
	v, err := s.deps.VatReturns.Return(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toVatReturn(v))
}

// DeleteVatReturn implements ServerInterface.
func (s *server) DeleteVatReturn(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, ok := s.vatStart(w, r, administrationID)
	if !ok {
		return
	}
	rid, ok := parseID(w, id)
	if !ok {
		return
	}
	if err := s.deps.VatReturns.Delete(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
