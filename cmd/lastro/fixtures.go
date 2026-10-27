package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"lastro/modules/ingest"
	"lastro/platform/config"
)

// runFixtures is the Pluggy spike tool: it pulls real responses, prints a
// report that is safe to share and anonymizes them into fixtures. It only
// needs the LASTRO_PLUGGY_* variables, not the database.
func runFixtures(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("fixtures", flag.ContinueOnError)
	out := fs.String("out", "tmp/pluggy", "directory for the raw dump, report.txt and the anonymized copy; keep it out of git")
	report := fs.String("report", "", "only print the report of an existing dump or fixtures directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *report != "" {
		return ingest.ReportFixtures(*report, os.Stdout)
	}

	cfg, err := config.LoadPluggy()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(*out, "report.txt"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if err := ingest.DumpFixtures(ctx, cfg, *out, io.MultiWriter(os.Stdout, f)); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\nRaw responses: %s (real data, never commit it)\n", filepath.Join(*out, "raw"))
	fmt.Fprintf(os.Stderr, "Anonymized copy: %s (review it, then copy it to modules/ingest/testdata/pluggy)\n", filepath.Join(*out, "anonymized"))
	return nil
}
