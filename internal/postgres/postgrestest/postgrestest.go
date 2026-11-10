// Package postgrestest starts disposable, migrated PostgreSQL databases for tests.
package postgrestest

import (
	"context"
	"fmt"

	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// StartEmpty runs a new PostgreSQL container with no schema and returns its
// connection string. The returned function removes the container.
func StartEmpty(ctx context.Context) (string, func(), error) {
	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("moneyharpy"),
		tcpostgres.WithUsername("moneyharpy"),
		tcpostgres.WithPassword("moneyharpy"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return "", nil, fmt.Errorf("start container: %w", err)
	}
	stop := func() {
		// Best effort: a leftover container is reaped by testcontainers.
		_ = testcontainers.TerminateContainer(container)
	}
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		stop()
		return "", nil, fmt.Errorf("connection string: %w", err)
	}
	return url, stop, nil
}

// Start runs a new PostgreSQL container with all migrations applied. The
// returned function releases the pool and removes the container.
func Start(ctx context.Context) (*pgxpool.Pool, func(), error) {
	url, stop, err := StartEmpty(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := postgres.Migrate(ctx, url); err != nil {
		stop()
		return nil, nil, err
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		stop()
		return nil, nil, fmt.Errorf("open pool: %w", err)
	}
	return pool, func() {
		pool.Close()
		stop()
	}, nil
}
