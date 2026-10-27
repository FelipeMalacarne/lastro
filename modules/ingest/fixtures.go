package ingest

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"lastro/modules/ingest/internal/fixtures"
	"lastro/modules/ingest/internal/pluggy"
	"lastro/platform/config"
)

// DumpFixtures downloads everything Pluggy has for the configured items into
// dir/raw, prints a report that is safe to share and writes an anonymized
// copy into dir/anonymized. dir/raw holds real financial data and must never
// be committed; dir/anonymized is reviewed and then copied to testdata.
func DumpFixtures(ctx context.Context, cfg config.Pluggy, dir string, report io.Writer) error {
	raw := filepath.Join(dir, "raw")
	if err := fixtures.Dump(ctx, pluggy.New(cfg.ClientID, cfg.ClientSecret), cfg.ItemIDs, raw); err != nil {
		return fmt.Errorf("ingest: dump: %w", err)
	}
	if err := ReportFixtures(raw, report); err != nil {
		return err
	}
	a, err := fixtures.NewAnonymizer()
	if err != nil {
		return fmt.Errorf("ingest: anonymize: %w", err)
	}
	return a.Dir(raw, filepath.Join(dir, "anonymized"))
}

// ReportFixtures prints the report of a dump or of a fixtures directory.
func ReportFixtures(dir string, w io.Writer) error {
	s, err := fixtures.Load(dir)
	if err != nil {
		return fmt.Errorf("ingest: report: %w", err)
	}
	fixtures.Report(w, s, time.Now())
	return nil
}
