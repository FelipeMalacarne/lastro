// Command lastro: the client (ADR-0010). Every command talks to a lastro
// server through its HTTP API; the work that runs against the database
// belongs to lastrod.
//
//	lastro version      print the version
//
// connect, sync and contribute arrive with M7. Like lastrod, the CLI uses only
// the standard library, and depguard keeps it out of app, the modules and
// platform.
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
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "lastro is the client; the server and its jobs are lastrod.")
}

func runVersion(_ context.Context, _ []string) error {
	fmt.Println(version)
	return nil
}
