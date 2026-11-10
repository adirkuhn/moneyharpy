package hookclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestPublic(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8": true, "2606:4700::1111": true,
		"127.0.0.1": false, "10.1.2.3": false, "172.16.0.1": false, "192.168.1.1": false, "169.254.169.254": false,
		"100.64.0.1": false, "0.0.0.0": false, "::1": false, "fc00::1": false, "fe80::1": false, "::ffff:127.0.0.1": false, "224.0.0.1": false,
	} {
		if got := Public(netip.MustParseAddr(addr)); got != want {
			t.Errorf("Public(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestPostBlocksLocalUnlessAllowed(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = r.Header.Get("Moneybird-Signature") + " " + string(b)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(strings.Repeat("x", 2000)))
	}))
	defer srv.Close()
	c := New()
	if _, err := c.Post(context.Background(), srv.URL, nil, []byte("{}"), false); !errors.Is(err, ErrBlocked) {
		t.Errorf("loopback error = %v, want ErrBlocked", err)
	}
	r, err := c.Post(context.Background(), srv.URL, map[string]string{"Moneybird-Signature": "t=1,v1=x"}, []byte(`{"a":1}`), true)
	if err != nil || r.Status != 201 || len(r.Body) != maxBody || got != `t=1,v1=x {"a":1}` {
		t.Errorf("Post() = %d %d, %v; server got %q", r.Status, len(r.Body), err, got)
	}
}

func TestPostDoesNotFollowRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	r, err := New().Post(context.Background(), srv.URL, nil, nil, true)
	if err != nil || r.Status != http.StatusTemporaryRedirect {
		t.Errorf("Post() = %+v, %v", r, err)
	}
}
