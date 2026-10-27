package sgs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSeries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dados/serie/bcdata.sgs.433/dados" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path != "/dados/serie/bcdata.sgs.12/dados" || r.URL.Query().Get("dataInicial") != "21/10/2026" {
			t.Errorf("unexpected %s", r.URL)
		}
		_, _ = w.Write([]byte(`[{"data":"21/10/2026","valor":"0.050788"},{"data":"22/10/2026","valor":"0.050788"}]`))
	}))
	defer srv.Close()
	c := New(srv.URL)
	from := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	vs, err := c.Series(context.Background(), CDI, from, from.AddDate(0, 0, 5))
	if err != nil || len(vs) != 2 || vs[1].Date.Day() != 22 || vs[0].Value != "0.050788" {
		t.Fatalf("got %+v, %v", vs, err)
	}
	if vs, err := c.Series(context.Background(), IPCA, from, from); err != nil || vs != nil {
		t.Fatalf("404 is empty: %+v, %v", vs, err)
	}
}
