package main

import (
	"context"
	"flag"

	"lastro/app"
	"lastro/migrations"
	"lastro/platform/config"
	"lastro/platform/db"
)

func runMigrate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := app.NewLogger(cfg)

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := migrations.Up(ctx, pool); err != nil {
		return err
	}
	logger.Info("migrations applied")
	return nil
}
