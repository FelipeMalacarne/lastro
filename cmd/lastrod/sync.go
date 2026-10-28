package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

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
	// A reprocess needs no Pluggy credentials, but without item ids it would
	// quietly do nothing.
	if *reprocess && len(cfg.Pluggy.ItemIDs) == 0 {
		return errors.New("config: set LASTRO_PLUGGY_ITEM_IDS (the items to reprocess)")
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
		fmt.Printf("%s: %d accounts; %d new, %d updated, %d removed, %d unchanged transactions; %d card bills; %d investments stored, with %d movements",
			name, it.Accounts, it.Created, it.Updated, it.Removed, it.Unchanged, it.Bills, it.Investments, it.Movements)
		if it.Skipped > 0 {
			fmt.Printf("; %d skipped (see the log)", it.Skipped)
		}
		fmt.Println()
	}
	if rep.Paired > 0 {
		fmt.Printf("Paired %d transfers and card bill payments between my accounts\n", rep.Paired)
	}
	if err != nil {
		return err
	}
	// Prices and indices after the positions, then the day's valuation. A
	// market failure is reported; the sync itself already succeeded.
	mrep, merr := a.RefreshMarket(ctx)
	fmt.Printf("Market: %d index values, %d quotes, %d Tesouro Direto titles", mrep.Rates, mrep.Quotes, mrep.Bonds)
	if len(mrep.Failed) > 0 {
		fmt.Printf("; failed: %s", strings.Join(mrep.Failed, ", "))
	}
	fmt.Println()
	if merr != nil {
		a.Logger.WarnContext(ctx, "market refresh failed", "err", merr)
	}
	// New alerts go out once, if a channel is set (ADR-0007).
	if n, err := a.SendAlerts(ctx); err != nil {
		a.Logger.WarnContext(ctx, "alerts not sent", "err", err)
	} else if n > 0 {
		fmt.Printf("Sent %d alerts\n", n)
	}
	// Last month's report, once, from the 2nd of a month on.
	if sent, err := a.SendReport(ctx); err != nil {
		a.Logger.WarnContext(ctx, "monthly report not sent", "err", err)
	} else if sent {
		fmt.Println("Sent last month's report")
	}
	return nil
}
