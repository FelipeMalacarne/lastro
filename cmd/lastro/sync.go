package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"time"

	"lastro/cmd/lastro/internal/client"
)

// runSync starts a sync on the server and follows it until it ends. If one
// is already running, it follows that one.
func runSync(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	detach := fs.Bool("detach", false, "start the sync and return without waiting")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := connected(ctx)
	if err != nil {
		return err
	}
	st, err := c.StartSync(ctx)
	switch {
	case errors.Is(err, client.ErrSyncRunning):
		fmt.Println("A sync is already running on the server; following it.")
	case err != nil:
		return fmt.Errorf("sync: %w", err)
	default:
		fmt.Println("Sync started on the server.")
	}
	if *detach {
		return nil
	}
	return follow(ctx, c, st, 2*time.Second)
}

func follow(ctx context.Context, c *client.Client, st client.SyncStatus, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
		var err error
		if st, err = c.SyncStatus(ctx); err != nil {
			return fmt.Errorf("sync: %w", err)
		}
		if st.Running {
			continue
		}
		if !st.OK {
			return fmt.Errorf("sync failed: %s", st.Message)
		}
		fmt.Println(st.Message)
		return nil
	}
}
