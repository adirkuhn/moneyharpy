// Command moneyharpy runs the MoneyHarpy server.
package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/connections"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/diskstore"
	"github.com/adirkuhn/moneyharpy/internal/google"
	"github.com/adirkuhn/moneyharpy/internal/hookclient"
	"github.com/adirkuhn/moneyharpy/internal/httpapi"
	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/invoicedoc"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/kvk"
	"github.com/adirkuhn/moneyharpy/internal/lm2"
	"github.com/adirkuhn/moneyharpy/internal/mail"
	"github.com/adirkuhn/moneyharpy/internal/membership"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/objectstore"
	"github.com/adirkuhn/moneyharpy/internal/overview"
	"github.com/adirkuhn/moneyharpy/internal/pdok"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/ratelimit"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/adirkuhn/moneyharpy/internal/uploads"
	"github.com/adirkuhn/moneyharpy/internal/webhooks"
	"github.com/adirkuhn/moneyharpy/web"
	"github.com/jackc/pgx/v5/pgxpool"
)

const minSecretKeyLength = 32

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

type config struct {
	databaseURL       string
	secretKey         string
	baseURL           string
	addr              string
	trustProxyHeaders bool
	// uploadsDir holds uploaded files while no S3-compatible storage is
	// configured.
	uploadsDir string
}

func loadConfig() (config, error) {
	c := config{
		databaseURL:       os.Getenv("DATABASE_URL"),
		secretKey:         os.Getenv("SECRET_KEY"),
		baseURL:           strings.TrimRight(os.Getenv("BASE_URL"), "/"),
		addr:              cmp.Or(os.Getenv("LISTEN_ADDR"), ":8080"),
		trustProxyHeaders: os.Getenv("TRUST_PROXY_HEADERS") == "true",
		uploadsDir:        cmp.Or(os.Getenv("UPLOADS_DIR"), "/data/uploads"),
	}
	switch {
	case c.databaseURL == "":
		return config{}, errors.New("DATABASE_URL is required")
	case len(c.secretKey) < minSecretKeyLength:
		return config{}, fmt.Errorf("SECRET_KEY must be at least %d characters", minSecretKeyLength)
	case c.baseURL == "":
		return config{}, errors.New("BASE_URL is required")
	}
	return c, nil
}

func run(logger *slog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := postgres.Migrate(ctx, cfg.databaseURL); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer pool.Close()

	ids := resource.NewGenerator(1)
	outbound := &http.Client{Timeout: 10 * time.Second}
	cipher := settings.NewCipher(cfg.secretKey)
	instance := settings.NewService(postgres.NewSettingsStore(pool), cipher)
	mailer := mail.New(instance, cfg.baseURL)
	s3 := objectstore.New(instance)
	files := uploads.New(s3, diskstore.New(cfg.uploadsDir))
	admins := administration.NewService(postgres.NewAdministrationStore(pool), ids, time.Now)
	invoices := invoicing.NewService(admins, postgres.NewInvoicingStore(pool), files, ids, time.Now)
	hooks := webhooks.NewService(admins, postgres.NewWebhookStore(pool, cipher), hookclient.New(), instance, ids, time.Now)
	people := identity.NewService(postgres.NewIdentityStore(pool), instance, ids, identity.NewHasher(identity.DefaultParams), mailer, cipher, time.Now)
	books := bookkeeping.NewService(admins, postgres.NewBookkeepingStore(pool), ids, time.Now)
	bank := banking.NewService(admins, books, postgres.NewBankingStore(pool), ids, time.Now)
	noteService := notes.NewService(postgres.NewNoteStore(pool), ids, time.Now)
	journalStore := postgres.NewJournalStore(pool)
	salesStore := postgres.NewSalesStore(pool)
	attachmentService := attachments.NewService(postgres.NewAttachmentStore(pool), files, ids, time.Now)
	contactService := contacts.NewService(admins, invoices, admins, contacts.AnyReferences{journalStore, salesStore}, noteService, postgres.NewContactStore(pool), ids, time.Now)
	ledger := journal.NewService(admins, books, contactService, journalStore, journalStore, noteService, attachmentService, ids, time.Now)
	invoiceService := sales.NewService(sales.Deps{
		Administrations: admins, Contacts: contactService, Invoicing: invoices, Ledger: books, Notes: noteService, Attachments: attachmentService,
		Store: salesStore, Renderer: invoicedoc.New(), Mailer: sales.MailClient{Client: mailer}, IDs: ids, Now: time.Now,
	})
	handler, err := httpapi.NewHandler(httpapi.Deps{
		Administrations:   admins,
		Invoicing:         invoices,
		Bookkeeping:       books,
		Banking:           bank,
		Overview:          overview.NewService(admins, bank),
		Journal:           ledger,
		Contacts:          contactService,
		Sales:             invoiceService,
		Storage:           s3,
		Identity:          people,
		Membership:        membership.NewService(admins, postgres.NewMembershipStore(pool), mailer, people, invoices, ids, time.Now),
		Connections:       connections.NewService(admins, postgres.NewConnectionStore(pool), ids, time.Now),
		Webhooks:          hooks,
		Settings:          instance,
		Resources:         postgres.NewResourceLocator(pool),
		Google:            google.New(instance, cfg.baseURL, google.Issuer),
		LM2:               lm2.New(instance, cfg.baseURL, outbound),
		Companies:         kvk.New(instance, kvk.ProductionURL, outbound),
		Addresses:         pdok.New(instance, pdok.ProductionURL, outbound),
		LookupLimiter:     ratelimit.New(60, time.Minute),
		ResetLimiter:      ratelimit.New(3, time.Hour),
		Mail:              mailer,
		BaseURL:           cfg.baseURL,
		Logger:            logger,
		Web:               web.Dist(),
		APILimiter:        ratelimit.New(150, 5*time.Minute),
		AuthLimiter:       ratelimit.New(10, time.Minute),
		TrustProxyHeaders: cfg.trustProxyHeaders,
	})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	go hooks.Run(ctx, 5*time.Second, func(err error) { logger.Error("deliver webhooks", "error", err) })

	errs := make(chan error, 1)
	go func() { errs <- srv.ListenAndServe() }()
	logger.Info("listening", "addr", cfg.addr, "base_url", cfg.baseURL)

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}
