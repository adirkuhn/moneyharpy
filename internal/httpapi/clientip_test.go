package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name      string
		trust     bool
		forwarded string
		want      string
	}{
		{name: "ignores header by default", forwarded: "1.1.1.1", want: "10.0.0.9"},
		{name: "trusted proxy", trust: true, forwarded: "1.1.1.1", want: "1.1.1.1"},
		{name: "spoofed entries before the proxy's own are ignored", trust: true, forwarded: "6.6.6.6, 7.7.7.7, 1.1.1.1", want: "1.1.1.1"},
		{name: "trims whitespace", trust: true, forwarded: "6.6.6.6,   1.1.1.1 ", want: "1.1.1.1"},
		{name: "falls back without header", trust: true, want: "10.0.0.9"},
		{name: "falls back on garbage", trust: true, forwarded: "not-an-ip", want: "10.0.0.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = "10.0.0.9:5555"
			if tt.forwarded != "" {
				r.Header.Set("X-Forwarded-For", tt.forwarded)
			}
			if got := clientIP(r, tt.trust); got != tt.want {
				t.Errorf("clientIP(%q, %v) = %q, want %q", tt.forwarded, tt.trust, got, tt.want)
			}
		})
	}
}
