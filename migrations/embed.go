// Package migrations holds the SQL migrations (goose format), embedded in the binary.
//
// Naming convention: <timestamp>_<module>_<description>.sql,
// e.g. 20261101120000_ledger_create_transactions.sql.
// Each module only creates and alters objects in its own schema.
package migrations

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

// Up applies every pending migration.
func Up(ctx context.Context, pool *pgxpool.Pool) (err error) {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() {
		if cerr := sqlDB.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("migrations: close: %w", cerr)
		}
	}()

	goose.SetBaseFS(files)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migrations: dialect: %w", err)
	}
	if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
		return fmt.Errorf("migrations: up: %w", err)
	}
	return nil
}

// DownTo rolls migrations back to a version: tests use it to check that a
// migration carries the data that was there before it.
func DownTo(ctx context.Context, pool *pgxpool.Pool, version int64) (err error) {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() {
		if cerr := sqlDB.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("migrations: close: %w", cerr)
		}
	}()

	goose.SetBaseFS(files)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migrations: dialect: %w", err)
	}
	if err := goose.DownToContext(ctx, sqlDB, ".", version); err != nil {
		return fmt.Errorf("migrations: down to %d: %w", version, err)
	}
	return nil
}
