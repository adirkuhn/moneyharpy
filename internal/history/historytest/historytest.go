// Package historytest keeps recorded events in memory, for the tests of
// the services that record them.
package historytest

import (
	"context"
	"slices"
	"sync"

	"github.com/adirkuhn/moneyharpy/internal/history"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Log is a history.Recorder that keeps what it is given.
type Log struct {
	mu     sync.Mutex
	Events []history.Event
}

// Record implements history.Recorder; as history.Service, it keeps the
// application's own work without a user.
func (l *Log) Record(ctx context.Context, events ...history.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range events {
		if history.IsSystem(ctx) {
			e.UserID, e.System = 0, true
		}
		l.Events = append(l.Events, e)
	}
	return nil
}

// Actions lists the actions recorded, in order.
func (l *Log) Actions() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.Events))
	for i, e := range l.Events {
		out[i] = e.Action
	}
	return out
}

// Of returns the events kept for each record, in the order recorded, as
// history.Service.Of does.
func (l *Log) Of(_ context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]history.Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := map[resource.ID][]history.Event{}
	for _, e := range l.Events {
		if e.AdministrationID == admin && e.EntityType == entityType && slices.Contains(entities, e.EntityID) {
			out[e.EntityID] = append(out[e.EntityID], e)
		}
	}
	return out, nil
}
