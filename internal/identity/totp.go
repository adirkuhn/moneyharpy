package identity

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // RFC 6238 and every authenticator app use HMAC-SHA1.
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"time"
)

// TOTP after RFC 6238: six digits, a new code every 30 seconds.
const (
	totpPeriod = 30
	totpDigits = 6
	// totpSkew accepts the codes of one step before and after, for clocks
	// that drift.
	totpSkew = 1
)

var base32NoPad = base32.StdEncoding.WithPadding(base32.NoPadding)

// totpStep is the 30-second step t falls in.
func totpStep(t time.Time) int64 { return t.Unix() / totpPeriod }

// totpCode is the code of a step for a secret.
func totpCode(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step)) //nolint:gosec // Steps are positive.
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1_000_000)
}

// matchTOTP returns the step a code belongs to near now, or false.
func matchTOTP(secret []byte, code string, now time.Time) (int64, bool) {
	if len(code) != totpDigits {
		return 0, false
	}
	step := totpStep(now)
	for d := int64(-totpSkew); d <= totpSkew; d++ {
		if hmac.Equal([]byte(totpCode(secret, step+d)), []byte(code)) {
			return step + d, true
		}
	}
	return 0, false
}

// otpauthURI is what authenticator apps read from the QR code.
func otpauthURI(issuer, account string, secret []byte) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{"secret": {base32NoPad.EncodeToString(secret)}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + q.Encode()
}
