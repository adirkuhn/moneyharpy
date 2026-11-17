package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/history"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const defaultEventsPerPage = 50

// ListEvents implements ServerInterface: Moneybird's list of events, newest
// first, with a cursor to the older ones in the Link header.
func (s *server) ListEvents(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListEventsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	var entityID *resource.ID
	if params.EntityId != nil {
		id, err := resource.ParseID(*params.EntityId)
		if err != nil {
			writeError(w, http.StatusBadRequest, "entity_id is not a valid id")
			return
		}
		entityID = &id
	}
	var after *history.Cursor
	if params.Cursor != nil {
		c, err := history.ParseCursor(*params.Cursor)
		if err != nil {
			writeError(w, http.StatusBadRequest, "cursor is not one this list made; follow the Link header")
			return
		}
		after = &c
	}
	ctx, me := r.Context(), user(r).ID
	listed, err := s.deps.Administrations.Get(ctx, admin, me)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// A connection sees what both its scopes and its user allow.
	grant, granted := grantFrom(ctx)
	can := func(perms ...administration.Permission) bool {
		return listed.Can(perms...) && (!granted || grant.Allows(perms...))
	}
	var perms []administration.Permission
	for _, p := range listed.Permissions {
		if can(p) {
			perms = append(perms, p)
		}
	}
	q, err := history.Scope(deref(params.EntityType), entityID, me, perms, can)
	var unknown *history.UnknownTypeError
	switch {
	case errors.Is(err, history.ErrForbidden):
		writeError(w, http.StatusForbidden, "Your user has no permission for the events of this record type")
		return
	case errors.As(err, &unknown):
		writeError(w, http.StatusBadRequest, fmt.Sprintf("entity_type %q has no events; ask for one of %s", deref(params.EntityType), strings.Join(unknown.Allowed, ", ")))
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_, perPage := pageNumbers(nil, params.PerPage)
	if perPage < 1 {
		perPage = defaultEventsPerPage
	}
	q.Limit, q.After = perPage, after
	events, next, err := s.deps.Events.List(ctx, admin, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if next != nil {
		v := r.URL.Query()
		v.Set("cursor", next.String())
		v.Set("per_page", fmt.Sprint(perPage))
		u := url.URL{Path: r.URL.Path, RawQuery: v.Encode()}
		w.Header().Set("Link", fmt.Sprintf("<%s%s>; rel=%q", strings.TrimSuffix(s.deps.BaseURL, "/"), u.String(), "next"))
	}
	writeJSON(w, http.StatusOK, toEvents(events))
}
