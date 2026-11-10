package mail

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"io"
	"math/big"
	"mime"
	"mime/quotedprintable"
	"net"
	stdmail "net/mail"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/settings"
)

type staticSettings struct{ inst settings.Instance }

func (s staticSettings) Instance(context.Context) (settings.Instance, error) { return s.inst, nil }

// fakeServer is a minimal SMTP server that records one conversation.
type fakeServer struct {
	listener net.Listener
	tlsCfg   *tls.Config
	offerTLS bool

	mu       sync.Mutex
	rcpt     []string
	from     string
	data     string
	authed   string
	usedTLS  bool
	finished chan struct{}
}

func startServer(t *testing.T, implicitTLS, offerTLS bool, cert tls.Certificate) *fakeServer {
	t.Helper()
	s := &fakeServer{tlsCfg: &tls.Config{Certificates: []tls.Certificate{cert}}, offerTLS: offerTLS, finished: make(chan struct{})}
	var err error
	if implicitTLS {
		s.listener, err = tls.Listen("tcp", "127.0.0.1:0", s.tlsCfg)
	} else {
		s.listener, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	s.usedTLS = implicitTLS
	t.Cleanup(func() { _ = s.listener.Close() })
	go s.serve()
	return s
}

func (s *fakeServer) port() int { return s.listener.Addr().(*net.TCPAddr).Port }

func (s *fakeServer) serve() {
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	defer close(s.finished)
	tp := textproto.NewConn(conn)
	_ = tp.PrintfLine("220 fake ready")
	tlsActive := s.usedTLS
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		verb := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		switch verb {
		case "EHLO", "HELO":
			caps := []string{"250-fake"}
			if s.offerTLS && !tlsActive {
				caps = append(caps, "250-STARTTLS")
			}
			caps = append(caps, "250 AUTH PLAIN")
			_ = tp.PrintfLine("%s", strings.Join(caps, "\r\n"))
		case "STARTTLS":
			_ = tp.PrintfLine("220 go")
			tc := tls.Server(conn, s.tlsCfg)
			if err := tc.Handshake(); err != nil {
				return
			}
			conn, tp = tc, textproto.NewConn(tc)
			tlsActive = true
			s.mu.Lock()
			s.usedTLS = true
			s.mu.Unlock()
		case "AUTH":
			parts := strings.Fields(line)
			raw, _ := base64.StdEncoding.DecodeString(parts[len(parts)-1])
			s.mu.Lock()
			s.authed = string(raw)
			s.mu.Unlock()
			_ = tp.PrintfLine("235 ok")
		case "MAIL":
			s.mu.Lock()
			s.from = line
			s.mu.Unlock()
			_ = tp.PrintfLine("250 ok")
		case "RCPT":
			s.mu.Lock()
			s.rcpt = append(s.rcpt, line)
			s.mu.Unlock()
			_ = tp.PrintfLine("250 ok")
		case "DATA":
			_ = tp.PrintfLine("354 go")
			body, err := tp.ReadDotBytes()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.data = string(body)
			s.mu.Unlock()
			_ = tp.PrintfLine("250 ok")
		case "QUIT":
			_ = tp.PrintfLine("221 bye")
			return
		default:
			_ = tp.PrintfLine("500 unknown")
		}
	}
}

func (s *fakeServer) wait(t *testing.T) {
	t.Helper()
	select {
	case <-s.finished:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not finish")
	}
}

func testCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "127.0.0.1"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(parsed)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

func client(port int, security, user string, pool *x509.CertPool) *Client {
	s := staticSettings{inst: settings.Instance{SMTP: settings.SMTP{
		Enabled: true, Host: "127.0.0.1", Port: port, Username: user, Password: "pw", From: "books@example.com", Security: security,
	}}}
	c := New(s, "https://harpy.test")
	c.rootCAs = pool
	return c
}

func decodeBody(t *testing.T, data string) (*stdmail.Message, string) {
	t.Helper()
	msg, err := stdmail.ReadMessage(bufio.NewReader(strings.NewReader(data)))
	if err != nil {
		t.Fatalf("ReadMessage() error = %v\n%s", err, data)
	}
	body, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return msg, string(body)
}

