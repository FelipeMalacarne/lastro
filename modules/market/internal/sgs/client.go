// Package sgs reads series from Banco Central's SGS (Sistema Gerenciador de
// Séries Temporais). No key is needed.
package sgs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Series codes.
const (
	CDI  = 12  // CDI, % a day, business days
	IPCA = 433 // IPCA, % a month
)

const DefaultURL = "https://api.bcb.gov.br"

type Client struct {
	base string
	http *http.Client
}

func New(base string) *Client {
	return &Client{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 30 * time.Second}}
}

// Value is one published value: the day and the rate in percent, as text.
type Value struct {
	Date  time.Time
	Value string
}

type wire struct {
	Data  string `json:"data"`
	Valor string `json:"valor"`
}

// failure is what SGS sends instead of the list, sometimes with a 200.
type failure struct {
	Erro struct {
		StatusCode int    `json:"statusCode"`
		Detail     string `json:"detail"`
	} `json:"erro"`
}

// Series reads a series between two days, both included. SGS answers 404
// when there is nothing in the range, which is an empty answer here; it
// also sends that 404 as an error object with a 200.
func (c *Client) Series(ctx context.Context, code int, from, to time.Time) ([]Value, error) {
	q := url.Values{"formato": {"json"}, "dataInicial": {from.Format("02/01/2006")}, "dataFinal": {to.Format("02/01/2006")}}
	u := fmt.Sprintf("%s/dados/serie/bcdata.sgs.%d/dados?%s", c.base, code, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("sgs: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sgs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("sgs: series %d: status %d: %s", code, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sgs: series %d: %w", code, err)
	}
	if b := bytes.TrimSpace(body); len(b) > 0 && b[0] == '{' {
		var f failure
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("sgs: series %d: %w", code, err)
		}
		if f.Erro.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("sgs: series %d: status %d: %s", code, f.Erro.StatusCode, f.Erro.Detail)
	}
	var rows []wire
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("sgs: series %d: %w", code, err)
	}
	out := make([]Value, 0, len(rows))
	for _, r := range rows {
		d, err := time.Parse("02/01/2006", r.Data)
		if err != nil {
			return nil, fmt.Errorf("sgs: series %d: date %q", code, r.Data)
		}
		out = append(out, Value{Date: d, Value: r.Valor})
	}
	return out, nil
}
