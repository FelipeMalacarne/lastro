package pluggy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakePluggy is a minimal Pluggy API: POST /auth plus whatever routes a test adds.
type fakePluggy struct {
	*http.ServeMux
	auths atomic.Int32
	key   atomic.Value // current valid API key
}

func newFake(t *testing.T) (*fakePluggy, *Client) {
	t.Helper()
	f := &fakePluggy{ServeMux: http.NewServeMux()}
	f.key.Store("key-1")
	f.HandleFunc("POST /auth", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["clientId"] != "id" || body["clientSecret"] != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":401,"codeDescription":"CLIENT_KEYS_UNAUTHORIZED","message":"Client keys are invalid"}`))
			return
		}
		f.auths.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{"apiKey": f.key.Load().(string)})
	})
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c := New("id", "secret", WithBaseURL(srv.URL))
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return f, c
}

// authorized wraps a handler so it answers 401 unless the current key is sent.
func (f *fakePluggy) authorized(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != f.key.Load().(string) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":401,"message":"Unauthorized"}`))
			return
		}
		h(w, r)
	}
}

func TestReusesTheAPIKey(t *testing.T) {
	f, c := newFake(t)
	f.HandleFunc("GET /accounts", f.authorized(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":"a1","type":"BANK","balance":120.5}]}`))
	}))
	for range 3 {
		if _, err := c.Accounts(context.Background(), "item"); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.auths.Load(); n != 1 {
		t.Fatalf("POST /auth called %d times, want 1", n)
	}
}

