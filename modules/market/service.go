package market

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"strings"
	"time"
	_ "time/tzdata" // the image has no zoneinfo; days turn in São Paulo

	"lastro/modules/market/internal/brapi"
	"lastro/modules/market/internal/sgs"
	"lastro/modules/market/internal/tesouro"
	"lastro/money"
)

// The external sources, declared here because tests swap them.
type (
	SeriesSource interface {
		Series(ctx context.Context, code int, from, to time.Time) ([]sgs.Value, error)
	}
	QuoteSource interface {
		Quote(ctx context.Context, symbol, rng string) (brapi.Quote, error)
	}
	BondSource interface {
		Latest(ctx context.Context) ([]tesouro.Row, error)
	}
)

// Sources builds the real clients; an empty token reads brapi without one.
func Sources(brapiToken string) (SeriesSource, QuoteSource, BondSource) {
	return sgs.New(sgs.DefaultURL), brapi.New(brapi.DefaultURL, brapiToken), tesouro.New(tesouro.DefaultURL)
}

// history is how far back, in years, a series or a new ticker is read the
// first time; brapi takes "1y", "2y" or "5y".
const history = 2

var saoPaulo, _ = time.LoadLocation("America/Sao_Paulo")

type Service struct {
	repo   *PostgresRepository
	series SeriesSource
	quotes QuoteSource
	bonds  BondSource
	logger *slog.Logger
	now    func() time.Time
}

func New(repo *PostgresRepository, s SeriesSource, q QuoteSource, b BondSource, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Service{repo: repo, series: s, quotes: q, bonds: b, logger: logger, now: time.Now}
}

func (s *Service) today() time.Time {
	y, m, d := s.now().In(saoPaulo).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Report says what a refresh brought in. A source that failed is named in
// Failed; the others still count.
type Report struct {
	Rates  int
	Quotes int
	Bonds  int
	Failed []string
}

// Refresh reads what is new: CDI and IPCA since the last stored day, the
// symbols' prices (a year of closes the first time), and Tesouro Direto's
// latest rates. One source failing never stops the others.
func (s *Service) Refresh(ctx context.Context, symbols []string) (Report, error) {
	var rep Report
	var errs []error
	fail := func(what string, err error) {
		rep.Failed = append(rep.Failed, what)
		errs = append(errs, err)
		s.logger.WarnContext(ctx, "market source failed", "source", what, "err", err)
	}
	for _, x := range []struct {
		series Series
		code   int
	}{{CDI, sgs.CDI}, {IPCA, sgs.IPCA}} {
		n, err := s.refreshSeries(ctx, x.series, x.code)
		rep.Rates += n
		if err != nil {
			fail(string(x.series), err)
		}
	}
	for _, sym := range symbols {
		n, err := s.refreshQuote(ctx, sym)
		rep.Quotes += n
		if err != nil {
			fail(sym, err)
		}
	}
	n, err := s.refreshBonds(ctx)
	rep.Bonds = n
	if err != nil {
		fail("Tesouro Direto", err)
	}
	if len(errs) > 0 && rep.Rates+rep.Quotes+rep.Bonds == 0 {
		return rep, fmt.Errorf("market: refresh: %w", errors.Join(errs...))
	}
	return rep, nil
}

func (s *Service) refreshSeries(ctx context.Context, series Series, code int) (int, error) {
	last, err := s.repo.lastRate(ctx, series)
	if err != nil {
		return 0, err
	}
	today := s.today()
	from := today.AddDate(-history, 0, 0)
	if last.Year() > 1 {
		from = last.AddDate(0, 0, 1)
	}
	if from.After(today) {
		return 0, nil
	}
	values, err := s.series.Series(ctx, code, from, today)
	if err != nil {
		return 0, err
	}
	now := s.now()
	for _, v := range values {
		r, err := ParsePercentRate(v.Value)
		if err != nil {
			return 0, err
		}
		if err := s.repo.addRate(ctx, Point{Series: series, Date: v.Date, Rate: r}, "sgs", now); err != nil {
			return 0, err
		}
	}
	return len(values), nil
}

func (s *Service) refreshQuote(ctx context.Context, symbol string) (int, error) {
	n, err := s.repo.quoteCount(ctx, symbol)
	if err != nil {
		return 0, err
	}
	rng := "5d"
	if n == 0 {
		rng = fmt.Sprintf("%dy", history)
	}
	q, err := s.quotes.Quote(ctx, symbol, rng)
	if err != nil {
		return 0, err
	}
	if q.Currency != "" && q.Currency != "BRL" {
		return 0, fmt.Errorf("%s is quoted in %s", symbol, q.Currency)
	}
	now, today := s.now(), s.today()
	saved := 0
	save := func(day time.Time, price string) error {
		c, err := cents(price)
		if err != nil {
			return err
		}
		y, m, d := day.In(saoPaulo).Date()
		err = s.repo.saveQuote(ctx, Quote{Symbol: symbol, Date: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), Close: c, Source: "brapi"}, now, today)
		if err == nil {
			saved++
		}
		return err
	}
	for _, h := range q.History {
		if err := save(h.Date, h.Close.String()); err != nil {
			return saved, err
		}
	}
	if q.Price != "" && !q.Time.IsZero() {
		if err := save(q.Time, q.Price.String()); err != nil {
			return saved, err
		}
	}
	return saved, nil
}

