// Command moneyharpy runs the MoneyHarpy server.
package main

import (
	"cmp"
	"context"
	"crypto/sha256"
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
	"github.com/adirkuhn/moneyharpy/internal/assets"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/connections"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/diskstore"
	"github.com/adirkuhn/moneyharpy/internal/downloads"
	"github.com/adirkuhn/moneyharpy/internal/google"
	"github.com/adirkuhn/moneyharpy/internal/history"
	"github.com/adirkuhn/moneyharpy/internal/hookclient"
	"github.com/adirkuhn/moneyharpy/internal/httpapi"
	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/invoicedoc"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/kvk"
	"github.com/adirkuhn/moneyharpy/internal/kvkweb"
	"github.com/adirkuhn/moneyharpy/internal/lm2"
	"github.com/adirkuhn/moneyharpy/internal/mail"
	"github.com/adirkuhn/moneyharpy/internal/membership"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/objectstore"
	"github.com/adirkuhn/moneyharpy/internal/ocr"
	"github.com/adirkuhn/moneyharpy/internal/overview"
	"github.com/adirkuhn/moneyharpy/internal/pdok"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/projects"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/ratelimit"
	"github.com/adirkuhn/moneyharpy/internal/recognize"
	"github.com/adirkuhn/moneyharpy/internal/recognize/ai"
	"github.com/adirkuhn/moneyharpy/internal/reports"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/adirkuhn/moneyharpy/internal/subscriptions"
	"github.com/adirkuhn/moneyharpy/internal/tasklists"
	"github.com/adirkuhn/moneyharpy/internal/timeentries"
	"github.com/adirkuhn/moneyharpy/internal/uploads"
	"github.com/adirkuhn/moneyharpy/internal/vatlookup"
	"github.com/adirkuhn/moneyharpy/internal/vies"
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
	// ocrURL is the OCR service (cmd/ocrd); without it, documents are not
	// read with OCR and previews show the file itself.
	ocrURL string
}

