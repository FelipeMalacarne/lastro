// Command lastro: one binary, several roles.
//
//	lastro serve        API + embedded frontend
//	lastro sync         pull accounts and transactions from Pluggy
//	lastro contribute   show where this month's contribution should go (M4)
//	lastro migrate      apply pending migrations
//	lastro fixtures     download Pluggy responses and anonymize them into test fixtures
//	lastro version      print the version
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
		{"contribute", "show where this month's contribution should go", runContribute},
		{"migrate", "apply pending migrations", runMigrate},
		{"fixtures", "download Pluggy responses and anonymize them into test fixtures", runFixtures},
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
	fmt.Fprintln(os.Stderr, "usage: lastro <command> [flags]")
	fmt.Fprintln(os.Stderr)
	for _, c := range commands() {
		fmt.Fprintf(os.Stderr, "  %-11s %s\n", c.name, c.summary)
	}
}

func runVersion(_ context.Context, _ []string) error {
	fmt.Println(version)
	return nil
}
