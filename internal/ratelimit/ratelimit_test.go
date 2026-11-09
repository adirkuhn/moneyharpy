package ratelimit

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestLimiterAllowsUpToLimitThenResets(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	l := New(2, time.Minute)
	l.now = func() time.Time { return now }

	steps := []struct {
		name string
		want Decision
	}{
		{name: "first", want: Decision{Allowed: true, Limit: 2, Remaining: 1, Reset: time.Minute}},
		{name: "second", want: Decision{Allowed: true, Limit: 2, Remaining: 0, Reset: time.Minute}},
		{name: "third", want: Decision{Allowed: false, Limit: 2, Remaining: 0, Reset: time.Minute}},
	}
	for _, s := range steps {
		if diff := cmp.Diff(s.want, l.Allow("ip")); diff != "" {
			t.Errorf("%s Allow() diff (-want +got):\n%s", s.name, diff)
		}
	}

	if got := l.Allow("other"); !got.Allowed {
		t.Errorf("Allow(other) = %+v, want allowed, keys are independent", got)
	}

	now = now.Add(time.Minute)
	if got := l.Allow("ip"); !got.Allowed || got.Remaining != 1 {
		t.Errorf("Allow() after window = %+v, want allowed with 1 remaining", got)
	}
}

func TestLimiterForgetsExpiredKeys(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	l := New(1, time.Minute)
	l.now = func() time.Time { return now }
	l.Allow("a")
	l.Allow("b")

	now = now.Add(2 * time.Minute)
	l.Allow("c")
	if got := len(l.windows); got != 1 {
		t.Errorf("tracked keys = %d, want 1 after expired keys are swept", got)
	}
}
