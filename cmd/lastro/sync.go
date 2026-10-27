package main

import (
	"context"
	"flag"
	"fmt"

	"lastro/app"
	"lastro/modules/ingest"
	"lastro/platform/config"
)

func runSync(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	full := fs.Bool("full", false, "read every item's whole history instead of the last 90 days")
	reprocess := fs.Bool("reprocess", false, "rebuild the ledger from the stored raw records, without calling Pluggy")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !*reprocess {
		if err := cfg.Pluggy.Validate(); err != nil {
			return err
		}
	}
	a, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer a.Close()

	rep, err := a.Ingest.Sync(ctx, ingest.SyncOptions{Full: *full, Reprocess: *reprocess})
	for _, it := range rep.Items {
		name := it.Institution
		if name == "" {
			name = it.ItemID
		}
		if it.Err != nil {
			fmt.Printf("%s: failed: %v\n", name, it.Err)
			continue
		}
		fmt.Printf("%s: %d accounts; %d new, %d updated, %d removed, %d unchanged transactions; %d investments stored",
			name, it.Accounts, it.Created, it.Updated, it.Removed, it.Unchanged, it.Investments)
		if it.Skipped > 0 {
			fmt.Printf("; %d skipped (see the log)", it.Skipped)
		}
		fmt.Println()
	}
	return err
}
