package brapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQuote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/quote/XXXX3" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":true,"message":"Não encontramos a ação XXXX3"}`))
			return
		}
		if r.URL.Query().Get("range") == "2y" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":true,"message":"O range \"2y\" não está disponível no seu plano.","code":"INVALID_RANGE",
				"details":{"limit":{"current":["1d","5d","1mo","3mo"],"requested":"2y"}}}`))
			return
		}
		if r.URL.Path != "/api/quote/PETR4" || r.URL.Query().Get("range") != "5d" || r.URL.Query().Get("token") != "secret" {
			t.Errorf("unexpected %s", r.URL)
		}
		_, _ = w.Write([]byte(`{"results":[{"symbol":"PETR4","currency":"BRL","regularMarketPrice":53.88,"regularMarketTime":"2026-10-27T03:05:51.000Z",
			"historicalDataPrice":[{"date":1792638000,"close":57.83},{"date":1792983600,"close":53.88}]}]}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "secret")
	q, err := c.Quote(context.Background(), "PETR4", "5d")
	if err != nil || q.Price.String() != "53.88" || len(q.History) != 2 || q.History[0].Close.String() != "57.83" {
		t.Fatalf("got %+v, %v", q, err)
	}
	if _, err := c.Quote(context.Background(), "XXXX3", ""); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("an unknown ticker fails, without the token: %v", err)
	}
	if _, err := c.Quote(context.Background(), "PETR4", "2y"); !errors.Is(err, ErrRange) {
		t.Fatalf("a range the plan refuses is ErrRange: %v", err)
	}
}

func TestFundamentals(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.RawQuery)
		if r.URL.Query().Get("modules") != "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":true,"message":"O seu plano não permite o módulo"}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"symbol":"WEGE3","regularMarketPrice":38.2,"priceEarnings":28.4,"earningsPerShare":1.345,
			"dividendsData":{"cashDividends":[
				{"label":"JCP","rate":0.157843,"lastDatePrior":"2026-09-30T00:00:00.000Z","paymentDate":"2026-12-12T00:00:00.000Z"},
				{"label":"DIVIDENDO","rate":0.08,"lastDatePrior":"2026-03-02T00:00:00.000Z","paymentDate":null},
				{"label":"DIVIDENDO","rate":0.1,"lastDatePrior":"","paymentDate":""}]}}]}`))
	}))
	defer srv.Close()
	f, err := New(srv.URL, "").Fundamentals(context.Background(), "WEGE3")
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 2 {
		t.Errorf("without the module after a refusal: %v", asked)
	}
	if f.PE.String() != "28.4" || f.EPS.String() != "1.345" || f.PB != "" || len(f.Dividends) != 2 {
		t.Fatalf("got %+v", f)
	}
	d := f.Dividends[0]
	if d.Label != "JCP" || d.Rate.String() != "0.157843" || d.ComDate.Format(time.DateOnly) != "2026-09-30" || d.PaymentDate.Format(time.DateOnly) != "2026-12-12" {
		t.Errorf("JCP: %+v", d)
	}
	if !f.Dividends[1].PaymentDate.IsZero() {
		t.Errorf("no payment date yet: %+v", f.Dividends[1])
	}
}