func TestRenewsAnExpiredKeyOnce(t *testing.T) {
	f, c := newFake(t)
	f.HandleFunc("GET /items/{id}", f.authorized(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"` + r.PathValue("id") + `","status":"UPDATED","connector":{"name":"Nubank","isOpenFinance":true}}`))
	}))
	if _, err := c.Item(context.Background(), "i1"); err != nil {
		t.Fatal(err)
	}
	f.key.Store("key-2") // Pluggy revoked the first key
	it, err := c.Item(context.Background(), "i1")
	if err != nil {
		t.Fatal(err)
	}
	if it.ID != "i1" || it.Connector.Name != "Nubank" || !it.Connector.IsOpenFinance {
		t.Fatalf("unexpected item: %+v", it)
	}
	if n := f.auths.Load(); n != 2 {
		t.Fatalf("POST /auth called %d times, want 2", n)
	}
}

func TestBadCredentials(t *testing.T) {
	f, _ := newFake(t)
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := New("id", "wrong", WithBaseURL(srv.URL))
	_, err := c.Item(context.Background(), "i1")
	var pe *Error
	if !errors.As(err, &pe) || pe.Status != http.StatusUnauthorized || pe.Code != "CLIENT_KEYS_UNAUTHORIZED" {
		t.Fatalf("want a 401 CLIENT_KEYS_UNAUTHORIZED, got %v", err)
	}
	if strings.Contains(err.Error(), "wrong") {
		t.Fatal("the error must not echo the secret")
	}
}

func TestTransactionsFollowTheCursor(t *testing.T) {
	f, c := newFake(t)
	f.HandleFunc("GET /v2/transactions", f.authorized(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("accountId") != "acc" || q.Get("dateFrom") != "2026-07-01" {
			t.Errorf("unexpected query %q", r.URL.RawQuery)
		}
		switch q.Get("after") {
		case "":
			_, _ = w.Write([]byte(`{"results":[{"id":"t1","amount":-10.5,"type":"DEBIT"},{"id":"t2","amount":3}],"next":"?accountId=acc&dateFrom=2026-07-01&after=c1"}`))
		case "c1":
			_, _ = w.Write([]byte(`{"results":[{"id":"t3","amount":0.1,"creditCardMetadata":{"installmentNumber":3,"totalInstallments":10}}],"next":null}`))
		default:
			t.Errorf("unexpected cursor %q", q.Get("after"))
		}
	}))
	txs, err := c.Transactions(context.Background(), "acc", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 3 || txs[0].ID != "t1" || txs[2].ID != "t3" {
		t.Fatalf("got %+v", txs)
	}
	if txs[0].Amount.String() != "-10.5" || txs[2].Amount.String() != "0.1" {
		t.Fatalf("amounts must keep the literal: %q %q", txs[0].Amount, txs[2].Amount)
	}
	if m := txs[2].CreditCardMetadata; m == nil || m.InstallmentNumber != 3 || m.TotalInstallments != 10 {
		t.Fatalf("installment not decoded: %+v", m)
	}
	if !strings.Contains(string(txs[1].Raw), `"t2"`) {
		t.Fatalf("Raw must keep the record: %s", txs[1].Raw)
	}
}

func TestInvestmentsReadEveryPage(t *testing.T) {
	f, c := newFake(t)
	f.HandleFunc("GET /investments", f.authorized(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "1":
			_, _ = w.Write([]byte(`{"page":1,"totalPages":2,"results":[{"id":"i1","type":"FIXED_INCOME","subtype":"CDB"}]}`))
		case "2":
			_, _ = w.Write([]byte(`{"page":2,"totalPages":2,"results":[{"id":"i2","type":"EQUITY","subtype":"STOCK"}]}`))
		}
	}))
	invs, err := c.Investments(context.Background(), "item")
	if err != nil {
		t.Fatal(err)
	}
	if len(invs) != 2 || invs[1].Subtype != "STOCK" {
		t.Fatalf("got %+v", invs)
	}
}

func TestInvestmentTransactionsReadEveryPage(t *testing.T) {
	f, c := newFake(t)
	f.HandleFunc("GET /investments/{id}/transactions", f.authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "i1" {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Query().Get("page") {
		case "1":
			_, _ = w.Write([]byte(`{"page":1,"totalPages":2,"results":[{"id":"t1","type":"BUY","quantity":10,"value":12.5,"amount":125,"date":"2025-03-10T00:00:00.000Z","tradeDate":"2025-03-12T00:00:00.000Z"}]}`))
		case "2":
			_, _ = w.Write([]byte(`{"page":2,"totalPages":2,"results":[{"id":"t2","type":"SELL","quantity":4,"value":15,"amount":60,"netAmount":59.2,"date":"2026-01-05T00:00:00.000Z","tradeDate":"2026-01-07T00:00:00.000Z"}]}`))
		}
	}))
	txs, err := c.InvestmentTransactions(context.Background(), "i1")
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 2 || txs[0].Type != "BUY" || txs[1].NetAmount.String() != "59.2" || txs[1].TradeDate[:10] != "2026-01-07" || len(txs[1].Raw) == 0 {
		t.Fatalf("got %+v", txs)
	}
}

func TestBillsReadEveryPage(t *testing.T) {
	f, c := newFake(t)
	f.HandleFunc("GET /bills", f.authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("accountId") != "card" {
			t.Errorf("unexpected query %q", r.URL.RawQuery)
		}
		switch r.URL.Query().Get("page") {
		case "1":
			_, _ = w.Write([]byte(`{"page":1,"totalPages":2,"results":[{"id":"b1","dueDate":"2026-09-05T00:00:00.000Z","totalAmount":981.75,"totalAmountCurrencyCode":"BRL","minimumPaymentAmount":null,"financeCharges":[]}]}`))
		case "2":
			_, _ = w.Write([]byte(`{"page":2,"totalPages":2,"results":[{"id":"b2","dueDate":"2026-10-05T00:00:00.000Z","totalAmount":1203.4,"minimumPaymentAmount":180.51,"financeCharges":[{"type":"LATE_PAYMENT_INTEREST","amount":18.4}]}]}`))
		}
	}))
	bills, err := c.Bills(context.Background(), "card")
	if err != nil {
		t.Fatal(err)
	}
	if len(bills) != 2 || bills[0].MinimumPaymentAmount != "" || bills[1].TotalAmount.String() != "1203.4" ||
		len(bills[1].FinanceCharges) != 1 || bills[1].FinanceCharges[0].Amount.String() != "18.4" || len(bills[1].Raw) == 0 {
		t.Fatalf("got %+v", bills)
	}
}

func TestWaitsOutRateLimits(t *testing.T) {
	f, c := newFake(t)
	var calls atomic.Int32
	var waited []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error { waited = append(waited, d); return nil }
	f.HandleFunc("GET /accounts", f.authorized(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("RateLimit-Reset", "7")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	if _, err := c.Accounts(context.Background(), "item"); err != nil {
		t.Fatal(err)
	}
	if len(waited) != 1 || waited[0] != 7*time.Second {
		t.Fatalf("waited %v, want [7s]", waited)
	}
}

func TestGivesUpAfterRepeatedRateLimits(t *testing.T) {
	f, c := newFake(t)
	f.HandleFunc("GET /accounts", f.authorized(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"code":429,"message":"Too many requests"}`))
	}))
	_, err := c.Accounts(context.Background(), "item")
	var pe *Error
	if !errors.As(err, &pe) || pe.Status != http.StatusTooManyRequests {
		t.Fatalf("want a 429 error, got %v", err)
	}
}
