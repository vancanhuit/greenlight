package data_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("GREENLIGHT_TEST_DB_DSN")
	if dsn == "" {
		// No test DB configured: skip DB tests entirely.
		os.Exit(m.Run())
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}
	testPool = pool

	sqlDB := stdlib.OpenDBFromPool(pool)
	if err := goose.SetDialect("postgres"); err != nil {
		panic(err)
	}
	if err := goose.Up(sqlDB, "../../migrations"); err != nil {
		panic(err)
	}
	_ = sqlDB.Close()

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func truncate(t *testing.T, tables ...string) {
	t.Helper()
	if testPool == nil {
		return
	}
	for _, tbl := range tables {
		_, err := testPool.Exec(context.Background(), "TRUNCATE "+tbl+" RESTART IDENTITY CASCADE")
		if err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}
}

func requireDB(t *testing.T) {
	t.Helper()
	if testPool == nil {
		t.Skip("GREENLIGHT_TEST_DB_DSN not set; skipping DB test")
	}
}
