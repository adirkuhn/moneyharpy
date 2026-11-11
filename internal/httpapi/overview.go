package httpapi

import (
	"net/http"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/overview"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// GetOverview implements ServerInterface.
func (s *server) GetOverview(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	var scopes []administration.Permission
	if g, ok := grantFrom(r.Context()); ok {
		scopes = append([]administration.Permission{}, g.Scopes...)
	}
	o, err := s.deps.Overview.Get(r.Context(), admin, user(r).ID, scopes)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toOverview(o))
}

func toOverview(o overview.Overview) Overview {
	out := Overview{RecentlyProcessed: make([]OverviewItem, len(o.RecentlyProcessed))}
	if o.Bank != nil {
		out.Bank = &OverviewBank{Balance: o.Bank.Balance.String(), Currency: o.Bank.Currency, UnprocessedCount: o.Bank.Unprocessed}
	}
	if o.Sales != nil {
		out.Sales = &OverviewSales{
			ToReceive: OverviewTally{Count: o.Sales.ToReceive.Count, Amount: o.Sales.ToReceive.Amount.String()},
			ToRemind:  OverviewTally{Count: o.Sales.ToRemind.Count, Amount: o.Sales.ToRemind.Amount.String()},
			Currency:  o.Sales.Currency,
		}
	}
	if o.Incoming != nil {
		out.Incoming = &OverviewIncoming{
			ToPay:     OverviewTally{Count: o.Incoming.ToPay.Count, Amount: o.Incoming.ToPay.Amount.String()},
			ToProcess: o.Incoming.ToProcess, Currency: o.Incoming.Currency,
		}
	}
	for i, it := range o.RecentlyProcessed {
		out.RecentlyProcessed[i] = OverviewItem{
			Type: OverviewItemType(it.Type), Id: it.ID.String(), Uri: it.Ref.URI(), Title: it.Title, Description: it.Description,
			Amount: it.Amount, Currency: it.Currency, Date: openapi_types.Date{Time: it.Date}, State: it.State, ProcessedAt: it.ProcessedAt,
		}
	}
	return out
}
