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
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"

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

// LM2 configures sign-in through an Akantio LM2 installation.
type LM2 struct {
	Enabled bool
	// URL is the public address of the LM2 installation.
	URL string
	// App is the name this installation is allowlisted under in LM2.
	App string
}

// KVK configures company search in the Dutch Chamber of Commerce register.
type KVK struct {
	Enabled bool
	APIKey  string
	// WebsiteSearch is an experimental, unofficial search through the public
	// search of the KVK website, used while the API above is not enabled. It
	// needs no key but may stop working or be blocked at any time.
	WebsiteSearch bool
}

// SMTP configures the mail server used for outgoing email.
type SMTP struct {
	Enabled  bool
	Host     string
	Port     int
	Username string
	Password string
	From     string
	// Security is "starttls", "tls" or "none".
	Security string
}

// Storage configures the S3-compatible object storage that holds uploaded
// files, such as logos.
type Storage struct {
	Enabled bool
	// Endpoint is the service address, such as https://s3.eu-west-1.amazonaws.com
	// or http://minio:9000. It has no path.
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	// PathStyle addresses buckets as endpoint/bucket rather than
	// bucket.endpoint, which most self-hosted services need.
	PathStyle bool
}

// Recognition configures the optional AI model Scan and Recognise asks
// about documents its rules could not read. Off by default; with an API
// provider documents are sent to that provider.
type Recognition struct {
	AIEnabled bool
	// Provider is "ollama" or "anthropic".
	Provider string
	// URL is the address of the Ollama server; empty for the provider's own.
	URL    string
	Model  string
	APIKey string
}

// RecognitionProviders are the AI providers Scan and Recognise can ask.
var RecognitionProviders = []string{"ollama", "anthropic"}

// Instance is the configuration of this MoneyHarpy installation.
type Instance struct {
	RegistrationOpen bool
	Google           Google
	LM2              LM2
	KVK              KVK
	// AddressLookupEnabled allows sending zipcodes and house numbers to the
	// Dutch government address service to complete addresses.
	AddressLookupEnabled bool
	SMTP                 SMTP
	Storage              Storage
	// WebhooksAllowLocal lets webhooks reach plain HTTP and private or
	// loopback addresses, for development. Off, webhooks need public HTTPS.
	WebhooksAllowLocal bool
	Recognition        Recognition
}

// Update changes selected settings. Nil fields stay as they are, and an empty
// secret keeps the stored one, because secrets are never sent back to clients.
type Update struct {
	RegistrationOpen   *bool
	GoogleEnabled      *bool
	GoogleClientID     *string
	GoogleClientSecret *string
	LM2Enabled         *bool
	LM2URL             *string
	LM2App             *string

	KVKEnabled              *bool
	KVKAPIKey               *string
	KVKWebsiteSearchEnabled *bool
	AddressLookupEnabled    *bool

	SMTPEnabled  *bool
	SMTPHost     *string
	SMTPPort     *int
	SMTPUsername *string
	SMTPPassword *string
	SMTPFrom     *string
	SMTPSecurity *string

	StorageEnabled   *bool
	StorageEndpoint  *string
	StorageRegion    *string
	StorageBucket    *string
	StorageAccessKey *string
	StorageSecretKey *string
	StoragePathStyle *bool

	WebhooksAllowLocal *bool

	RecognitionAIEnabled  *bool
	RecognitionAIProvider *string
	RecognitionAIURL      *string
	RecognitionAIModel    *string
	RecognitionAIAPIKey   *string
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
	LM2Enabled       bool   `json:"lm2_enabled"`
	LM2URL           string `json:"lm2_url"`
	LM2App           string `json:"lm2_app"`
	KVKEnabled       bool   `json:"kvk_enabled"`
	KVKKey           string `json:"kvk_api_key_sealed"`
	KVKWebsite       bool   `json:"kvk_website_search_enabled"`
	AddressLookup    bool   `json:"address_lookup_enabled"`
	SMTPEnabled      bool   `json:"smtp_enabled"`
	SMTPHost         string `json:"smtp_host"`
	SMTPPort         int    `json:"smtp_port"`
	SMTPUsername     string `json:"smtp_username"`
	SMTPPassword     string `json:"smtp_password_sealed"`
	SMTPFrom         string `json:"smtp_from"`
	SMTPSecurity     string `json:"smtp_security"`
	StorageEnabled   bool   `json:"storage_enabled"`
	StorageEndpoint  string `json:"storage_endpoint"`
	StorageRegion    string `json:"storage_region"`
	StorageBucket    string `json:"storage_bucket"`
	StorageAccessKey string `json:"storage_access_key"`
	StorageSecret    string `json:"storage_secret_key_sealed"`
	StoragePathStyle bool   `json:"storage_path_style"`
	WebhooksLocal    bool   `json:"webhooks_allow_local"`
	RecognitionAI    bool   `json:"recognition_ai_enabled"`
	RecognitionProv  string `json:"recognition_ai_provider"`
	RecognitionURL   string `json:"recognition_ai_url"`
	RecognitionModel string `json:"recognition_ai_model"`
	RecognitionKey   string `json:"recognition_ai_api_key_sealed"`
}

