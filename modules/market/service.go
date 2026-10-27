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
		Fundamentals(ctx context.Context, symbol string) (brapi.Fundamentals, error)
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
// first time; brapi takes "1y", "2y" or "5y". brapi's free plan goes back
// only shortRange, which a new ticker falls back to.
const (
	history    = 2
	shortRange = "3mo"
)

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
			continue
		}
		// Fundamentals and dividends are a bonus: a plan without them
		// never fails the refresh.
		if err := s.refreshFundamentals(ctx, sym); err != nil {
			s.logger.WarnContext(ctx, "fundamentals not read", "symbol", sym, "err", err)
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
	if errors.Is(err, brapi.ErrRange) && n == 0 {
		q, err = s.quotes.Quote(ctx, symbol, shortRange)
	}
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

// refreshFundamentals reads a company's per-share figures and dividends
// once a day. Indices (^BVSP) have none.
func (s *Service) refreshFundamentals(ctx context.Context, symbol string) error {
	if strings.HasPrefix(symbol, "^") {
		return nil
	}
	today, now := s.today(), s.now()
	if done, err := s.repo.hasFundamental(ctx, symbol, today); err != nil || done {
		return err
	}
	f, err := s.quotes.Fundamentals(ctx, symbol)
	if err != nil {
		return err
	}
	row := Fundamental{Symbol: symbol, Date: today}
	for _, x := range []struct {
		dst **int64
		s   string
		by  int64
	}{{&row.EPSMicro, f.EPS.String(), 1_000_000}, {&row.BookMicro, f.BookValue.String(), 1_000_000}, {&row.PEMilli, f.PE.String(), 1000}, {&row.PBMilli, f.PB.String(), 1000}} {
		if *x.dst, err = optional(x.s, x.by); err != nil {
			return err
		}
	}
	for _, d := range f.Dividends {
		rate, err := scaled(d.Rate.String(), 1_000_000)
		if err != nil || rate <= 0 {
			continue
		}
		label := d.Label
		if label == "" {
			label = "DIVIDENDO"
		}
		if err := s.repo.saveDividend(ctx, Dividend{Symbol: symbol, ComDate: d.ComDate, Label: label, RateMicro: rate, PaymentDate: d.PaymentDate}, "brapi", now); err != nil {
			return err
		}
	}
	return s.repo.saveFundamental(ctx, row, "brapi", now)
}

// Record stores market data that did not come from a source's refresh:
// the dev seed's, or figures typed in. Closes are kept as given.
func (s *Service) Record(ctx context.Context, quotes []Quote, divs []Dividend, funds []Fundamental) error {
	now := s.now()
	for _, q := range quotes {
		if err := s.repo.saveQuote(ctx, q, now, q.Date); err != nil {
			return fmt.Errorf("market: record: %w", err)
		}
	}
	for _, d := range divs {
		if d.RateMicro <= 0 || d.ComDate.IsZero() {
			return fmt.Errorf("%w: a dividend needs a com date and a rate", ErrInvalid)
		}
		if err := s.repo.saveDividend(ctx, d, "manual", now); err != nil {
			return fmt.Errorf("market: record: %w", err)
		}
	}
	for _, f := range funds {
		if err := s.repo.saveFundamental(ctx, f, "manual", now); err != nil {
			return fmt.Errorf("market: record: %w", err)
		}
	}
	return nil
}

// Dividends is every cash dividend known for these symbols, by symbol,
// oldest com date first.
func (s *Service) Dividends(ctx context.Context, symbols []string) (map[string][]Dividend, error) {
	out := map[string][]Dividend{}
	if len(symbols) == 0 {
		return out, nil
	}
	ds, err := s.repo.dividends(ctx, symbols)
	if err != nil {
		return nil, fmt.Errorf("market: dividends: %w", err)
	}
	for _, d := range ds {
		out[d.Symbol] = append(out[d.Symbol], d)
	}
	return out, nil
}

// Valuations is a company's multiples on each day lastro read its
// fundamentals since from, at that day's close (or the latest before it).
func (s *Service) Valuations(ctx context.Context, symbol string, from time.Time) ([]Valuation, error) {
	fs, err := s.repo.fundamentals(ctx, symbol, from)
	if err != nil {
		return nil, fmt.Errorf("market: valuations: %w", err)
	}
	if len(fs) == 0 {
		return nil, nil
	}
	qs, err := s.repo.quotes(ctx, symbol, fs[0].Date.AddDate(0, 0, -10))
	if err != nil {
		return nil, fmt.Errorf("market: valuations: %w", err)
	}
	divs, err := s.repo.dividends(ctx, []string{symbol})
	if err != nil {
		return nil, fmt.Errorf("market: valuations: %w", err)
	}
	var out []Valuation
	i := 0
	var close int64
	for _, f := range fs {
		for i < len(qs) && !qs[i].Date.After(f.Date) {
			close = qs[i].Close.Cents()
			i++
		}
		if close == 0 {
			continue
		}
		out = append(out, ValueOn(f, close, divs))
	}
	return out, nil
}

// refreshBonds reads Tesouro Direto's file only when the titles stored are
// older than the last business day: the file is big and the source
// rate-limits.
func (s *Service) refreshBonds(ctx context.Context) (int, error) {
	stored, err := s.repo.latestBonds(ctx)
	if err != nil {
		return 0, err
	}
	if len(stored) > 0 && !stored[0].Date.Before(lastWeekday(s.today())) {
		return 0, nil
	}
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
