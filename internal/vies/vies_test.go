package vies

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/recognize"
)

func serve(t *testing.T, h http.HandlerFunc) (*Client, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL+"/", srv.Client()), &calls
}

func TestLookupSendsOnlyTheVATNumberAndReadsTheCompany(t *testing.T) {
	c, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/ms/NL/vat/123456782B01" || r.URL.RawQuery != "" {
			t.Errorf("request = %s %s", r.Method, r.URL)
		}
		if b, _ := io.ReadAll(r.Body); len(b) != 0 {
			t.Errorf("body = %q", b)
		}
		_, _ = io.WriteString(w, `{"isValid":true,"requestDate":"2026-11-14T10:00:00.000Z","userError":"VALID","name":"EXAMPLE SHOP B.V.","address":"EXAMPLE STREET 00001\n1234AB AMSTERDAM \n","vatNumber":"123456782B01"}`)
	})
	got, err := c.LookupVAT(context.Background(), "nl 1234.56.782.B01")
	want := recognize.RegisteredCompany{Valid: true, Name: "EXAMPLE SHOP B.V.", Address: "EXAMPLE STREET 00001\n1234AB AMSTERDAM \n"}
	if err != nil || got != want {
		t.Errorf("LookupVAT() = %+v, %v, want %+v", got, err, want)
	}
}

func TestLookupLeavesOutWhatTheRegisterDoesNotTell(t *testing.T) {
	c, _ := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"isValid":true,"userError":"VALID","name":"---","address":"---"}`)
	})
	got, err := c.LookupVAT(context.Background(), "DE123456789")
	if err != nil || got != (recognize.RegisteredCompany{Valid: true}) {
		t.Errorf("LookupVAT() = %+v, %v", got, err)
	}
}

func TestLookupKeepsAnswersAndNotErrors(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	c, calls := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"isValid":false,"userError":"INVALID","name":"---","address":"---"}`)
	})
	ctx := context.Background()
	if _, err := c.LookupVAT(ctx, "NL123456782B01"); err == nil {
		t.Error("an unavailable service gave no error")
	}
	fail.Store(false)
	for range 3 {
		if got, err := c.LookupVAT(ctx, "NL123456782B01"); err != nil || got.Valid {
			t.Errorf("LookupVAT() = %+v, %v", got, err)
		}
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("asked VIES %d times, want 2: the error is not kept, the answer is", n)
	}
	c.now = func() time.Time { return time.Now().Add(cacheFor + time.Minute) }
	_, _ = c.LookupVAT(ctx, "NL123456782B01")
	if n := calls.Load(); n != 3 {
		t.Errorf("asked VIES %d times, want 3 once the answer is old", n)
	}
}

func TestLookupErrorsOfTheService(t *testing.T) {
	for name, body := range map[string]string{
		"user error":  `{"isValid":false,"userError":"MS_UNAVAILABLE"}`,
		"not JSON":    `<html>`,
		"rate limits": `{"isValid":false,"userError":"MS_MAX_CONCURRENT_REQ"}`,
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := serve(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) })
			if _, err := c.LookupVAT(context.Background(), "NL123456782B01"); err == nil {
				t.Error("no error")
			}
		})
	}
}

func TestLookupRefusesNumbersOutsideVIES(t *testing.T) {
	c, calls := serve(t, func(http.ResponseWriter, *http.Request) {})
	for _, vat := range []string{"GB123456789", "NL", "", "NL12/../x"} {
		if _, err := c.LookupVAT(context.Background(), vat); err == nil {
			t.Errorf("LookupVAT(%q) gave no error", vat)
		}
	}
	if calls.Load() != 0 {
		t.Error("VIES was asked")
	}
}

func TestLookupTimesOut(t *testing.T) {
	release := make(chan struct{})
	c, _ := serve(t, func(http.ResponseWriter, *http.Request) { <-release })
	defer close(release)
	c.timeout = 50 * time.Millisecond
	start := time.Now()
	if _, err := c.LookupVAT(context.Background(), "NL123456782B01"); err == nil {
		t.Error("no error")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v", d)
	}
}
