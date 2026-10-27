package ingest

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"lastro/modules/ingest/internal/pluggy"
	"lastro/modules/portfolio"
	"lastro/money"
)

// mapInvestment turns a Pluggy investment into what portfolio follows.
// Fully withdrawn ones come through inactive, so their position closes.
func mapInvestment(inv pluggy.Investment, item, institution string, at time.Time) (portfolio.Report, error) {
	if inv.CurrencyCode != "" && inv.CurrencyCode != "BRL" {
		return portfolio.Report{}, fmt.Errorf("%w: investment in %s", errUnsupported, inv.CurrencyCode)
	}
	r := portfolio.Report{
		ExternalID:  inv.ID,
		Item:        item,
		Institution: institution,
		Kind:        kindOfInvestment(inv),
		Name:        strings.TrimSpace(inv.Name),
		Indexer:     indexerOf(inv.RateType),
		AsOf:        at,
		Active:      inv.Status == "" || inv.Status == "ACTIVE",
	}
	if inv.Code != "" {
		if t, err := portfolio.NewTicker(inv.Code); err == nil {
			r.Ticker = t
		}
	}
	value := inv.Balance
	if value == "" {
		value = inv.Amount
	}
	cents, err := parseCents(cmpNumber(value, "0"))
	if err != nil {
		return portfolio.Report{}, err
	}
	r.Value = money.BRLCents(max(cents, 0))
	if inv.Original != "" {
		c, err := parseCents(inv.Original)
		if err != nil {
			return portfolio.Report{}, err
		}
		m := money.BRLCents(c)
		r.Invested = &m
	}
	if inv.Quantity != "" {
		q, err := portfolio.ParseQuantity(string(inv.Quantity))
		if err != nil {
			return portfolio.Report{}, err
		}
		r.Quantity = q
	}
	// 100% of CDI or Selic; IPCA + 6,1% or 12% a year fixed.
	switch r.Indexer {
	case portfolio.IndexerCDI, portfolio.IndexerSelic:
		r.Rate = percentBps(inv.Rate)
	case portfolio.IndexerIPCA, portfolio.IndexerPre:
		r.Rate = percentBps(inv.FixedRate)
	}
	if inv.Date != "" {
		if d, err := parseDate(inv.Date); err == nil {
			r.AsOf = d
		}
	}
	if inv.DueDate != "" {
		if d, err := parseDate(inv.DueDate); err == nil {
			r.Maturity = &d
		}
	}
	return r, nil
}

func kindOfInvestment(inv pluggy.Investment) portfolio.Kind {
	switch inv.Type {
	case "FIXED_INCOME":
		if inv.Subtype == "TREASURY" {
			return portfolio.KindTreasury
		}
		return portfolio.KindFixedIncome
	case "EQUITY":
		switch inv.Subtype {
		case "BDR":
			return portfolio.KindBDR
		case "REAL_ESTATE_FUND":
			return portfolio.KindFII
		case "ETF":
			return portfolio.KindETF
		}
		return portfolio.KindStock
	case "ETF":
		return portfolio.KindETF
	case "MUTUAL_FUND":
		return portfolio.KindFund
	}
	return portfolio.KindOther
}

func indexerOf(rateType string) portfolio.Indexer {
	switch strings.ToUpper(rateType) {
	case "CDI":
		return portfolio.IndexerCDI
	case "SELIC":
		return portfolio.IndexerSelic
	case "IPCA":
		return portfolio.IndexerIPCA
	case "PRE_FIXADO", "PREFIXADO", "PRE":
		return portfolio.IndexerPre
	}
	return portfolio.IndexerNone
}

// percentBps reads a percent ("100", "6.1") into basis points; empty is zero.
func percentBps(n json.Number) money.Percent {
	r, ok := new(big.Rat).SetString(string(n))
	if !ok {
		return money.FromBps(0)
	}
	r.Mul(r, big.NewRat(100, 1))
	r.Add(r, big.NewRat(1, 2))
	return money.FromBps(new(big.Int).Quo(r.Num(), r.Denom()).Int64())
}

func cmpNumber(n json.Number, def string) json.Number {
	if n == "" {
		return json.Number(def)
	}
	return n
}
