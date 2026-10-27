package portfolio

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"lastro/modules/market"
)

// DividendsView is the Dividends panel (F8): what the shares held earned,
// by year and by ticker.
type DividendsView struct {
	Since        *string             `json:"since"` // the first day lastro knew the holdings
	YearCents    int64               `json:"yearCents"`
	ValueCents   int64               `json:"valueCents"` // what the shares are worth today
	Years        []DividendYearView  `json:"years"`
	Tickers      []DividendStockView `json:"tickers"`
	ForwardCents int64               `json:"forwardCents"` // the last 12 months per share at today's quantities
	Upcoming     []PaymentView       `json:"upcoming"`     // earned, not paid yet
	Missing      []string            `json:"missing"`      // held, with no dividend data from the source
}

type DividendYearView struct {
	Year  int   `json:"year"`
	Cents int64 `json:"cents"`
}

type DividendStockView struct {
	Ticker        string       `json:"ticker"`
	YearCents     int64        `json:"yearCents"`
	TrailingMicro int64        `json:"trailingMicro"` // per share, last 12 months
	YieldBps      int64        `json:"yieldBps"`
	Next          *PaymentView `json:"next"`
}

type PaymentView struct {
	Ticker      string  `json:"ticker"`
	Label       string  `json:"label"`
	ComDate     string  `json:"comDate"`
	PaymentDate *string `json:"paymentDate"`
	RateMicro   int64   `json:"rateMicro"`
	Cents       int64   `json:"cents"`
}

func paymentView(p Payment) PaymentView {
	v := PaymentView{Ticker: p.Ticker, Label: p.Label, ComDate: day(p.ComDate), RateMicro: p.RateMicro, Cents: p.Cents}
	if !p.PaymentDate.IsZero() {
		v.PaymentDate = ptr(day(p.PaymentDate))
	}
	return v
}

// Dividends counts what the tickers held paid on the shares held on each com
// date, from the source's payments per share.
func (s *Service) Dividends(ctx context.Context) (DividendsView, error) {
	h, err := s.repo.held(ctx)
	if err != nil {
		return DividendsView{}, fmt.Errorf("portfolio: dividends: %w", err)
	}
	history := NewHeldHistory(h)
	ps, err := s.repo.positions(ctx)
	if err != nil {
		return DividendsView{}, fmt.Errorf("portfolio: dividends: %w", err)
	}
	today := s.Today()
	current := heldOf(ps, today)
	tickers := map[string]bool{}
	for _, x := range h {
		tickers[x.Ticker] = true
	}
	for _, x := range current {
		tickers[x.Ticker] = true
	}
	var list []string
	for t := range tickers {
		list = append(list, t)
	}
	sort.Strings(list)
	divs, err := s.market.Dividends(ctx, list)
	if err != nil {
		return DividendsView{}, err
	}
	quotes, err := s.market.LatestQuotes(ctx, list)
	if err != nil {
		return DividendsView{}, err
	}

	v := DividendsView{Years: []DividendYearView{}, Tickers: []DividendStockView{}, Upcoming: []PaymentView{}, Missing: []string{}}
	if since := history.Since(); !since.IsZero() {
		v.Since = ptr(day(since))
	}
	years := map[int]int64{}
	byTicker := map[string]*DividendStockView{}
	for _, p := range Earned(divs, history, today) {
		if p.Day().After(today) {
			v.Upcoming = append(v.Upcoming, paymentView(p))
			continue
		}
		years[p.Day().Year()] += p.Cents
		if p.Day().Year() == today.Year() {
			v.YearCents += p.Cents
			stockOf(byTicker, p.Ticker).YearCents += p.Cents
		}
	}
	for y, c := range years {
		v.Years = append(v.Years, DividendYearView{Year: y, Cents: c})
	}
	sort.Slice(v.Years, func(i, j int) bool { return v.Years[i].Year < v.Years[j].Year })

	for _, x := range current {
		ds := divs[x.Ticker]
		q, ok := quotes[x.Ticker]
		if ok {
			v.ValueCents += x.Quantity.Of(q.Close).Cents()
		}
		if len(ds) == 0 {
			v.Missing = append(v.Missing, x.Ticker)
			continue
		}
		st := stockOf(byTicker, x.Ticker)
		st.TrailingMicro = market.TrailingPerShare(ds, today)
		if ok {
			st.YieldBps = market.YieldBps(st.TrailingMicro, q.Close.Cents())
		}
		v.ForwardCents += perShare(st.TrailingMicro, x.Quantity)
		if n := nextPayment(ds, today, x.Quantity); n != nil {
			n.Ticker = x.Ticker
			st.Next = n
		}
	}
	for _, st := range byTicker {
		v.Tickers = append(v.Tickers, *st)
	}
	sort.Slice(v.Tickers, func(i, j int) bool {
		if v.Tickers[i].YearCents != v.Tickers[j].YearCents {
			return v.Tickers[i].YearCents > v.Tickers[j].YearCents
		}
		return v.Tickers[i].Ticker < v.Tickers[j].Ticker
	})
	return v, nil
}

