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

// Record implements history.Recorder.
func (l *Log) Record(_ context.Context, events ...history.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Events = append(l.Events, events...)
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