// Instance returns the current settings with secrets decrypted.
func (s *Service) Instance(ctx context.Context) (Instance, error) {
	raw, err := s.store.Load(ctx, instanceKey)
	if errors.Is(err, ErrNotSet) {
		return Instance{RegistrationOpen: true, Recognition: Recognition{Provider: RecognitionProviders[0]}}, nil
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
		LM2:              LM2{Enabled: st.LM2Enabled, URL: st.LM2URL, App: st.LM2App},
		KVK:              KVK{Enabled: st.KVKEnabled, WebsiteSearch: st.KVKWebsite},

		AddressLookupEnabled: st.AddressLookup,
		WebhooksAllowLocal:   st.WebhooksLocal,
		Recognition: Recognition{
			AIEnabled: st.RecognitionAI, Provider: cmp.Or(st.RecognitionProv, RecognitionProviders[0]), URL: st.RecognitionURL,
			Model: st.RecognitionModel,
		},
		SMTP: SMTP{
			Enabled: st.SMTPEnabled, Host: st.SMTPHost, Port: st.SMTPPort, Username: st.SMTPUsername,
			From: st.SMTPFrom, Security: st.SMTPSecurity,
		},
		Storage: Storage{
			Enabled: st.StorageEnabled, Endpoint: st.StorageEndpoint, Region: st.StorageRegion, Bucket: st.StorageBucket,
			AccessKey: st.StorageAccessKey, PathStyle: st.StoragePathStyle,
		},
	}
	if st.GoogleSecret != "" {
		if inst.Google.ClientSecret, err = s.cipher.Open(st.GoogleSecret); err != nil {
			return Instance{}, fmt.Errorf("decrypt google secret: %w", err)
		}
	}
	if st.KVKKey != "" {
		if inst.KVK.APIKey, err = s.cipher.Open(st.KVKKey); err != nil {
			return Instance{}, fmt.Errorf("decrypt kvk key: %w", err)
		}
	}
	if st.SMTPPassword != "" {
		if inst.SMTP.Password, err = s.cipher.Open(st.SMTPPassword); err != nil {
			return Instance{}, fmt.Errorf("decrypt smtp password: %w", err)
		}
	}
	if st.StorageSecret != "" {
		if inst.Storage.SecretKey, err = s.cipher.Open(st.StorageSecret); err != nil {
			return Instance{}, fmt.Errorf("decrypt storage secret: %w", err)
		}
	}
	if st.RecognitionKey != "" {
		if inst.Recognition.APIKey, err = s.cipher.Open(st.RecognitionKey); err != nil {
			return Instance{}, fmt.Errorf("decrypt recognition api key: %w", err)
		}
	}
	return inst, nil
}

