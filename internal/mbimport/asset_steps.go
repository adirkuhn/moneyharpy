package mbimport

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/assets"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const kindAsset = "asset"

type mbAsset struct {
	ID            mbID      `json:"id"`
	LedgerAccount *mbID     `json:"ledger_account_id"`
	Name          string    `json:"name"`
	PurchaseDate  string    `json:"purchase_date"`
	PurchaseValue mbDecimal `json:"purchase_value"`
	Plan          *struct {
		Active          bool      `json:"active"`
		LifespanInYears mbDecimal `json:"lifespan_in_years"`
		ResidualValue   mbDecimal `json:"residual_value"`
	} `json:"value_change_plan"`
	Sources []struct {
		DetailID mbID `json:"detail_id"`
	} `json:"sources"`
	ValueChanges []struct {
		Type             string    `json:"type"`
		Date             string    `json:"date"`
		Amount           mbDecimal `json:"amount"`
		Description      *string   `json:"description"`
		ExternallyBooked bool      `json:"externally_booked"`
	} `json:"value_changes"`
	Disposal json.RawMessage `json:"disposal"`
}

// assets brings in the fixed assets with their value changes as Moneybird
// booked them, and the purchase lines or journal rows they came from.
func (r *run) assets(ctx context.Context) error {
	return r.listed(ctx, StepAssets, "assets", nil, kindAsset, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var a mbAsset
		if err := json.Unmarshal(raw, &a); err != nil {
			return 0, err
		}
		in, err := r.assetInput(ctx, a)
		if err != nil {
			return 0, err
		}
		got, err := r.s.targets.Assets.Import(ctx, r.job.AdministrationID, r.job.UserID, in)
		if err != nil {
			return 0, err
		}
		if len(a.Disposal) > 0 && string(a.Disposal) != "null" {
			if err := r.partial(ctx, r.job.Step(StepAssets), kindAsset, got.ID, "came in without its disposal, which is to be added by hand"); err != nil {
				return 0, err
			}
		}
		return a.ID.id(), nil
	})
}

func (r *run) assetInput(ctx context.Context, a mbAsset) (assets.ImportedAsset, error) {
	field := func(name, why string) error {
		var verr resource.ValidationError
		verr.Add(name, why)
		return &verr
	}
	ledger, err := r.mapped(ctx, kindLedgerAccount, ref(a.LedgerAccount))
	if err != nil {
		return assets.ImportedAsset{}, err
	}
	day, err := time.Parse(time.DateOnly, a.PurchaseDate)
	if err != nil {
		return assets.ImportedAsset{}, field("purchase_date", "is not a date")
	}
	value, err := money.ParseAmount(string(a.PurchaseValue))
	if err != nil {
		return assets.ImportedAsset{}, field("purchase_value", "is not an amount")
	}
	name := a.Name
	in := assets.ImportedAsset{
		Origin: resource.Origin{ID: a.ID.id()},
		Input:  assets.Input{Name: &name, LedgerAccountID: ledger, PurchaseDate: &day, PurchaseValue: &value},
	}
	if a.Plan != nil && a.Plan.Active {
		years, err := strconv.ParseFloat(string(a.Plan.LifespanInYears), 64)
		if err != nil {
			return assets.ImportedAsset{}, field("value_change_plan.lifespan_in_years", "is not a number")
		}
		residual, err := money.ParseAmount(string(a.Plan.ResidualValue))
		if err != nil {
			return assets.ImportedAsset{}, field("value_change_plan.residual_value", "is not an amount")
		}
		in.Input.Plan = &assets.PlanInput{LifespanYears: &years, ResidualValue: &residual}
	}
	for _, s := range a.Sources {
		in.Sources = append(in.Sources, s.DetailID.id())
	}
	for i, c := range a.ValueChanges {
		d, err := time.Parse(time.DateOnly, c.Date)
		if err != nil {
			return assets.ImportedAsset{}, field(fmt.Sprintf("value_changes.%d.date", i), "is not a date")
		}
		amount, err := money.ParseAmount(string(c.Amount))
		if err != nil {
			return assets.ImportedAsset{}, field(fmt.Sprintf("value_changes.%d.amount", i), "is not an amount")
		}
		in.ValueChanges = append(in.ValueChanges, assets.ValueChange{Type: c.Type, Date: d, Amount: amount, Description: c.Description, ExternallyBooked: c.ExternallyBooked})
	}
	return in, nil
}
