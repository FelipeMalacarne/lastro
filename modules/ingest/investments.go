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

// mapHistory turns an investment and its transactions into what portfolio
// keeps to rebuild the days before the first sync. Pluggy sends every
// amount positive; the type, and for a transfer the movement type, says
// the direction.
func mapHistory(inv pluggy.Investment, txs []pluggy.InvestmentTransaction, item, institution string, at time.Time) (portfolio.History, error) {
	r, err := mapInvestment(inv, item, institution, at)
	if err != nil {
		return portfolio.History{}, err
	}
	h := portfolio.History{Report: r, Movements: make([]portfolio.Movement, 0, len(txs))}
	if inv.IssueDate != "" {
		if d, err := parseDate(inv.IssueDate); err == nil {
			h.Issued = &d
		}
	}
	for _, t := range txs {
		m, err := mapMovement(t)
		if err != nil {
			return portfolio.History{}, fmt.Errorf("investment %s: %w", inv.ID, err)
		}
		h.Movements = append(h.Movements, m)
	}
	return h, nil
}

func mapMovement(t pluggy.InvestmentTransaction) (portfolio.Movement, error) {
	m := portfolio.Movement{ExternalID: t.ID}
	switch t.Type {
	case "BUY":
		m.Kind = portfolio.MoveBuy
	case "SELL":
		m.Kind = portfolio.MoveSell
	case "TRANSFER":
		m.Kind = portfolio.MoveTransferIn
		if t.MovementType == "DEBIT" {
			m.Kind = portfolio.MoveTransferOut
		}
	case "TAX":
		m.Kind = portfolio.MoveTax
	case "INTEREST":
		m.Kind = portfolio.MoveIncome
	case "AMORTIZATION":
		m.Kind = portfolio.MoveAmortization
	default:
		return portfolio.Movement{}, fmt.Errorf("%w: investment transaction of type %q", errUnsupported, t.Type)
	}
	d, err := parseDate(t.Date)
	if err != nil {
		return portfolio.Movement{}, err
	}
	m.Date = d
	amount, err := parseCents(cmpNumber(t.Amount, "0"))
	if err != nil {
		return portfolio.Movement{}, err
	}
	m.Amount = money.BRLCents(abs(amount))
	if t.NetAmount != "" {
		net, err := parseCents(t.NetAmount)
		if err != nil {
			return portfolio.Movement{}, err
		}
		n := money.BRLCents(abs(net))
		m.Net = &n
	}
	if t.Quantity != "" {
		q, err := portfolio.ParseQuantity(strings.TrimPrefix(string(t.Quantity), "-"))
		if err != nil {
			return portfolio.Movement{}, err
		}
		m.Quantity = &q
	}
	return m, nil
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
