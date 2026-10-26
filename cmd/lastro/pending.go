package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
)

var errNotYet = errors.New("not implemented yet")

// runSync lands in Phase 1 (ingest + ledger).
func runSync(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return fmt.Errorf("sync: %w (Phase 1: ingest + ledger, see docs/PLAN.md)", errNotYet)
}

// runContribute lands in Phase 4 (market + portfolio).
func runContribute(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("contribute", flag.ContinueOnError)
	_ = fs.String("amount", "", "contribution amount, e.g. 4000")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return fmt.Errorf("contribute: %w (Phase 4: market + portfolio, see docs/PLAN.md)", errNotYet)
}