func loadConfig() (config, error) {
	c := config{
		databaseURL:       os.Getenv("DATABASE_URL"),
		secretKey:         os.Getenv("SECRET_KEY"),
		baseURL:           strings.TrimRight(os.Getenv("BASE_URL"), "/"),
		addr:              cmp.Or(os.Getenv("LISTEN_ADDR"), ":8080"),
		trustProxyHeaders: os.Getenv("TRUST_PROXY_HEADERS") == "true",
		uploadsDir:        cmp.Or(os.Getenv("UPLOADS_DIR"), "/data/uploads"),
		ocrURL:            os.Getenv("OCR_URL"),
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
	purchaseStore := postgres.NewPurchaseStore(pool)
	attachmentService := attachments.NewService(postgres.NewAttachmentStore(pool), files, ids, time.Now)
	timeEntryStore := postgres.NewTimeEntryStore(pool)
	contactService := contacts.NewService(admins, invoices, admins, contacts.AnyReferences{journalStore, salesStore, timeEntryStore, purchaseStore}, noteService, postgres.NewContactStore(pool), ids, time.Now)
	catalogStore := postgres.NewCatalogStore(pool)
	subscriptionStore := postgres.NewSubscriptionStore(pool)
	productService := products.NewService(admins, books, contactService, catalogStore, products.AnyReferences{salesStore, subscriptionStore, purchaseStore}, ids, time.Now)
	projectService := projects.NewService(admins, catalogStore, projects.AnyReferences{salesStore, journalStore, timeEntryStore, purchaseStore}, ids, time.Now)
	ledger := journal.NewService(admins, books, contactService, projectService, journalStore, journalStore, noteService, attachmentService, ids, time.Now)
	invoiceService := sales.NewService(sales.Deps{
		Administrations: admins, Contacts: contactService, Products: productService, Projects: projectService, Invoicing: invoices, Ledger: books, Notes: noteService, Attachments: attachmentService,
		Store: salesStore, Renderer: invoicedoc.New(), Mailer: sales.MailClient{Client: mailer}, Bank: bank, History: history.NewService(postgres.NewEventStore(pool), ids, time.Now),
		Notifier: httpapi.SalesNotifier{Webhooks: hooks, Logger: logger}, IDs: ids, Now: time.Now,
	})
	bank.UseInvoices(invoiceService)
	// Subscriptions bill through recurring sales invoices and complete the
	// invoices those make; downloads keep what exports make.
	subscriptionService := subscriptions.NewService(subscriptions.Deps{
		Administrations: admins, Contacts: contactService, Products: productService, Sales: invoiceService, Store: subscriptionStore,
		Notifier: httpapi.SalesNotifier{Webhooks: hooks, Logger: logger}, IDs: ids, Now: time.Now,
	})
	invoiceService.UseRecurringHook(subscriptionService)
	downloadService := downloads.NewService(admins, subscriptionStore, files, ids, time.Now)
	// Incoming documents: purchase invoices, receipts, general and typeless
	// documents and purchase transactions.
	documentService := purchases.NewService(purchases.Deps{
		Administrations: admins, Contacts: contactService, Products: productService, Projects: projectService, Ledger: books, Notes: noteService,
		Attachments: attachmentService, Store: purchaseStore, Bank: purchases.Banking{Service: bank}, IDs: ids, Now: time.Now,
	})
	bank.UseDocuments(documentService)
	// Scan and Recognise reads uploads in the background and fills them in:
	// e-invoices and text by its rules, scans through the OCR service, and
	// with the optional AI connections when an administrator turned them on.
	ocrService := ocr.New(cfg.ocrURL)
	// VIES is asked only while an administrator turned the VAT number lookup
	// on, by Scan and Recognise and by the contact form, sharing one cache.
	viesClient := vies.New(vies.ProductionURL, outbound)
	recognition := recognize.NewService(recognize.Deps{
		Documents: documentService, Contacts: contactService, Ledger: books, Settings: instance,
		Store: postgres.NewRecognitionStore(pool), OCR: ocrService,
		Parsers: recognize.Parsers{EInvoice: recognize.ParseEInvoice, Text: recognize.ParseText},
		// The AI connections are asked in the administrator's order.
		Models: func(c settings.Recognition) (recognize.Model, error) { return ai.FromSettings(c, logger) },
		// New suppliers get their registered name from VIES, when an
		// administrator turned that on.
		Registry: viesClient,
		Notifier: httpapi.RecognitionNotifier{Webhooks: hooks, Logger: logger}, Logger: logger, Now: time.Now,
	})
	documentService.UseRecognition(recognition)
	bank.UsePurchases(documentService)
	bank.UseProjects(projectService)
	// Fixed assets, depreciated each month by a background job.
	assetService := assets.NewService(admins, books, postgres.NewAssetStore(pool), httpapi.AssetNotifier{Webhooks: hooks, Logger: logger}, ids, time.Now)
	home := overview.NewService(admins, bank, invoiceService)
	home.UseIncoming(documentService)
	// Reports read the journal; their exports land in the downloads.
	reportService := reports.NewService(admins, postgres.NewReportStore(pool), downloadService, books, time.Now)
	// The official KVK API is preferred; the experimental website search is
	// used only while the API is not enabled, and is capped for the whole
	// installation to stay a polite visitor.
	kvkWebsite := kvkweb.New(instance, kvkweb.ProductionURL, kvkweb.ProductionEnvConfig, outbound, ratelimit.New(30, time.Minute))
	handler, err := httpapi.NewHandler(httpapi.Deps{
		Administrations:   admins,
		Invoicing:         invoices,
		Bookkeeping:       books,
		Banking:           bank,
		Overview:          home,
		Journal:           ledger,
		Contacts:          contactService,
		Sales:             invoiceService,
		Purchases:         documentService,
		Recognition:       recognition,
		Assets:            assetService,
		Products:          productService,
		Projects:          projectService,
		Subscriptions:     subscriptionService,
		Downloads:         downloadService,
		Reports:           reportService,
		TimeEntries:       timeentries.NewService(admins, timeEntryStore, noteService, ids, time.Now),
		TaskLists:         tasklists.NewService(admins, postgres.NewTaskListStore(pool), noteService, tasklists.LedgerReports{Accounts: books}, ids, time.Now),
		Storage:           s3,
		Identity:          people,
		Membership:        membership.NewService(admins, postgres.NewMembershipStore(pool), mailer, people, invoices, ids, time.Now),
		Connections:       connections.NewService(admins, postgres.NewConnectionStore(pool), ids, time.Now),
		Webhooks:          hooks,
		Settings:          instance,
		Resources:         postgres.NewResourceLocator(pool),
		Google:            google.New(instance, cfg.baseURL, google.Issuer),
		LM2:               lm2.New(instance, cfg.baseURL, outbound),
		Companies:         administration.Registers{kvk.New(instance, kvk.ProductionURL, outbound), kvkWebsite},
		VATNumbers:        vatlookup.New(instance, viesClient),
		CompanyCheck:      kvkWebsite,
		Addresses:         pdok.New(instance, pdok.ProductionURL, outbound),
		LookupLimiter:     ratelimit.New(60, time.Minute),
		ResetLimiter:      ratelimit.New(3, time.Hour),
		Mail:              mailer,
		OCR:               ocrService,
		BaseURL:           cfg.baseURL,
		Logger:            logger,
		Web:               web.Dist(),
		APILimiter:        ratelimit.New(150, 5*time.Minute),
		AuthLimiter:       ratelimit.New(10, time.Minute),
		TrustProxyHeaders: cfg.trustProxyHeaders,
		FileKey:           fileKey(cfg.secretKey),
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
	go invoiceService.Run(ctx, time.Minute, func(err error) { logger.Error("sales invoice background work", "error", err) })
	go subscriptionService.Run(ctx, time.Minute, func(err error) { logger.Error("subscription charge billing", "error", err) })
	go recognition.Run(ctx, 15*time.Second, func(err error) { logger.Error("scan and recognise", "error", err) })
	go assetService.Run(ctx, time.Hour, func(err error) { logger.Error("asset depreciation", "error", err) })
	// Attachments added before they had a sum get one, once, for duplicate detection.
	go func() {
		if err := attachmentService.HashMissing(ctx, purchases.ComparedKinds); err != nil {
			logger.Error("sum attachments", "error", err)
		}
	}()

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

// fileKey derives the key that signs download links from the secret key, so
// every instance accepts the links of the others.
func fileKey(secret string) []byte {
	sum := sha256.Sum256([]byte("moneyharpy file links\x00" + secret))
	return sum[:]
}
