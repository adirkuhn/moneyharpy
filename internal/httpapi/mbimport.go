package httpapi

import (
	"net/http"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/mbimport"
)

func toMoneybirdImport(j mbimport.Job) MoneybirdImport {
	out := MoneybirdImport{
		SourceAdministrationId: j.SourceID, SourceName: j.SourceName, State: MoneybirdImportState(j.State),
		Steps: make([]MoneybirdImportStep, len(j.Steps)), WaitingUntil: j.WaitingUntil, Error: j.Error,
		NextAttemptAt: retryAt(j),
		StartedAt:     j.StartedAt, FinishedAt: j.FinishedAt, UpdatedAt: j.UpdatedAt,
	}
	for i, st := range j.Steps {
		problems := st.Problems
		if problems == nil {
			problems = []string{}
		}
		out.Steps[i] = MoneybirdImportStep{
			Kind: MoneybirdImportStepKind(st.Kind), Total: st.Total, Imported: st.Imported, Skipped: st.Skipped, Failed: st.Failed,
			Done: st.Done, Problems: problems,
		}
	}
	return out
}

// GetMoneybirdImport implements ServerInterface.
func (s *server) GetMoneybirdImport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	j, err := s.deps.MoneybirdImport.Status(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toMoneybirdImport(j))
}

// StartMoneybirdImport implements ServerInterface.
func (s *server) StartMoneybirdImport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[MoneybirdImportRequest](w, r)
	if !ok {
		return
	}
	j, err := s.deps.MoneybirdImport.Start(r.Context(), admin, user(r).ID, body.MoneybirdImport.Token, body.MoneybirdImport.AdministrationId)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toMoneybirdImport(j))
}

// ListMoneybirdAdministrations implements ServerInterface.
func (s *server) ListMoneybirdAdministrations(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[ListMoneybirdAdministrationsJSONBody](w, r)
	if !ok {
		return
	}
	list, err := s.deps.MoneybirdImport.Administrations(r.Context(), admin, user(r).ID, body.Token)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]MoneybirdAdministration, len(list))
	for i, a := range list {
		country, currency := a.Country, a.Currency
		out[i] = MoneybirdAdministration{Id: a.ID, Name: a.Name, Country: &country, Currency: &currency}
	}
	writeJSON(w, http.StatusOK, out)
}

// retryAt is when a queued import tries again after a failure; nil while it
// runs or once it ended.
func retryAt(j mbimport.Job) *time.Time {
	if j.State != mbimport.Running || j.Error == "" {
		return nil
	}
	return j.NextAttemptAt
}

// TryMoneybirdImportNow implements ServerInterface.
func (s *server) TryMoneybirdImportNow(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	j, err := s.deps.MoneybirdImport.TryNow(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toMoneybirdImport(j))
}

// ListMoneybirdImportProblems implements ServerInterface.
func (s *server) ListMoneybirdImportProblems(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListMoneybirdImportProblemsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	page, perPage := pageNumbers(params.Page, params.PerPage)
	list, total, err := s.deps.MoneybirdImport.Problems(r.Context(), admin, user(r).ID, page, perPage)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]MoneybirdImportProblem, len(list))
	for i, p := range list {
		out[i] = MoneybirdImportProblem{Step: p.Step, Kind: p.Kind, SourceId: p.SourceID.String(), Message: p.Message, Partial: p.Partial, CreatedAt: p.CreatedAt}
	}
	s.writeLinks(w, r, page, perPage, total)
	writeJSON(w, http.StatusOK, out)
}
