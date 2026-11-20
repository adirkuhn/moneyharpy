package mbclient

import "time"

// limiter counts requests in a rolling window. Moneybird counts its limit
// per window too, so staying under it here means never meeting a 429 when
// MoneyHarpy is the only client using the token.
type limiter struct {
	limit  int
	window time.Duration
	// sent holds the times of the requests still inside the window, oldest
	// first.
	sent []time.Time
}

// delay says how long to wait at now before another request fits in the
// window; zero when it fits right away.
func (l *limiter) delay(now time.Time) time.Duration {
	l.prune(now)
	if l.limit <= 0 || len(l.sent) < l.limit {
		return 0
	}
	// A slot frees when the request that many places back leaves the window.
	return l.sent[len(l.sent)-l.limit].Add(l.window).Sub(now)
}

// record notes a request sent at now.
func (l *limiter) record(now time.Time) {
	l.sent = append(l.sent, now)
	l.prune(now)
}

// prune forgets requests that left the window, and any beyond the limit
// that can no longer decide a delay, so memory stays bounded.
func (l *limiter) prune(now time.Time) {
	cut := 0
	for cut < len(l.sent) && !l.sent[cut].Add(l.window).After(now) {
		cut++
	}
	if extra := len(l.sent) - cut - l.limit; l.limit > 0 && extra > 0 {
		cut += extra
	}
	if cut > 0 {
		l.sent = append(l.sent[:0], l.sent[cut:]...)
	}
}
