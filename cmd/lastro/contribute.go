package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"unicode/utf8"

	"lastro/cmd/lastro/internal/client"
	"lastro/money"
)

// runContribute prints where a contribution goes, with the server's planner,
// the same as the "Where to contribute" screen. Without --amount it takes
// this month's pay yourself first.
func runContribute(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("contribute", flag.ContinueOnError)
	amount := fs.String("amount", "", "contribution in reais, e.g. 3000 or 3.000,50 (default: this month's pay yourself first)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := connected(ctx)
	if err != nil {
		return err
	}

	var cents int64
	if *amount != "" {
		m, err := parseReais(*amount)
		if err != nil {
			return err
		}
		cents = m.Cents()
	} else {
		b, err := c.Budget(ctx)
		if err != nil {
			return fmt.Errorf("contribute: %w", err)
		}
		if cents = b.PayYourselfFirst.PlannedCents; cents == 0 {
			return fmt.Errorf("contribute: pass --amount, or set pay yourself first in Settings")
		}
	}
	plan, err := c.Contribute(ctx, cents)
	if err != nil {
		return fmt.Errorf("contribute: %w", err)
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

// className is how a class reads on screen, as in the portfolio module.
var className = map[string]string{"cdi": "CDI", "ipca": "IPCA+", "global": "Global stocks", "br_stocks": "Brazilian stocks"}

func formatPlan(p client.Plan) string {
	width := utf8.RuneCountInString(money.BRLCents(p.AmountCents).String())
	// Right-aligned by runes, not bytes.
	right := func(s string) string { return strings.Repeat(" ", max(0, width-utf8.RuneCountInString(s))) + s }
	var b strings.Builder
	for _, s := range p.Splits {
		note := ""
		switch s.Why {
		case "below_band":
			note = "below band"
		case "above_band":
			note = "above band"
		}
		if s.Buy != nil && s.AmountCents > 0 {
			if note != "" {
				note += "; "
			}
			note += *s.Buy
		}
		name := className[s.Class]
		if name == "" {
			name = s.Class
		}
		fmt.Fprintf(&b, "%-17s %s  %s\n", name, right(money.BRLCents(s.AmountCents).String()), note)
	}
	fmt.Fprintf(&b, "%-17s %s\n", "", strings.Repeat("-", width))
	fmt.Fprintf(&b, "%-17s %s\n", "Total", right(money.BRLCents(p.AmountCents).String()))
	return b.String()
}
