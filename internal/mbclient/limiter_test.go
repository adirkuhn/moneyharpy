package mbclient

import (
	"testing"
	"time"
)

func TestLimiterLetsTheLimitThroughWithoutWaiting(t *testing.T) {
	l := limiter{limit: 3, window: time.Minute}
	now := time.Unix(1_800_000_000, 0)
	for i := range 3 {
		if d := l.delay(now); d != 0 {
			t.Fatalf("request %d: delay = %v, want none", i+1, d)
		}
		l.record(now)
	}
	if d := l.delay(now); d != time.Minute {
		t.Errorf("fourth request: delay = %v, want a full window", d)
	}
}

func TestLimiterFreesSlotsAsTheWindowRolls(t *testing.T) {
	l := limiter{limit: 2, window: time.Minute}
	t0 := time.Unix(1_800_000_000, 0)
	l.record(t0)
	l.record(t0.Add(20 * time.Second))
	// The oldest request leaves the window 60s after it was sent.
	if d := l.delay(t0.Add(30 * time.Second)); d != 30*time.Second {
		t.Errorf("delay = %v, want 30s", d)
	}
	if d := l.delay(t0.Add(time.Minute)); d != 0 {
		t.Errorf("delay once the oldest left = %v, want none", d)
	}
	l.record(t0.Add(time.Minute))
	if d := l.delay(t0.Add(time.Minute)); d != 20*time.Second {
		t.Errorf("delay after refill = %v, want 20s", d)
	}
	if len(l.sent) > 2 {
		t.Errorf("limiter kept %d timestamps, want at most its limit", len(l.sent))
	}
}