func stockOf(m map[string]*DividendStockView, t string) *DividendStockView {
	if m[t] == nil {
		m[t] = &DividendStockView{Ticker: t}
	}
	return m[t]
}

// nextPayment is the first payment still to come: approved with a com date
// today or later, or earned and paid after today.
func nextPayment(ds []market.Dividend, today time.Time, q Quantity) *PaymentView {
	for _, d := range ds {
		if !d.ComDate.Before(today) || d.PaymentDate.After(today) {
			p := paymentView(Payment{Label: d.Label, ComDate: d.ComDate, PaymentDate: d.PaymentDate, RateMicro: d.RateMicro, Cents: perShare(d.RateMicro, q)})
			return &p
		}
	}
	return nil
}

// PriceDrop is a held ticker that fell at least its alert's threshold on
// its latest close.
type PriceDrop struct {
	Ticker       string
	Date         time.Time
	ChangeBps    int64
	CloseCents   int64
	ThresholdBps int64
	Thesis       string // the stock's thesis, when written
}

// PriceAlerts is each ticker's threshold, in basis points of a fall in a day.
func (s *Service) PriceAlerts(ctx context.Context) (map[string]int64, error) {
	a, err := s.repo.priceAlerts(ctx)
	if err != nil {
		return nil, fmt.Errorf("portfolio: price alerts: %w", err)
	}
	return a, nil
}

// SetPriceAlert sets the daily fall that raises an alert for a ticker.
func (s *Service) SetPriceAlert(ctx context.Context, ticker string, bps int64) error {
	t, err := NewTicker(ticker)
	if err != nil {
		return err
	}
	if bps, err = NewDropThreshold(bps); err != nil {
		return err
	}
	if err := s.repo.savePriceAlert(ctx, t, bps, s.now()); err != nil {
		return fmt.Errorf("portfolio: price alert: %w", err)
	}
	return nil
}

// DeletePriceAlert stops watching a ticker.
func (s *Service) DeletePriceAlert(ctx context.Context, ticker string) error {
	t, err := NewTicker(ticker)
	if err != nil {
		return err
	}
	if err := s.repo.deletePriceAlert(ctx, t); err != nil {
		return fmt.Errorf("portfolio: price alert: %w", err)
	}
	return nil
}

// PriceDrops are the alerts that fired on the latest close of the last few
// days, for tickers still held.
func (s *Service) PriceDrops(ctx context.Context) ([]PriceDrop, error) {
	alerts, err := s.repo.priceAlerts(ctx)
	if err != nil || len(alerts) == 0 {
		return nil, err
	}
	held, err := s.Tickers(ctx)
	if err != nil {
		return nil, err
	}
	theses, err := s.repo.theses(ctx)
	if err != nil {
		return nil, fmt.Errorf("portfolio: price drops: %w", err)
	}
	thesis := map[string]string{}
	for _, t := range theses {
		thesis[t.Ticker.String()] = t.Thesis
	}
	today := s.Today()
	var out []PriceDrop
	for _, t := range held {
		bps, ok := alerts[t]
		if !ok {
			continue
		}
		qs, err := s.market.Quotes(ctx, t, today.AddDate(0, 0, -10))
		if err != nil {
			return nil, err
		}
		if len(qs) < 2 {
			continue
		}
		prev, last := qs[len(qs)-2], qs[len(qs)-1]
		if last.Date.Before(today.AddDate(0, 0, -4)) {
			continue // stale: no close in the last few days
		}
		if change, hit := Dropped(prev.Close.Cents(), last.Close.Cents(), bps); hit {
			out = append(out, PriceDrop{Ticker: t, Date: last.Date, ChangeBps: change, CloseCents: last.Close.Cents(), ThresholdBps: bps, Thesis: strings.TrimSpace(thesis[t])})
		}
	}
	return out, nil
}
