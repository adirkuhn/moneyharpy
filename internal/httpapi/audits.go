package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/audits"
)

type auditJSON struct {
	UserID    string    `json:"user_id"`
	AuditedAt time.Time `json:"audited_at"`
	Debit     string    `json:"debit"`
	Credit    string    `json:"credit"`
}

type auditLineJSON struct {
	LedgerAccountID string     `json:"ledger_account_id"`
	Debit           string     `json:"debit"`
	Credit          string     `json:"credit"`
	TaxRateIDs      []string   `json:"tax_rate_ids"`
	Audit           *auditJSON `json:"audit"`
	Changed         bool       `json:"changed"`
}

func toAuditLine(l audits.Line) auditLineJSON {
	out := auditLineJSON{LedgerAccountID: l.LedgerAccountID.String(), Debit: l.Debit.String(), Credit: l.Credit.String(), TaxRateIDs: []string{}, Changed: l.Changed}
	for _, id := range l.TaxRateIDs {
		out.TaxRateIDs = append(out.TaxRateIDs, id.String())
	}
	if a := l.Audit; a != nil {
		out.Audit = &auditJSON{UserID: a.UserID.String(), AuditedAt: a.AuditedAt, Debit: a.Debit.String(), Credit: a.Credit.String()}
	}
	return out
}

func (s *server) failAudit(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, audits.ErrInvalid):
		writeError(w, http.StatusBadRequest, "Period is invalid")
	case errors.Is(err, audits.ErrNotFound):
		writeError(w, http.StatusNotFound, "Record not found for model name: Category")
	default:
		s.fail(w, r, err)
	}
}

// GetAuditReport implements ServerInterface.
func (s *server) GetAuditReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetAuditReportParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	report, err := s.deps.Audits.Report(r.Context(), admin, user(r).ID, deref((*string)(params.Period)))
	if err != nil {
		s.failAudit(w, r, err)
		return
	}
	type reportJSON struct {
		PeriodFrom     string          `json:"period_from"`
		PeriodTo       string          `json:"period_to"`
		LedgerAccounts []auditLineJSON `json:"ledger_accounts"`
	}
	out := reportJSON{PeriodFrom: report.Period.From.Format(time.DateOnly), PeriodTo: report.Period.To.Format(time.DateOnly), LedgerAccounts: []auditLineJSON{}}
	for _, l := range report.Lines {
		out.LedgerAccounts = append(out.LedgerAccounts, toAuditLine(l))
	}
	writeJSON(w, http.StatusOK, out)
}

// AuditLedgerAccount implements ServerInterface.
func (s *server) AuditLedgerAccount(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, ledgerAccountID Identifier, params AuditLedgerAccountParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	ledger, ok := parseID(w, ledgerAccountID)
	if !ok {
		return
	}
	line, err := s.deps.Audits.Mark(r.Context(), admin, user(r).ID, ledger, deref((*string)(params.Period)))
	if err != nil {
		s.failAudit(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAuditLine(line))
}

// UnauditLedgerAccount implements ServerInterface.
func (s *server) UnauditLedgerAccount(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, ledgerAccountID Identifier, params UnauditLedgerAccountParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	ledger, ok := parseID(w, ledgerAccountID)
	if !ok {
		return
	}
	if err := s.deps.Audits.Unmark(r.Context(), admin, user(r).ID, ledger, deref((*string)(params.Period))); err != nil {
		s.failAudit(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
