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
