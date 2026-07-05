package main

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
	"github.com/vancanhuit/greenlight/migrations"
)

// migrateDB applies any pending embedded migrations. A Postgres session lock
// ensures that only one instance runs migrations at a time, which is safe when
// several API instances start concurrently.
func (app *application) migrateDB(pool *pgxpool.Pool) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	// Closing this wrapper does not close the underlying pgxpool.Pool, so the
	// application keeps using the pool after migrations complete.
	defer func() { _ = sqlDB.Close() }()

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		sqlDB,
		migrations.FS,
		goose.WithSessionLocker(locker),
	)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	results, err := provider.Up(ctx)
	if err != nil {
		return err
	}

	for _, r := range results {
		app.logger.Info("applied migration",
			"version", strconv.FormatInt(r.Source.Version, 10),
			"source", r.Source.Path,
		)
	}

	return nil
}