func (s *Service) refreshBonds(ctx context.Context) (int, error) {
	rows, err := s.bonds.Latest(ctx)
	if err != nil {
		return 0, err
	}
	now := s.now()
	for _, r := range rows {
		rate, err := ParsePercentRate(r.BuyRate)
		if err != nil {
			return 0, err
		}
		price, err := cents(r.BuyPrice)
		if err != nil {
			return 0, err
		}
		b := Bond{Name: r.Name, Maturity: r.Maturity, Date: r.Date, BuyRate: rate.Percent(), Price: price}
		if err := s.repo.addBond(ctx, b, now); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

// cents rounds a price in reais, written with a dot or a comma, to cents,
// half away from zero.
func cents(s string) (money.Money, error) {
	r, ok := new(big.Rat).SetString(strings.ReplaceAll(strings.TrimSpace(s), ",", "."))
	if !ok {
		return money.Money{}, fmt.Errorf("%w: price %q", ErrInvalid, s)
	}
	r.Mul(r, big.NewRat(100, 1))
	n, d := r.Num(), r.Denom()
	q, rem := new(big.Int).QuoRem(n, d, new(big.Int))
	if new(big.Int).Mul(new(big.Int).Abs(rem), big.NewInt(2)).Cmp(d) >= 0 {
		q.Add(q, big.NewInt(int64(n.Sign())))
	}
	return money.BRLCents(q.Int64()), nil
}

// Rates is a series between two days, both included.
func (s *Service) Rates(ctx context.Context, series Series, from, to time.Time) ([]Point, error) {
	out, err := s.repo.rates(ctx, series, from, to)
	if err != nil {
		return nil, fmt.Errorf("market: rates: %w", err)
	}
	return out, nil
}

// Accumulated is what a series added up to between two days, both
// included, and the last day it has a value for (zero when it has none).
func (s *Service) Accumulated(ctx context.Context, series Series, from, to time.Time) (Rate, time.Time, error) {
	pts, err := s.Rates(ctx, series, from, to)
	if err != nil {
		return Rate{}, time.Time{}, err
	}
	rates := make([]Rate, len(pts))
	var last time.Time
	for i, p := range pts {
		rates[i] = p.Rate
		last = p.Date
	}
	return Compound(rates), last, nil
}

// LatestQuotes is each symbol's most recent price, for the ones known.
func (s *Service) LatestQuotes(ctx context.Context, symbols []string) (map[string]Quote, error) {
	if len(symbols) == 0 {
		return map[string]Quote{}, nil
	}
	qs, err := s.repo.latestQuotes(ctx, symbols)
	if err != nil {
		return nil, fmt.Errorf("market: latest quotes: %w", err)
	}
	out := make(map[string]Quote, len(qs))
	for _, q := range qs {
		out[q.Symbol] = q
	}
	return out, nil
}

// Quotes is a symbol's closes since a day, oldest first.
func (s *Service) Quotes(ctx context.Context, symbol string, from time.Time) ([]Quote, error) {
	out, err := s.repo.quotes(ctx, symbol, from)
	if err != nil {
		return nil, fmt.Errorf("market: quotes: %w", err)
	}
	return out, nil
}

// Bonds is Tesouro Direto's latest day, by title and maturity.
func (s *Service) Bonds(ctx context.Context) ([]Bond, error) {
	out, err := s.repo.latestBonds(ctx)
	if err != nil {
		return nil, fmt.Errorf("market: bonds: %w", err)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
