package main

import (
	"strings"
	"testing"

	"lastro/cmd/lastro/internal/client"
	"lastro/modules/portfolio"
)

func TestParseReais(t *testing.T) {
	for in, want := range map[string]int64{"3000": 300000, "3000.50": 300050, "3.000,50": 300050, "R$ 3.000": 300000, "1.234.567": 123456700} {
		got, err := parseReais(in)
		if err != nil || got.Cents() != want {
			t.Errorf("%q = %d, %v", in, got.Cents(), err)
		}
	}
	if _, err := parseReais("abc"); err == nil {
		t.Error("abc is not an amount")
	}
}

func TestFormatPlan(t *testing.T) {
	buy := "Tesouro IPCA+ 2035, IPCA + 7,2%"
	out := formatPlan(client.Plan{AmountCents: 300000, Splits: []client.Split{
		{Class: "ipca", AmountCents: 200000, Why: "below_band", Buy: &buy},
		{Class: "global", AmountCents: 0, Why: "above_band"},
	}})
	want := "IPCA+             R$ 2.000,00  below band; Tesouro IPCA+ 2035, IPCA + 7,2%\n" +
		"Global stocks         R$ 0,00  above band\n" +
		"                  -----------\n" +
		"Total             R$ 3.000,00\n"
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
	if strings.Contains(out, "-R$") {
		t.Error("never a negative")
	}
}

// The client keeps its own names for the classes; they must match the module's.
func TestClassNamesMatchTheServer(t *testing.T) {
	for _, c := range portfolio.Classes {
		if got := className[string(c)]; got != c.Name() {
			t.Errorf("class %q: client says %q, server says %q", c, got, c.Name())
		}
	}
}
