package client

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
	"time"

	"golang.org/x/oauth2"
)

// Client calls one lastro server's API.
type Client struct {
	base string
	http *http.Client
}

// APIError is an answer the server refused, with its message.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	switch e.Status {
	case http.StatusUnauthorized:
		return "the server did not accept the sign-in: run lastro connect again"
	case http.StatusForbidden:
		return "this client may not do that: " + e.Message
	}
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("the server answered %d", e.Status)
}

// New makes a client for a saved config. A refreshed token is saved through
// save, so the next command starts from it.
func New(ctx context.Context, c Config, save func(Config) error) *Client {
	hc := &http.Client{Timeout: 30 * time.Second}
	if c.Auth == nil || c.Auth.Token == nil {
		return &Client{base: c.URL, http: hc}
	}
	cfg := &oauth2.Config{
		ClientID: c.Auth.ClientID,
		Endpoint: oauth2.Endpoint{TokenURL: c.Auth.TokenURL, AuthStyle: oauth2.AuthStyleInParams},
		Scopes:   c.Auth.Scopes,
	}
	refreshCtx := context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: 15 * time.Second})
	src := &savingSource{
		src:  oauth2.ReuseTokenSource(c.Auth.Token, cfg.TokenSource(refreshCtx, c.Auth.Token)),
		last: c.Auth.Token.AccessToken,
		save: func(t *oauth2.Token) error {
			a := *c.Auth
			a.Token = t
			c.Auth = &a
			return save(c)
		},
	}
	hc.Transport = &oauth2.Transport{Source: src, Base: http.DefaultTransport}
	return &Client{base: c.URL, http: hc}
}

// savingSource saves each new token: a refresh rotates the refresh token too.
type savingSource struct {
	src  oauth2.TokenSource
	last string
	save func(*oauth2.Token) error
}

func (s *savingSource) Token() (*oauth2.Token, error) {
	t, err := s.src.Token()
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) {
			return nil, errors.New("the sign-in expired: run lastro connect again")
		}
		return nil, err
	}
	if t.AccessToken != s.last {
		s.last = t.AccessToken
		if err := s.save(t); err != nil {
			return nil, fmt.Errorf("save the refreshed token: %w", err)
		}
	}
	return t, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		e := &APIError{Status: resp.StatusCode}
		var msg struct{ Code, Message string }
		if json.Unmarshal(raw, &msg) == nil {
			e.Code, e.Message = msg.Code, msg.Message
		}
		return e
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	return nil
}

// Me is who the server sees.
type Me struct {
	Email      string `json:"email"`
	Subject    string `json:"subject"`
	Client     string `json:"client"`
	Permission string `json:"permission"`
	Version    string `json:"version"`
	Env        string `json:"env"`
}

func (c *Client) Me(ctx context.Context) (Me, error) {
	var m Me
	return m, c.do(ctx, http.MethodGet, "/api/v1/me", nil, &m)
}

// SyncStatus is the server's last or running sync.
type SyncStatus struct {
	Running  bool    `json:"running"`
	Started  *string `json:"started"`
	Finished *string `json:"finished"`
	OK       bool    `json:"ok"`
	Message  string  `json:"message"`
}

// ErrSyncRunning is a sync already going on the server.
var ErrSyncRunning = errors.New("a sync is already running")

// StartSync starts a sync on the server.
func (c *Client) StartSync(ctx context.Context) (SyncStatus, error) {
	var s SyncStatus
	err := c.do(ctx, http.MethodPost, "/api/v1/sync", nil, &s)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusConflict {
		return s, ErrSyncRunning
	}
	return s, err
}

func (c *Client) SyncStatus(ctx context.Context) (SyncStatus, error) {
	var s SyncStatus
	return s, c.do(ctx, http.MethodGet, "/api/v1/sync", nil, &s)
}

// Budget is the part of a month's budget the CLI reads.
type Budget struct {
	Month            string `json:"month"`
	PayYourselfFirst struct {
		PlannedCents int64 `json:"plannedCents"`
	} `json:"payYourselfFirst"`
}

// Budget reads this month's budget.
func (c *Client) Budget(ctx context.Context) (Budget, error) {
	var b Budget
	return b, c.do(ctx, http.MethodGet, "/api/v1/budget", nil, &b)
}

// Plan is where a contribution goes, by class.
type Plan struct {
	AmountCents int64   `json:"amountCents"`
	TotalCents  int64   `json:"totalCents"`
	Splits      []Split `json:"splits"`
}

type Split struct {
	Class       string  `json:"class"`
	AmountCents int64   `json:"amountCents"`
	Why         string  `json:"why"` // below_band, gap, target, above_band or at_target
	Buy         *string `json:"buy"`
}

// Contribute asks the planner for amountCents.
func (c *Client) Contribute(ctx context.Context, amountCents int64) (Plan, error) {
	var p Plan
	q := url.Values{"amount": {strconv.FormatInt(amountCents, 10)}}
	return p, c.do(ctx, http.MethodGet, "/api/v1/contribute?"+q.Encode(), nil, &p)
}
