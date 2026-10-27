package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"unicode/utf8"

	"lastro/app"
	"lastro/modules/budget"
	"lastro/modules/portfolio"
	"lastro/money"
	"lastro/platform/config"
)

// runContribute prints where a contribution goes, with the same planner as
// the "Where to contribute" screen. Without --amount it takes this month's
// pay yourself first.
func runContribute(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("contribute", flag.ContinueOnError)
	amount := fs.String("amount", "", "contribution in reais, e.g. 3000 or 3.000,50 (default: this month's pay yourself first)")
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

	var m money.Money
	if *amount != "" {
		if m, err = parseReais(*amount); err != nil {
			return err
		}
	} else {
		v, err := a.Budget.Month(ctx, budget.MonthOf(a.Portfolio.Today()))
		if err != nil {
			return err
		}
		if v.PayYourselfFirst.PlannedCents == 0 {
			return fmt.Errorf("contribute: pass --amount, or set pay yourself first in Settings")
		}
		m = money.BRLCents(v.PayYourselfFirst.PlannedCents)
	}
	plan, err := a.Portfolio.Plan(ctx, m)
	if err != nil {
		return err
	}
	fmt.Print(formatPlan(plan))
	return nil
}

// parseReais reads "3000", "3000.50", "3.000,50" or "R$ 3.000".
func parseReais(s string) (money.Money, error) {
	t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "R$"))
	if strings.Contains(t, ",") {
		t = strings.ReplaceAll(t, ".", "")
		t = strings.ReplaceAll(t, ",", ".")
	} else if strings.Count(t, ".") > 1 || (strings.Contains(t, ".") && len(t)-strings.LastIndex(t, ".") == 4) {
		t = strings.ReplaceAll(t, ".", "") // "3.000": thousands
	}
	m, err := money.ParseDecimal(t, money.BRL)
	if err != nil {
		return money.Money{}, fmt.Errorf("contribute: amount %q: %w", s, err)
	}
	return m, nil
}

func formatPlan(p portfolio.PlanView) string {
	width := utf8.RuneCountInString(money.BRLCents(p.AmountCents).String())
	// Right-aligned by runes, not bytes.
	right := func(s string) string { return strings.Repeat(" ", max(0, width-utf8.RuneCountInString(s))) + s }
	var b strings.Builder
	for _, s := range p.Splits {
		note := ""
		switch s.Why {
		case portfolio.WhyBelowBand:
			note = "below band"
		case portfolio.WhyAboveBand:
			note = "above band"
		}
		if s.Buy != nil && s.AmountCents > 0 {
			if note != "" {
				note += "; "
			}
			note += *s.Buy
		}
		fmt.Fprintf(&b, "%-17s %s  %s\n", s.Class.Name(), right(money.BRLCents(s.AmountCents).String()), note)
	}
	fmt.Fprintf(&b, "%-17s %s\n", "", strings.Repeat("-", width))
	fmt.Fprintf(&b, "%-17s %s\n", "Total", right(money.BRLCents(p.AmountCents).String()))
	return b.String()
}
