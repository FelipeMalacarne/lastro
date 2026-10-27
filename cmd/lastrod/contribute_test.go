package main

import (
	"strings"
	"testing"

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
	out := formatPlan(portfolio.PlanView{AmountCents: 300000, Splits: []portfolio.SplitView{
		{Class: portfolio.ClassIPCA, AmountCents: 200000, Why: portfolio.WhyBelowBand, Buy: &buy},
		{Class: portfolio.ClassGlobal, AmountCents: 0, Why: portfolio.WhyAboveBand},
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
