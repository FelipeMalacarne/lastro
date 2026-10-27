// Package tesouro reads Tesouro Direto's daily rates and prices from
// Tesouro Transparente's open data (a semicolon CSV of every day since 2002).
package tesouro

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultURL = "https://www.tesourotransparente.gov.br/ckan/dataset/df56aa42-484a-4a59-8184-7676580c81e3/resource/796d2059-14e9-44e3-80c9-2d9e30b405c1/download/PrecoTaxaTesouroDireto.csv"

type Client struct {
	url  string
	http *http.Client
}

func New(url string) *Client { return &Client{url: url, http: &http.Client{Timeout: 2 * time.Minute}} }

// Row is one title on one day; rates and prices as text with a comma.
type Row struct {
	Name     string
	Maturity time.Time
	Date     time.Time
	BuyRate  string // % a year
	BuyPrice string // R$
}

// Latest reads the file and keeps only its most recent day.
func (c *Client) Latest(ctx context.Context) ([]Row, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, fmt.Errorf("tesouro: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tesouro: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tesouro: status %d", resp.StatusCode)
	}
	return parse(resp.Body)
}

// parse keeps the rows of the latest "Data Base". Columns: Tipo Titulo;
// Data Vencimento; Data Base; Taxa Compra Manha; Taxa Venda Manha; PU Compra
// Manha; PU Venda Manha; PU Base Manha.
func parse(r io.Reader) ([]Row, error) {
	cr := csv.NewReader(r)
	cr.Comma = ';'
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("tesouro: header: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))] = i
	}
	for _, need := range []string{"Tipo Titulo", "Data Vencimento", "Data Base", "Taxa Compra Manha", "PU Compra Manha"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("tesouro: no %q column", need)
		}
	}
	var latest time.Time
	var out []Row
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tesouro: %w", err)
		}
		day, err := time.Parse("02/01/2006", rec[col["Data Base"]])
		if err != nil || day.Before(latest) {
			continue
		}
		mat, err := time.Parse("02/01/2006", rec[col["Data Vencimento"]])
		if err != nil {
			continue
		}
		if day.After(latest) {
			latest, out = day, out[:0]
		}
		out = append(out, Row{
			Name: strings.TrimSpace(rec[col["Tipo Titulo"]]), Maturity: mat, Date: day,
			BuyRate: rec[col["Taxa Compra Manha"]], BuyPrice: rec[col["PU Compra Manha"]],
		})
	}
	return out, nil
}
