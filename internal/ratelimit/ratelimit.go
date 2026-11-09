// Package ratelimit throttles requests per client with fixed time windows.
package ratelimit

import (
	"sync"
	"time"
)

// Decision is the outcome of counting one request.
type Decision struct {
	Allowed   bool
	Limit     int
	Remaining int
	Reset     time.Duration
}

type window struct {
	start time.Time
	count int
}

// Limiter counts requests per key. State is per process.
type Limiter struct {
	limit  int
	length time.Duration
	now    func() time.Time

	mu        sync.Mutex
	windows   map[string]*window
	lastSweep time.Time
}

// New returns a Limiter allowing limit requests per key in each period.
func New(limit int, period time.Duration) *Limiter {
	return &Limiter{limit: limit, length: period, now: time.Now, windows: map[string]*window{}}
}

// Allow counts a request for key and reports whether it may proceed.
func (l *Limiter) Allow(key string) Decision {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	w, ok := l.windows[key]
	if !ok || now.Sub(w.start) >= l.length {
		w = &window{start: now}
		l.windows[key] = w
	}
	reset := w.start.Add(l.length).Sub(now)
	if w.count >= l.limit {
		return Decision{Allowed: false, Limit: l.limit, Remaining: 0, Reset: reset}
	}
	w.count++
	return Decision{Allowed: true, Limit: l.limit, Remaining: l.limit - w.count, Reset: reset}
}

// sweep drops expired windows at most once per period, keeping memory bounded
// by the number of clients seen recently.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < l.length {
		return
	}
	l.lastSweep = now
	for key, w := range l.windows {
		if now.Sub(w.start) >= l.length {
			delete(l.windows, key)
		}
	}
}
