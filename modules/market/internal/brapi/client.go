// Package brapi reads B3 quotes from brapi.dev. A token is optional: without
// one only a few tickers answer.
package brapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultURL = "https://brapi.dev"

type Client struct {
	base  string
	token string
	http  *http.Client
}

func New(base, token string) *Client {
	return &Client{base: strings.TrimRight(base, "/"), token: token, http: &http.Client{Timeout: 30 * time.Second}}
}

// Price is a close, or the latest price for today, as brapi sends it.
type Price struct {
	Date  time.Time
	Close json.Number
}

// Quote is a ticker's latest price and, when asked for a range, its daily
// closes.
type Quote struct {
	Symbol   string
	Currency string
	Price    json.Number
	Time     time.Time
	History  []Price
}

type wire struct {
	Results []struct {
		Symbol             string      `json:"symbol"`
		Currency           string      `json:"currency"`
		RegularMarketPrice json.Number `json:"regularMarketPrice"`
		RegularMarketTime  time.Time   `json:"regularMarketTime"`
		HistoricalData     []struct {
			Date  int64       `json:"date"`
			Close json.Number `json:"close"`
		} `json:"historicalDataPrice"`
	} `json:"results"`
	Error   bool   `json:"error"`
	Message string `json:"message"`
}

// Quote asks for one ticker; rng is brapi's range ("5d", "1y"), or empty for
// only the latest price.
func (c *Client) Quote(ctx context.Context, symbol, rng string) (Quote, error) {
	q := url.Values{}
	if rng != "" {
		q.Set("range", rng)
		q.Set("interval", "1d")
	}
	if c.token != "" {
		q.Set("token", c.token)
	}
	u := c.base + "/api/quote/" + url.PathEscape(symbol)
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Quote{}, fmt.Errorf("brapi: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// The URL holds the token: never let it into the error.
		return Quote{}, fmt.Errorf("brapi: %s: request failed", symbol)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return Quote{}, fmt.Errorf("brapi: %s: status %d: %s", symbol, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var w wire
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(&w); err != nil {
		return Quote{}, fmt.Errorf("brapi: %s: %w", symbol, err)
	}
	if w.Error || len(w.Results) == 0 {
		return Quote{}, fmt.Errorf("brapi: %s: %s", symbol, w.Message)
	}
	r := w.Results[0]
	out := Quote{Symbol: r.Symbol, Currency: r.Currency, Price: r.RegularMarketPrice, Time: r.RegularMarketTime}
	for _, h := range r.HistoricalData {
		out.History = append(out.History, Price{Date: time.Unix(h.Date, 0).UTC(), Close: h.Close})
	}
	return out, nil
}

// CashDividend is one cash payment per share a company approved: dividends,
// JCP (interest on capital, before tax) or a fund's income.
type CashDividend struct {
	Label       string // "DIVIDENDO", "JCP", "RENDIMENTO"
	Rate        json.Number
	ComDate     time.Time // the last day holding the share earns it
	PaymentDate time.Time // zero when not set yet
}

// Fundamentals is what brapi says about a company: earnings and book value
// per share, the multiples it computed, and the cash dividends. A field the
// plan or the asset does not have is empty.
type Fundamentals struct {
	Symbol    string
	Price     json.Number
	EPS       json.Number // earnings per share, last 12 months
	PE        json.Number
	BookValue json.Number // per share
	PB        json.Number
	Dividends []CashDividend
}

type fundamentalsWire struct {
	Results []struct {
		Symbol               string      `json:"symbol"`
		RegularMarketPrice   json.Number `json:"regularMarketPrice"`
		PriceEarnings        json.Number `json:"priceEarnings"`
		EarningsPerShare     json.Number `json:"earningsPerShare"`
		DefaultKeyStatistics *struct {
			PriceToBook json.Number `json:"priceToBook"`
			BookValue   json.Number `json:"bookValue"`
		} `json:"defaultKeyStatistics"`
		DividendsData *struct {
			CashDividends []struct {
				Label         string      `json:"label"`
				Rate          json.Number `json:"rate"`
				LastDatePrior string      `json:"lastDatePrior"`
				PaymentDate   string      `json:"paymentDate"`
			} `json:"cashDividends"`
		} `json:"dividendsData"`
	} `json:"results"`
	Error   bool   `json:"error"`
	Message string `json:"message"`
}

// Fundamentals asks for a ticker's fundamentals and dividends. The key
// statistics module needs a paid plan: when brapi refuses it, the request
// is made again without it, and then without the dividends.
func (c *Client) Fundamentals(ctx context.Context, symbol string) (Fundamentals, error) {
	var last error
	for _, extra := range []url.Values{
		{"fundamental": {"true"}, "dividends": {"true"}, "modules": {"defaultKeyStatistics"}},
		{"fundamental": {"true"}, "dividends": {"true"}},
		{"fundamental": {"true"}},
	} {
		var w fundamentalsWire
		if last = c.get(ctx, symbol, extra, &w); last != nil {
			continue
		}
		if w.Error || len(w.Results) == 0 {
			last = fmt.Errorf("brapi: %s: %s", symbol, w.Message)
			continue
		}
		r := w.Results[0]
		out := Fundamentals{Symbol: r.Symbol, Price: r.RegularMarketPrice, EPS: r.EarningsPerShare, PE: r.PriceEarnings}
		if k := r.DefaultKeyStatistics; k != nil {
			out.BookValue, out.PB = k.BookValue, k.PriceToBook
		}
		if d := r.DividendsData; d != nil {
			for _, x := range d.CashDividends {
				com, err := day(x.LastDatePrior)
				if err != nil || com.IsZero() || x.Rate == "" {
					continue // without a com date or a rate it cannot be counted
				}
				pay, _ := day(x.PaymentDate)
				out.Dividends = append(out.Dividends, CashDividend{Label: strings.ToUpper(strings.TrimSpace(x.Label)), Rate: x.Rate, ComDate: com, PaymentDate: pay})
			}
		}
		return out, nil
	}
	return Fundamentals{}, last
}

// get reads /api/quote/{symbol} with extra parameters into v.
func (c *Client) get(ctx context.Context, symbol string, extra url.Values, v any) error {
	q := url.Values{}
	for k, vs := range extra {
		q[k] = vs
	}
	if c.token != "" {
		q.Set("token", c.token)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/quote/"+url.PathEscape(symbol)+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("brapi: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("brapi: %s: request failed", symbol)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("brapi: %s: status %d: %s", symbol, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("brapi: %s: %w", symbol, err)
	}
	return nil
}

// day reads brapi's dates ("2024-08-21T00:00:00.000Z", sometimes a plain
// day, sometimes empty) as a calendar day.
func day(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if len(s) >= 10 {
		if t, err := time.Parse(time.DateOnly, s[:10]); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("brapi: date %q", s)
}
