package identity

import (
	"strings"
	"testing"
	"time"
)

// The SHA-1 test vectors of RFC 6238, appendix B, cut to six digits.
func TestTOTPMatchesRFC6238(t *testing.T) {
	secret := []byte("12345678901234567890")
	for unix, want := range map[int64]string{59: "287082", 1111111109: "081804", 1111111111: "050471", 1234567890: "005924", 2000000000: "279037", 20000000000: "353130"} {
		if got := totpCode(secret, totpStep(time.Unix(unix, 0))); got != want {
			t.Errorf("code at %d = %s, want %s", unix, got, want)
		}
	}
}

func TestMatchTOTPAllowsOneStepOfDrift(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1111111109, 0)
	for d, want := range map[time.Duration]bool{0: true, -30 * time.Second: true, 30 * time.Second: true, 90 * time.Second: false} {
		code := totpCode(secret, totpStep(now.Add(d)))
		if _, ok := matchTOTP(secret, code, now); ok != want {
			t.Errorf("drift %v matched = %v", d, ok)
		}
	}
	if _, ok := matchTOTP(secret, "12345", now); ok {
		t.Error("matched a short code")
	}
}

func TestOtpauthURI(t *testing.T) {
	uri := otpauthURI("MoneyHarpy", "ann@example.com", []byte("12345678901234567890"))
	if !strings.HasPrefix(uri, "otpauth://totp/MoneyHarpy:ann@example.com?") || !strings.Contains(uri, "secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ") || !strings.Contains(uri, "issuer=MoneyHarpy") {
		t.Errorf("uri = %s", uri)
	}
}
