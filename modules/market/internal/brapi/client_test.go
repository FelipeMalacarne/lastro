package brapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQuote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/quote/XXXX3" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":true,"message":"Não encontramos a ação XXXX3"}`))
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
}
