package mbimport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// vatRoles are the categories Moneybird's VAT returns book on.
var vatRoles = []string{"payable_vat", "receivable_vat", "vat_paid_or_received"}

// vat has the VAT categories come to what Moneybird has at the end of every
// month. Moneybird's VAT returns move the VAT charged and paid to what is
// to be paid to or got back from the tax authority; its API does not give
// them, so the month they did gets a correction of the difference.
func (r *run) vat(ctx context.Context) error {
	st := r.job.Step(StepVAT)
	ours := map[resource.ID]bool{}
	for _, role := range vatRoles {
		id, err := r.s.targets.Books.SystemLedgerAccount(ctx, r.job.AdministrationID, r.job.UserID, role)
		if err != nil {
			return err
		}
		if id != 0 {
			ours[id] = true
		}
	}
	if len(ours) == 0 {
		return nil
	}
	first, ok, err := r.s.targets.Reports.FirstDay(ctx, r.job.AdministrationID, r.job.UserID)
	if err != nil || !ok {
		return err
	}
	now := r.s.now()
	for m := time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, time.UTC); !m.After(now); m = m.AddDate(0, 1, 0) {
		st.Total++
		if err := r.vatMonth(ctx, st, m, ours); err != nil {
			return err
		}
		if err := r.save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *run) vatMonth(ctx context.Context, st *Step, month time.Time, ours map[resource.ID]bool) error {
	end := month.AddDate(0, 1, -1)
	raw, err := r.read.Get(ctx, r.job.SourceID, "reports/balance_sheet.json", url.Values{"period": {month.Format("200601")}})
	if err != nil {
		return err
	}
	theirs, any, err := r.vatBalances(ctx, raw, ours)
	if err != nil {
		return err
	}
	if !any {
		// Moneybird gave no balance sheet for the month.
		st.Skipped++
		return nil
	}
	here, err := r.s.targets.Reports.Balances(ctx, r.job.AdministrationID, r.job.UserID, end)
	if err != nil {
		return err
	}
	var rows []journal.RowInput
	var off money.Amount
	for id := range ours {
		diff := theirs[id] - here[id]
		if diff == 0 {
			continue
		}
		off += diff
		row := journal.RowInput{LedgerAccountID: &id, Description: ptrTo("VAT as in Moneybird")}
		if text := money.Amount(abs(int64(diff))).String(); diff > 0 {
			row.Debit = &text
		} else {
			row.Credit = &text
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		st.Skipped++
		return nil
	}
	if off != 0 {
		return r.fail(ctx, st, kindVAT, resource.ID(month.Year()*100+int(month.Month())),
			fmt.Errorf("VAT at %s differs from Moneybird by %s that its returns do not account for", end.Format(time.DateOnly), off))
	}
	day := end.Format(time.DateOnly)
	reference := "VAT as in Moneybird"
	if _, err := r.s.targets.Journal.ImportCorrection(ctx, r.job.AdministrationID, r.job.UserID, journal.DocumentChanges{Reference: &reference, Date: &day, Rows: rows}); err != nil {
		if isRecordProblem(err) {
			return r.fail(ctx, st, kindVAT, resource.ID(month.Year()*100+int(month.Month())), err)
		}
		return err
	}
	st.Imported++
	return nil
}

const kindVAT = "vat_balance"

// vatBalances reads the balances of the VAT categories here from
// Moneybird's balance sheet, debits positive; any tells whether it had
// categories at all.
func (r *run) vatBalances(ctx context.Context, raw json.RawMessage, ours map[resource.ID]bool) (map[resource.ID]money.Amount, bool, error) {
	var sheet map[string]map[string][]mbReportLine
	if err := json.Unmarshal(raw, &sheet); err != nil {
		return nil, false, nil
	}
	out := map[resource.ID]money.Amount{}
	any := false
	var walk func(side string, lines []mbReportLine) error
	walk = func(side string, lines []mbReportLine) error {
		for _, l := range lines {
			any = true
			if err := walk(side, l.Children); err != nil {
				return err
			}
			here, err := r.mapped(ctx, kindLedgerAccount, ref(&l.LedgerAccountID))
			if err != nil || here == nil || !ours[*here] {
				if err != nil {
					return err
				}
				continue
			}
			value, err := money.ParseAmount(string(l.Value))
			if err != nil {
				continue
			}
			if side == "credit" {
				value = -value
			}
			out[*here] += value
		}
		return nil
	}
	for side, sections := range sheet {
		for _, lines := range sections {
			if err := walk(side, lines); err != nil {
				return nil, false, err
			}
		}
	}
	return out, any, nil
}

type mbReportLine struct {
	LedgerAccountID mbID           `json:"ledger_account_id"`
	Value           mbDecimal      `json:"value"`
	Children        []mbReportLine `json:"children"`
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
