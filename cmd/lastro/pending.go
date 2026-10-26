package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
)

var errNotYet = errors.New("not implemented yet")

// runSync lands in M1 (ingest + ledger).
func runSync(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return fmt.Errorf("sync: %w (M1, see docs/PLAN.md)", errNotYet)
}

// runContribute lands in M4 (market + portfolio).
func runContribute(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("contribute", flag.ContinueOnError)
	_ = fs.String("amount", "", "contribution amount, e.g. 4000")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return fmt.Errorf("contribute: %w (M4, see docs/PLAN.md)", errNotYet)
}
