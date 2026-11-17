package settings

import (
	"cmp"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"regexp"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// BankLink configures bank connections through Enable Banking, which reads
// the transactions of the administrations' own bank accounts under its
// PSD2 licence. Off by default: transactions then pass through Enable
// Banking.
type BankLink struct {
	Enabled bool
	// ApplicationID is the id of the application registered in Enable
	// Banking's control panel.
	ApplicationID string
	// PrivateKey is the PEM private key the application was registered
	// with; it signs the requests.
	PrivateKey string
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func applyBankLink(cfg *BankLink, u Update) {
	if u.BankLinkEnabled != nil {
		cfg.Enabled = *u.BankLinkEnabled
	}
	if u.BankLinkApplicationID != nil {
		cfg.ApplicationID = strings.TrimSpace(*u.BankLinkApplicationID)
	}
	if u.BankLinkPrivateKey != nil {
		cfg.PrivateKey = cmp.Or(strings.TrimSpace(*u.BankLinkPrivateKey), cfg.PrivateKey)
	}
}

func validateBankLink(cfg BankLink, verr *resource.ValidationError) {
	if cfg.PrivateKey != "" {
		if _, err := ParseRSAKey(cfg.PrivateKey); err != nil {
			verr.Add("enable_banking_private_key", "must be the application's RSA private key in PEM")
		}
	}
	if cfg.ApplicationID != "" && !uuidPattern.MatchString(cfg.ApplicationID) {
		verr.Add("enable_banking_application_id", "must be the application id from Enable Banking")
	}
	if !cfg.Enabled {
		return
	}
	if cfg.ApplicationID == "" {
		verr.Add("enable_banking_application_id", "can't be blank while bank connections are enabled")
	}
	if cfg.PrivateKey == "" {
		verr.Add("enable_banking_private_key", "can't be blank while bank connections are enabled")
	}
}

// ParseRSAKey reads an RSA private key in PEM, PKCS #8 or PKCS #1.
func ParseRSAKey(s string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, errNotRSA
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errNotRSA
	}
	return key, nil
}
