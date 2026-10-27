//go:build integration

// Package dbtest gives integration tests a real, migrated Postgres.
//
// The first call starts Postgres 17 in a container (testcontainers) and
// applies every migration to a template database; each call then clones the
// template, so every test starts from an empty, fully migrated database.
// Tests using it carry the integration build tag: `task test:integration`.
package dbtest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"lastro/migrations"
	"lastro/platform/db"
)

const template = "lastro_template"

var (
	once     sync.Once
	baseURL  string // connection string with the database name as %s
	startErr error
	counter  atomic.Int64
)

// New returns a pool to a fresh database, closed when the test ends.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	once.Do(start)
	if startErr != nil {
		t.Fatalf("dbtest: %v", startErr)
	}
	ctx := context.Background()
	// CREATE DATABASE … TEMPLATE needs nobody connected to the template.
	admin, err := db.Open(ctx, fmt.Sprintf(baseURL, "postgres"))
	if err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	name := fmt.Sprintf("test_%d", counter.Add(1))
	_, err = admin.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", name, template))
	admin.Close()
	if err != nil {
		t.Fatalf("dbtest: create database: %v", err)
	}
	pool, err := db.Open(ctx, fmt.Sprintf(baseURL, name))
	if err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func start() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase(template),
		postgres.WithUsername("lastro"),
		postgres.WithPassword("lastro"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		startErr = fmt.Errorf("start postgres: %w", err)
		return
	}
	url, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		startErr = err
		return
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		startErr = err
		return
	}
	defer pool.Close()
	if err := migrations.Up(ctx, pool); err != nil {
		startErr = err
		return
	}
	baseURL = strings.Replace(strings.ReplaceAll(url, "%", "%%"), "/"+template+"?", "/%s?", 1)
}
