// Package hookclient posts webhook deliveries. Unless told otherwise, it only
// connects to public addresses, checked on the address actually dialled, so
// a webhook cannot be aimed at the server's own network.
package hookclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/webhooks"
)

// ErrBlocked reports a private, loopback or otherwise local address.
var ErrBlocked = errors.New("address is not public")

const (
	timeout  = 10 * time.Second
	maxBody  = 1 << 10
	maxDrain = 64 << 10
)

// Client implements webhooks.Sender.
type Client struct {
	public, local *http.Client
}

// New returns a Client.
func New() *Client {
	return &Client{public: client(true), local: client(false)}
}

func client(guard bool) *http.Client {
	dialer := &net.Dialer{Timeout: timeout}
	if guard {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip, err := netip.ParseAddr(host)
			if err != nil {
				return err
			}
			if !Public(ip) {
				return fmt.Errorf("%w: %s", ErrBlocked, ip)
			}
			return nil
		}
	}
	transport := &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: timeout, ResponseHeaderTimeout: timeout, MaxIdleConns: 20, IdleConnTimeout: time.Minute}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		// A redirect could lead anywhere; the receiver must answer itself.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// Public reports whether ip is a public unicast address.
func Public(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, p := range blocked {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// blocked are special-use ranges that IsPrivate and friends do not cover.
var blocked = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// Post implements webhooks.Sender.
func (c *Client) Post(ctx context.Context, url string, headers map[string]string, body []byte, allowLocal bool) (webhooks.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return webhooks.Response{}, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	hc := c.public
	if allowLocal {
		hc = c.local
	}
	res, err := hc.Do(req)
	if err != nil {
		if errors.Is(err, ErrBlocked) {
			return webhooks.Response{}, ErrBlocked
		}
		return webhooks.Response{}, err
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(io.LimitReader(res.Body, maxBody))
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, maxDrain)) // Lets the connection be reused.
	return webhooks.Response{Status: res.StatusCode, Body: string(b)}, nil
}
