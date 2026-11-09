// Package settings holds instance configuration kept in the database, so an
// administrator can change it at runtime. Secrets are encrypted at rest.
package settings

import (
	"cmp"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ErrNotSet reports a key with no stored value.
var ErrNotSet = errors.New("setting not set")

const instanceKey = "instance"

// Google configures sign-in with Google.
type Google struct {
	Enabled      bool
	ClientID     string
	ClientSecret string
}

// Instance is the configuration of this MoneyHarpy installation.
type Instance struct {
	RegistrationOpen bool
	Google           Google
}

// Update changes selected settings. Nil fields stay as they are, and an empty
// secret keeps the stored one, because secrets are never sent back to clients.
type Update struct {
	RegistrationOpen   *bool
	GoogleEnabled      *bool
	GoogleClientID     *string
	GoogleClientSecret *string
}

// Store persists opaque values by key.
type Store interface {
	// Load returns the value for key, or ErrNotSet.
	Load(ctx context.Context, key string) ([]byte, error)
	Save(ctx context.Context, key string, value []byte) error
}

// Service reads and changes instance settings.
type Service struct {
	store  Store
	cipher *Cipher
}

// NewService returns a Service encrypting secrets with c.
func NewService(store Store, c *Cipher) *Service {
	return &Service{store: store, cipher: c}
}

type storedInstance struct {
	RegistrationOpen bool   `json:"registration_open"`
	GoogleEnabled    bool   `json:"google_enabled"`
	GoogleClientID   string `json:"google_client_id"`
	GoogleSecret     string `json:"google_client_secret_sealed"`
}

// Instance returns the current settings with secrets decrypted.
func (s *Service) Instance(ctx context.Context) (Instance, error) {
	raw, err := s.store.Load(ctx, instanceKey)
	if errors.Is(err, ErrNotSet) {
		return Instance{RegistrationOpen: true}, nil
	}
	if err != nil {
		return Instance{}, fmt.Errorf("load settings: %w", err)
	}
	var st storedInstance
	if err := json.Unmarshal(raw, &st); err != nil {
		return Instance{}, fmt.Errorf("decode settings: %w", err)
	}
	inst := Instance{
		RegistrationOpen: st.RegistrationOpen,
		Google:           Google{Enabled: st.GoogleEnabled, ClientID: st.GoogleClientID},
	}
	if st.GoogleSecret != "" {
		if inst.Google.ClientSecret, err = s.cipher.Open(st.GoogleSecret); err != nil {
			return Instance{}, fmt.Errorf("decrypt google secret: %w", err)
		}
	}
	return inst, nil
}

// RegistrationOpen reports whether new users may join.
func (s *Service) RegistrationOpen(ctx context.Context) (bool, error) {
	inst, err := s.Instance(ctx)
	if err != nil {
		return false, err
	}
	return inst.RegistrationOpen, nil
}

// Update applies u and returns the resulting settings.
func (s *Service) Update(ctx context.Context, u Update) (Instance, error) {
	inst, err := s.Instance(ctx)
	if err != nil {
		return Instance{}, err
	}
	if u.RegistrationOpen != nil {
		inst.RegistrationOpen = *u.RegistrationOpen
	}
	if u.GoogleEnabled != nil {
		inst.Google.Enabled = *u.GoogleEnabled
	}
	if u.GoogleClientID != nil {
		inst.Google.ClientID = *u.GoogleClientID
	}
	if u.GoogleClientSecret != nil {
		inst.Google.ClientSecret = cmp.Or(*u.GoogleClientSecret, inst.Google.ClientSecret)
	}

	var verr resource.ValidationError
	if inst.Google.Enabled && inst.Google.ClientID == "" {
		verr.Add("google_client_id", "can't be blank while Google login is enabled")
	}
	if inst.Google.Enabled && inst.Google.ClientSecret == "" {
		verr.Add("google_client_secret", "can't be blank while Google login is enabled")
	}
	if err := verr.OrNil(); err != nil {
		return Instance{}, err
	}

	st := storedInstance{
		RegistrationOpen: inst.RegistrationOpen,
		GoogleEnabled:    inst.Google.Enabled,
		GoogleClientID:   inst.Google.ClientID,
	}
	if inst.Google.ClientSecret != "" {
		if st.GoogleSecret, err = s.cipher.Seal(inst.Google.ClientSecret); err != nil {
			return Instance{}, fmt.Errorf("encrypt google secret: %w", err)
		}
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return Instance{}, fmt.Errorf("encode settings: %w", err)
	}
	if err := s.store.Save(ctx, instanceKey, raw); err != nil {
		return Instance{}, fmt.Errorf("save settings: %w", err)
	}
	return inst, nil
}

// Cipher encrypts values with AES-256-GCM.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher derives an encryption key from secretKey.
func NewCipher(secretKey string) *Cipher {
	key := sha256.Sum256([]byte(secretKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		panic(fmt.Sprintf("create AES cipher: %v", err)) // A 32-byte key cannot fail.
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic(fmt.Sprintf("create GCM: %v", err)) // Standard parameters cannot fail.
	}
	return &Cipher{aead: aead}
}

// Seal encrypts plaintext into a printable string.
func (c *Cipher) Seal(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}
	return base64.StdEncoding.EncodeToString(c.aead.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}

// Open decrypts a value produced by Seal.
func (c *Cipher) Open(sealed string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return "", fmt.Errorf("decode sealed value: %w", err)
	}
	n := c.aead.NonceSize()
	if len(raw) < n {
		return "", errors.New("sealed value too short")
	}
	plain, err := c.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt sealed value: %w", err)
	}
	return string(plain), nil
}
