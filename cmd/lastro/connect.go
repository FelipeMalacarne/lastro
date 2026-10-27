package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"lastro/cmd/lastro/internal/client"
)

// runConnect points the CLI at a server and signs in, if it asks for it,
// through the device flow: a code to confirm in the browser.
func runConnect(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: lastro connect <url>, e.g. lastro connect https://lastro.ftm.dev.br")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("connect: give the server's URL")
	}
	l, err := client.StartLogin(ctx, fs.Arg(0))
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	if l.Device != nil {
		fmt.Printf("Open %s\nand confirm the code %s.\n\nWaiting…\n", l.Device.URL, l.Device.UserCode)
	}
	cfg, err := l.Finish(ctx)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	me, err := client.New(ctx, cfg, client.Save).Me(ctx)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	if err := client.Save(cfg); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	p, _ := client.Path()
	fmt.Printf("Connected to %s as %s. Saved in %s.\n", cfg.URL, who(me), p)
	return nil
}

func runWhoami(ctx context.Context, _ []string) error {
	cfg, err := client.Load()
	if err != nil {
		return err
	}
	me, err := client.New(ctx, cfg, client.Save).Me(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("%s at %s (%s, %s; server %s)\n", who(me), cfg.URL, me.Client, me.Permission, me.Version)
	return nil
}

func runDisconnect(_ context.Context, _ []string) error {
	if err := client.Remove(); err != nil {
		return err
	}
	fmt.Println("Disconnected. The token is gone from this machine; it stays valid at the provider until it expires.")
	return nil
}

// connected loads the saved server, for every command that talks to it.
func connected(ctx context.Context) (*client.Client, error) {
	cfg, err := client.Load()
	if err != nil {
		return nil, err
	}
	return client.New(ctx, cfg, client.Save), nil
}

func who(me client.Me) string {
	if me.Email != "" {
		return me.Email
	}
	return me.Subject
}
