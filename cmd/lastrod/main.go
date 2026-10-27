// Command lastrod: the server, in several roles (ADR-0005, ADR-0010). Every
// command runs here, against the database; the lastro client goes through the API.
//
//	lastrod serve        API + embedded frontend
//	lastrod sync         pull accounts and transactions from Pluggy
//	lastrod market       refresh CDI, IPCA, Tesouro Direto and quotes
//	lastrod migrate      apply pending migrations
//	lastrod fixtures     download Pluggy responses and anonymize them into test fixtures
//	lastrod seed         fill a dev database with made-up history (dev only)
//	lastrod version      print the version
//
// The CLI uses only the standard library. If it ever needs nested
// subcommands or completion, switch to cobra without touching the modules.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// version is set at build time: -ldflags "-X main.version=..."
var version = "dev"

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, args []string) error
}

func commands() []command {
	return []command{
		{"serve", "start the API and the embedded frontend", runServe},
		{"sync", "pull accounts and transactions from Pluggy", runSync},
		{"market", "refresh CDI, IPCA, Tesouro Direto and the quotes of what is held", runMarket},
		{"migrate", "apply pending migrations", runMigrate},
		{"fixtures", "download Pluggy responses and anonymize them into test fixtures", runFixtures},
		{"seed", "fill a dev database with fourteen months of made-up money (LASTRO_ENV=dev only)", runSeed},
		{"version", "print the version", runVersion},
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := dispatch(ctx, os.Args[1:])
	stop()
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func dispatch(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage()
		return nil
	}
	for _, c := range commands() {
		if c.name == args[0] {
			return c.run(ctx, args[1:])
		}
	}
	usage()
	return fmt.Errorf("unknown command: %q", args[0])
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: lastrod <command> [flags]")
	fmt.Fprintln(os.Stderr)
	for _, c := range commands() {
		fmt.Fprintf(os.Stderr, "  %-11s %s\n", c.name, c.summary)
	}
}

func runVersion(_ context.Context, _ []string) error {
	fmt.Println(version)
	return nil
}
