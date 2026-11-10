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
	"github.com/adirkuhn/moneyharpy/internal/google"
	"github.com/adirkuhn/moneyharpy/internal/httpapi"
	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/kvk"
	"github.com/adirkuhn/moneyharpy/internal/lm2"
	"github.com/adirkuhn/moneyharpy/internal/mail"
	"github.com/adirkuhn/moneyharpy/internal/pdok"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/ratelimit"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/settings"
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
}

func loadConfig() (config, error) {
	c := config{
		databaseURL:       os.Getenv("DATABASE_URL"),
		secretKey:         os.Getenv("SECRET_KEY"),
		baseURL:           strings.TrimRight(os.Getenv("BASE_URL"), "/"),
		addr:              cmp.Or(os.Getenv("LISTEN_ADDR"), ":8080"),
		trustProxyHeaders: os.Getenv("TRUST_PROXY_HEADERS") == "true",
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
	instance := settings.NewService(postgres.NewSettingsStore(pool), settings.NewCipher(cfg.secretKey))
	mailer := mail.New(instance, cfg.baseURL)
	handler, err := httpapi.NewHandler(httpapi.Deps{
		Administrations:   administration.NewService(postgres.NewAdministrationStore(pool), ids, time.Now),
		Identity:          identity.NewService(postgres.NewIdentityStore(pool), instance, ids, identity.NewHasher(identity.DefaultParams), mailer, time.Now),
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
