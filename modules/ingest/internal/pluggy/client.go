// Package pluggy is the HTTP client for the Pluggy API. Only ingest uses it,
// and none of its types leave the ingest module.
//
// The client authenticates with the Dashboard credentials, reuses the API key
// for most of its 2-hour life, and handles the two pagination models: the
// cursor of /v2/transactions and the page numbers of /investments.
package pluggy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the only Pluggy environment; the sandbox is a connector, not a host.
const DefaultBaseURL = "https://api.pluggy.ai"

const (
	// keyLifetime is shorter than the 2 hours Pluggy grants, so a key never
	// expires in the middle of a sync.
	keyLifetime = 100 * time.Minute
	// maxBody bounds a response; a page of 500 transactions is well under 2 MB.
	maxBody = 32 << 20
	// maxRateLimitRetries is how many 429 answers one request waits out.
	maxRateLimitRetries = 3
)

// Error is a non-2xx answer from Pluggy. Code is the stable codeDescription
// (e.g. CLIENT_KEYS_UNAUTHORIZED) when Pluggy sends one.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("pluggy: HTTP %d %s: %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("pluggy: HTTP %d: %s", e.Status, e.Message)
}

// Client talks to the Pluggy API. It is safe for concurrent use.
type Client struct {
	baseURL      string
	clientID     string
	clientSecret string
	http         *http.Client
	now          func() time.Time
	sleep        func(ctx context.Context, d time.Duration) error

	mu         sync.Mutex
	apiKey     string
	keyExpires time.Time
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL points the client at another host (tests use httptest).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// New creates a client for the given Dashboard credentials.
func New(clientID, clientSecret string, opts ...Option) *Client {
	c := &Client{
		baseURL:      DefaultBaseURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		http:         &http.Client{Timeout: time.Minute},
		now:          time.Now,
		sleep:        sleepCtx,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Item returns one item (connection) with its status.
func (c *Client) Item(ctx context.Context, id string) (Item, error) {
	var raw json.RawMessage
	if err := c.get(ctx, "/items/"+url.PathEscape(id), &raw); err != nil {
		return Item{}, fmt.Errorf("pluggy: item: %w", err)
	}
	it, err := decode[Item](raw)
	if err != nil {
		return Item{}, fmt.Errorf("pluggy: item: %w", err)
	}
	it.Raw = raw
	return it, nil
}

// Accounts returns the bank and credit card accounts of an item.
func (c *Client) Accounts(ctx context.Context, itemID string) ([]Account, error) {
	var page struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := c.get(ctx, "/accounts?itemId="+url.QueryEscape(itemID), &page); err != nil {
		return nil, fmt.Errorf("pluggy: accounts: %w", err)
	}
	out := make([]Account, 0, len(page.Results))
	for _, raw := range page.Results {
		a, err := decode[Account](raw)
		if err != nil {
			return nil, fmt.Errorf("pluggy: accounts: %w", err)
		}
		a.Raw = raw
		out = append(out, a)
	}
	return out, nil
}

// Transactions returns every transaction of an account dated on or after
// from, following the cursor until the last page. A zero from returns the
// whole history Pluggy keeps (up to 12 months).
func (c *Client) Transactions(ctx context.Context, accountID string, from time.Time) ([]Transaction, error) {
	q := url.Values{"accountId": {accountID}}
	if !from.IsZero() {
		q.Set("dateFrom", from.Format(time.DateOnly))
	}
	next := "?" + q.Encode()
	var out []Transaction
	for next != "" {
		var page struct {
			Results []json.RawMessage `json:"results"`
			Next    *string           `json:"next"`
		}
		if err := c.get(ctx, "/v2/transactions"+next, &page); err != nil {
			return nil, fmt.Errorf("pluggy: transactions: %w", err)
		}
		for _, raw := range page.Results {
			t, err := decode[Transaction](raw)
			if err != nil {
				return nil, fmt.Errorf("pluggy: transactions: %w", err)
			}
			t.Raw = raw
			out = append(out, t)
		}
		next = ""
		if page.Next != nil {
			// next is a ready-made query string; it already carries the filters.
			next = *page.Next
			if !strings.HasPrefix(next, "?") {
				next = "?" + next
			}
		}
	}
	return out, nil
}

// Investments returns every investment of an item, reading all pages.
func (c *Client) Investments(ctx context.Context, itemID string) ([]Investment, error) {
	var out []Investment
	for page := 1; ; page++ {
		var resp struct {
			TotalPages int               `json:"totalPages"`
			Results    []json.RawMessage `json:"results"`
		}
		q := url.Values{"itemId": {itemID}, "page": {strconv.Itoa(page)}, "pageSize": {"500"}}
		if err := c.get(ctx, "/investments?"+q.Encode(), &resp); err != nil {
			return nil, fmt.Errorf("pluggy: investments: %w", err)
		}
		for _, raw := range resp.Results {
			inv, err := decode[Investment](raw)
			if err != nil {
				return nil, fmt.Errorf("pluggy: investments: %w", err)
			}
			inv.Raw = raw
			out = append(out, inv)
		}
		if page >= resp.TotalPages {
			return out, nil
		}
	}
}

// get calls a read endpoint and decodes the JSON body into out. It renews the
// API key once on 401 and waits out up to maxRateLimitRetries 429 answers.
func (c *Client) get(ctx context.Context, path string, out any) error {
	renewed, waited := false, 0
	for {
		key, err := c.key(ctx)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-API-KEY", key)
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			defer func() { _ = resp.Body.Close() }()
			if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}
			return nil
		case resp.StatusCode == http.StatusUnauthorized && !renewed:
			drain(resp)
			c.dropKey()
			renewed = true
		case resp.StatusCode == http.StatusTooManyRequests && waited < maxRateLimitRetries:
			wait := retryAfter(resp.Header)
			drain(resp)
			waited++
			if err := c.sleep(ctx, wait); err != nil {
				return err
			}
		default:
			return readError(resp)
		}
	}
}

// key returns a valid API key, asking POST /auth for a new one when needed.
func (c *Client) key(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.apiKey != "" && c.now().Before(c.keyExpires) {
		return c.apiKey, nil
	}
	if c.clientID == "" || c.clientSecret == "" {
		return "", errors.New("pluggy: missing client id or secret")
	}
	body, err := json.Marshal(map[string]string{"clientId": c.clientID, "clientSecret": c.clientSecret})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("pluggy: auth: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pluggy: auth: %w", readError(resp))
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		APIKey string `json:"apiKey"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&out); err != nil || out.APIKey == "" {
		return "", errors.New("pluggy: auth: response without an API key")
	}
	c.apiKey, c.keyExpires = out.APIKey, c.now().Add(keyLifetime)
	return c.apiKey, nil
}

func (c *Client) dropKey() {
	c.mu.Lock()
	c.apiKey = ""
	c.mu.Unlock()
}

// readError turns a non-2xx response into an *Error and closes the body.
func readError(resp *http.Response) error {
	defer func() { _ = resp.Body.Close() }()
	e := &Error{Status: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
	var body struct {
		CodeDescription string `json:"codeDescription"`
		Message         string `json:"message"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body); err == nil {
		e.Code = body.CodeDescription
		if body.Message != "" {
			e.Message = body.Message
		}
	}
	return e
}

// retryAfter reads how long to wait after a 429: RateLimit-Reset when
// present, otherwise Retry-After, capped at a minute.
func retryAfter(h http.Header) time.Duration {
	for _, name := range []string{"RateLimit-Reset", "Retry-After"} {
		if s, err := strconv.Atoi(strings.TrimSpace(h.Get(name))); err == nil && s >= 0 {
			return min(time.Duration(s)*time.Second, time.Minute)
		}
	}
	return time.Minute
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	err := json.Unmarshal(raw, &v)
	return v, err
}