func TestPasswordResetMail(t *testing.T) {
	cert, _ := testCert(t)
	tests := []struct {
		name, language, subject, phrase string
	}{
		{name: "english", language: "en", subject: "Reset your MoneyHarpy password", phrase: "Choose a new password"},
		{name: "dutch", language: "nl", subject: "Stel je MoneyHarpy-wachtwoord opnieuw in", phrase: "Kies een nieuw wachtwoord"},
		{name: "belgian uses dutch", language: "nl-be", subject: "Stel je MoneyHarpy-wachtwoord opnieuw in", phrase: "Kies een nieuw wachtwoord"},
		{name: "unset uses english", language: "", subject: "Reset your MoneyHarpy password", phrase: "Choose a new password"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := startServer(t, false, false, cert)
			if err := client(srv.port(), "none", "", nil).SendPasswordReset(context.Background(), "ann@example.com", "tok+en/1", tt.language); err != nil {
				t.Fatalf("SendPasswordReset() error = %v", err)
			}
			srv.wait(t)

			msg, body := decodeBody(t, srv.data)
			dec := new(mime.WordDecoder)
			if got, _ := dec.DecodeHeader(msg.Header.Get("Subject")); got != tt.subject {
				t.Errorf("Subject = %q, want %q", got, tt.subject)
			}
			if got := msg.Header.Get("To"); got != "ann@example.com" {
				t.Errorf("To = %q", got)
			}
			if got := msg.Header.Get("From"); got != "books@example.com" {
				t.Errorf("From = %q", got)
			}
			if !strings.Contains(body, tt.phrase) {
				t.Errorf("body = %q, want %q", body, tt.phrase)
			}
			if want := "https://harpy.test/passwords/edit?token=tok%2Ben%2F1"; !strings.Contains(body, want) {
				t.Errorf("body = %q, want link %q", body, want)
			}
			if len(srv.rcpt) != 1 || !strings.Contains(srv.rcpt[0], "ann@example.com") {
				t.Errorf("rcpt = %v", srv.rcpt)
			}
		})
	}
}

func TestStartTLSAuthenticatesOverEncryption(t *testing.T) {
	cert, pool := testCert(t)
	srv := startServer(t, false, true, cert)
	if err := client(srv.port(), "starttls", "user", pool).SendTest(context.Background(), "ann@example.com"); err != nil {
		t.Fatalf("SendTest() error = %v", err)
	}
	srv.wait(t)
	if !srv.usedTLS {
		t.Error("connection was not upgraded to TLS")
	}
	if want := "\x00user\x00pw"; srv.authed != want {
		t.Errorf("auth = %q, want %q", srv.authed, want)
	}
}

func TestImplicitTLS(t *testing.T) {
	cert, pool := testCert(t)
	srv := startServer(t, true, false, cert)
	if err := client(srv.port(), "tls", "user", pool).SendTest(context.Background(), "ann@example.com"); err != nil {
		t.Fatalf("SendTest() error = %v", err)
	}
	srv.wait(t)
	if srv.data == "" {
		t.Error("no message delivered")
	}
}

func TestRefusesToSendCredentialsInClear(t *testing.T) {
	cert, _ := testCert(t)
	t.Run("server does not offer STARTTLS", func(t *testing.T) {
		srv := startServer(t, false, false, cert)
		if err := client(srv.port(), "starttls", "user", nil).SendTest(context.Background(), "a@b.nl"); err == nil {
			t.Error("SendTest() error = nil, want refusal")
		}
		if srv.authed != "" {
			t.Error("credentials were sent")
		}
	})
	t.Run("untrusted certificate", func(t *testing.T) {
		srv := startServer(t, false, true, cert)
		if err := client(srv.port(), "starttls", "user", nil).SendTest(context.Background(), "a@b.nl"); err == nil {
			t.Error("SendTest() error = nil, want certificate failure")
		}
		if srv.authed != "" {
			t.Error("credentials were sent")
		}
	})
	t.Run("security none with a username", func(t *testing.T) {
		srv := startServer(t, false, false, cert)
		err := client(srv.port(), "none", "user", nil).SendTest(context.Background(), "a@b.nl")
		if err == nil {
			t.Error("SendTest() error = nil, want refusal")
		}
		if srv.authed != "" {
			t.Error("credentials were sent")
		}
	})
}

func TestRejectsRecipientWithLineBreak(t *testing.T) {
	err := client(1, "none", "", nil).SendTest(context.Background(), "a@b.nl\r\nBcc: x@y.nl")
	if err == nil {
		t.Error("SendTest() error = nil, want rejection")
	}
}

func TestDisabled(t *testing.T) {
	c := New(staticSettings{}, "https://harpy.test")
	if err := c.SendTest(context.Background(), "a@b.nl"); !errors.Is(err, ErrDisabled) {
		t.Errorf("SendTest() error = %v, want ErrDisabled", err)
	}
}