// WebhooksAllowLocal reports whether webhooks may reach plain HTTP and local
// addresses.
func (s *Service) WebhooksAllowLocal(ctx context.Context) (bool, error) {
	inst, err := s.Instance(ctx)
	if err != nil {
		return false, err
	}
	return inst.WebhooksAllowLocal, nil
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
	if u.LM2Enabled != nil {
		inst.LM2.Enabled = *u.LM2Enabled
	}
	if u.LM2URL != nil {
		inst.LM2.URL = strings.TrimRight(strings.TrimSpace(*u.LM2URL), "/")
	}
	if u.LM2App != nil {
		inst.LM2.App = strings.TrimSpace(*u.LM2App)
	}

	if u.KVKEnabled != nil {
		inst.KVK.Enabled = *u.KVKEnabled
	}
	if u.KVKAPIKey != nil {
		inst.KVK.APIKey = cmp.Or(*u.KVKAPIKey, inst.KVK.APIKey)
	}
	if u.KVKWebsiteSearchEnabled != nil {
		inst.KVK.WebsiteSearch = *u.KVKWebsiteSearchEnabled
	}
	if u.WebhooksAllowLocal != nil {
		inst.WebhooksAllowLocal = *u.WebhooksAllowLocal
	}
	if u.AddressLookupEnabled != nil {
		inst.AddressLookupEnabled = *u.AddressLookupEnabled
	}

	applySMTP(&inst.SMTP, u)
	applyStorage(&inst.Storage, u)
	applyRecognition(&inst.Recognition, u)

	var verr resource.ValidationError
	validateSMTP(inst.SMTP, &verr)
	validateStorage(inst.Storage, &verr)
	validateRecognition(inst.Recognition, &verr)
	if inst.KVK.Enabled && inst.KVK.APIKey == "" {
		verr.Add("kvk_api_key", "can't be blank while company search is enabled")
	}
	if inst.Google.Enabled && inst.Google.ClientID == "" {
		verr.Add("google_client_id", "can't be blank while Google login is enabled")
	}
	if inst.Google.Enabled && inst.Google.ClientSecret == "" {
		verr.Add("google_client_secret", "can't be blank while Google login is enabled")
	}
	if inst.LM2.Enabled && inst.LM2.URL == "" {
		verr.Add("lm2_url", "can't be blank while LM2 login is enabled")
	}
	if inst.LM2.Enabled && inst.LM2.App == "" {
		verr.Add("lm2_app", "can't be blank while LM2 login is enabled")
	}
	if inst.LM2.URL != "" && !validHTTPAddress(inst.LM2.URL) {
		verr.Add("lm2_url", "must be an http or https address without a query")
	}
	if err := verr.OrNil(); err != nil {
		return Instance{}, err
	}

	st := storedInstance{
		RegistrationOpen: inst.RegistrationOpen,
		GoogleEnabled:    inst.Google.Enabled,
		GoogleClientID:   inst.Google.ClientID,
		LM2Enabled:       inst.LM2.Enabled,
		LM2URL:           inst.LM2.URL,
		LM2App:           inst.LM2.App,
		KVKEnabled:       inst.KVK.Enabled,
		KVKWebsite:       inst.KVK.WebsiteSearch,
		AddressLookup:    inst.AddressLookupEnabled,
		WebhooksLocal:    inst.WebhooksAllowLocal,
		SMTPEnabled:      inst.SMTP.Enabled,
		SMTPHost:         inst.SMTP.Host,
		SMTPPort:         inst.SMTP.Port,
		SMTPUsername:     inst.SMTP.Username,
		SMTPFrom:         inst.SMTP.From,
		SMTPSecurity:     inst.SMTP.Security,
		StorageEnabled:   inst.Storage.Enabled,
		StorageEndpoint:  inst.Storage.Endpoint,
		StorageRegion:    inst.Storage.Region,
		StorageBucket:    inst.Storage.Bucket,
		StorageAccessKey: inst.Storage.AccessKey,
		StoragePathStyle: inst.Storage.PathStyle,
		RecognitionAI:    inst.Recognition.AIEnabled,
		RecognitionProv:  inst.Recognition.Provider,
		RecognitionURL:   inst.Recognition.URL,
		RecognitionModel: inst.Recognition.Model,
	}
	if inst.Recognition.APIKey != "" {
		if st.RecognitionKey, err = s.cipher.Seal(inst.Recognition.APIKey); err != nil {
			return Instance{}, fmt.Errorf("encrypt recognition api key: %w", err)
		}
	}
	if inst.Storage.SecretKey != "" {
		if st.StorageSecret, err = s.cipher.Seal(inst.Storage.SecretKey); err != nil {
			return Instance{}, fmt.Errorf("encrypt storage secret: %w", err)
		}
	}
	if inst.SMTP.Password != "" {
		if st.SMTPPassword, err = s.cipher.Seal(inst.SMTP.Password); err != nil {
			return Instance{}, fmt.Errorf("encrypt smtp password: %w", err)
		}
	}
	if inst.KVK.APIKey != "" {
		if st.KVKKey, err = s.cipher.Seal(inst.KVK.APIKey); err != nil {
			return Instance{}, fmt.Errorf("encrypt kvk key: %w", err)
		}
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

func applySMTP(cfg *SMTP, u Update) {
	if u.SMTPEnabled != nil {
		cfg.Enabled = *u.SMTPEnabled
	}
	if u.SMTPHost != nil {
		cfg.Host = strings.TrimSpace(*u.SMTPHost)
	}
	if u.SMTPPort != nil {
		cfg.Port = *u.SMTPPort
	}
	if u.SMTPUsername != nil {
		cfg.Username = strings.TrimSpace(*u.SMTPUsername)
	}
	if u.SMTPPassword != nil {
		cfg.Password = cmp.Or(*u.SMTPPassword, cfg.Password)
	}
	if u.SMTPFrom != nil {
		cfg.From = strings.TrimSpace(*u.SMTPFrom)
	}
	if u.SMTPSecurity != nil {
		cfg.Security = *u.SMTPSecurity
	}
}

func validateSMTP(cfg SMTP, verr *resource.ValidationError) {
	if !cfg.Enabled {
		return
	}
	if cfg.Host == "" || strings.ContainsAny(cfg.Host, " \t\r\n") {
		verr.Add("smtp_host", "must be a host name")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		verr.Add("smtp_port", "must be between 1 and 65535")
	}
	if addr, err := mail.ParseAddress(cfg.From); err != nil || addr.Address != cfg.From {
		verr.Add("smtp_from", "must be a plain email address")
	}
	if !slices.Contains([]string{"starttls", "tls", "none"}, cfg.Security) {
		verr.Add("smtp_security", "is not included in the list")
	}
}

func applyStorage(cfg *Storage, u Update) {
	if u.StorageEnabled != nil {
		cfg.Enabled = *u.StorageEnabled
	}
	if u.StorageEndpoint != nil {
		cfg.Endpoint = strings.TrimRight(strings.TrimSpace(*u.StorageEndpoint), "/")
	}
	if u.StorageRegion != nil {
		cfg.Region = strings.TrimSpace(*u.StorageRegion)
	}
	if u.StorageBucket != nil {
		cfg.Bucket = strings.TrimSpace(*u.StorageBucket)
	}
	if u.StorageAccessKey != nil {
		cfg.AccessKey = strings.TrimSpace(*u.StorageAccessKey)
	}
	if u.StorageSecretKey != nil {
		cfg.SecretKey = cmp.Or(*u.StorageSecretKey, cfg.SecretKey)
	}
	if u.StoragePathStyle != nil {
		cfg.PathStyle = *u.StoragePathStyle
	}
}

func applyRecognition(cfg *Recognition, u Update) {
	if u.RecognitionAIEnabled != nil {
		cfg.AIEnabled = *u.RecognitionAIEnabled
	}
	if u.RecognitionAIProvider != nil {
		cfg.Provider = strings.TrimSpace(*u.RecognitionAIProvider)
	}
	if u.RecognitionAIURL != nil {
		cfg.URL = strings.TrimRight(strings.TrimSpace(*u.RecognitionAIURL), "/")
	}
	if u.RecognitionAIModel != nil {
		cfg.Model = strings.TrimSpace(*u.RecognitionAIModel)
	}
	if u.RecognitionAIAPIKey != nil {
		cfg.APIKey = cmp.Or(*u.RecognitionAIAPIKey, cfg.APIKey)
	}
}

func validateRecognition(cfg Recognition, verr *resource.ValidationError) {
	if !slices.Contains(RecognitionProviders, cfg.Provider) {
		verr.Add("recognition_ai_provider", "is not included in the list")
	}
	if cfg.URL != "" && !validHTTPAddress(cfg.URL) {
		verr.Add("recognition_ai_url", "must be an http or https address without a query")
	}
	if !cfg.AIEnabled {
		return
	}
	if cfg.Model == "" {
		verr.Add("recognition_ai_model", "can't be blank while the AI model is enabled")
	}
	if cfg.Provider == "anthropic" && cfg.APIKey == "" {
		verr.Add("recognition_ai_api_key", "can't be blank while the AI model is enabled")
	}
}

// bucketPattern follows the S3 bucket naming rules.
var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

func validateStorage(cfg Storage, verr *resource.ValidationError) {
	if !cfg.Enabled {
		return
	}
	if u, err := url.Parse(cfg.Endpoint); err != nil || !validHTTPAddress(cfg.Endpoint) || (u.Path != "" && u.Path != "/") {
		verr.Add("storage_endpoint", "must be an http or https address without a path")
	}
	if !bucketPattern.MatchString(cfg.Bucket) {
		verr.Add("storage_bucket", "must be a valid bucket name")
	}
	if cfg.AccessKey == "" {
		verr.Add("storage_access_key", "can't be blank while file storage is enabled")
	}
	if cfg.SecretKey == "" {
		verr.Add("storage_secret_key", "can't be blank while file storage is enabled")
	}
}

func validHTTPAddress(address string) bool {
	u, err := url.Parse(address)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" &&
		u.User == nil && u.RawQuery == "" && u.Fragment == ""
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
