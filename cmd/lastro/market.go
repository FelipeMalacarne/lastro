package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"lastro/app"
	"lastro/platform/config"
)

// runMarket refreshes CDI, IPCA, Tesouro Direto and the quotes of the
// tickers held, plus any named on the command line. lastro sync does the
// same after every sync.
func runMarket(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("market", flag.ContinueOnError)
	extra := fs.String("symbols", "", "more tickers to quote, comma separated (e.g. PETR4,IVVB11)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	a, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	var symbols []string
	for _, s := range strings.Split(*extra, ",") {
		if s = strings.ToUpper(strings.TrimSpace(s)); s != "" {
			symbols = append(symbols, s)
		}
	}
	rep, err := a.RefreshMarket(ctx, symbols...)
	fmt.Printf("%d index values, %d quotes, %d Tesouro Direto titles", rep.Rates, rep.Quotes, rep.Bonds)
	if len(rep.Failed) > 0 {
		fmt.Printf("; failed: %s (see the log)", strings.Join(rep.Failed, ", "))
	}
	fmt.Println()
	return err
}
