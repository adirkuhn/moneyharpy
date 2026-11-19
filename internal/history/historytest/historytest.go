// Package historytest keeps recorded events in memory, for the tests of
// the services that record them.
package historytest

import (
	"context"
	"sync"

	"github.com/adirkuhn/moneyharpy/internal/history"
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
			e.UserID = 0
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
