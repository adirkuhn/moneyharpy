// Package postgrestest gives tests disposable, migrated PostgreSQL databases.
//
// One container serves the whole test binary. Every call gets a database of
// its own in it: an empty one is created, a migrated one is copied from a
// template migrated once, which takes a fraction of a second where starting
// a container takes seconds. Testcontainers removes the container when the
// test binary ends.
package postgrestest

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const template = "migrated_template"

var (
	once     sync.Once
	server   *url.URL // the container's maintenance database
	setupErr error
	mu       sync.Mutex // CREATE DATABASE from one template runs one at a time
	seq      atomic.Int64

	templateOnce sync.Once
	templateErr  error
)

// start runs the shared container once.
func start(ctx context.Context) (*url.URL, error) {
	once.Do(func() {
		container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
			tcpostgres.WithDatabase("moneyharpy"),
			tcpostgres.WithUsername("moneyharpy"),
			tcpostgres.WithPassword("moneyharpy"),
			tcpostgres.BasicWaitStrategies(),
		)
		if err != nil {
			setupErr = fmt.Errorf("start container: %w", err)
			return
		}
		raw, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			setupErr = fmt.Errorf("connection string: %w", err)
			return
		}
		server, setupErr = url.Parse(raw)
	})
	return server, setupErr
}

// create makes a new database in the shared container, copied from the
// template when one is given, and returns its URL and a function that
// drops it.
func create(ctx context.Context, from string) (string, func(), error) {
	base, err := start(ctx)
	if err != nil {
		return "", nil, err
	}
	name := fmt.Sprintf("test_%d", seq.Add(1))
	stmt := "CREATE DATABASE " + name
	if from != "" {
		stmt += " TEMPLATE " + from
	}
	if err := exec(ctx, base.String(), stmt); err != nil {
		return "", nil, fmt.Errorf("create database: %w", err)
	}
	db := *base
	db.Path = "/" + name
	drop := func() {
		// Best effort: the container goes when the test binary ends.
		_ = exec(context.Background(), base.String(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	}
	return db.String(), drop, nil
}

func exec(ctx context.Context, url, stmt string) error {
	mu.Lock()
	defer mu.Unlock()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	_, err = conn.Exec(ctx, stmt)
	return err
}

// migrateTemplate creates and migrates the template database.
func migrateTemplate(ctx context.Context) error {
	base, err := start(ctx)
	if err != nil {
		return err
	}
	if err := exec(ctx, base.String(), "CREATE DATABASE "+template); err != nil {
		return fmt.Errorf("create template: %w", err)
	}
	db := *base
	db.Path = "/" + template
	return postgres.Migrate(ctx, db.String())
}

// StartEmpty creates a database with no schema and returns its connection
// string. The returned function drops it.
func StartEmpty(ctx context.Context) (string, func(), error) {
	return create(ctx, "")
}

// Start creates a database with all migrations applied. The returned
// function releases the pool and drops the database.
func Start(ctx context.Context) (*pgxpool.Pool, func(), error) {
	templateOnce.Do(func() { templateErr = migrateTemplate(ctx) })
	if templateErr != nil {
		return nil, nil, templateErr
	}
	url, drop, err := create(ctx, template)
	if err != nil {
		return nil, nil, err
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		drop()
		return nil, nil, fmt.Errorf("open pool: %w", err)
	}
	return pool, func() {
		pool.Close()
		drop()
	}, nil
}
