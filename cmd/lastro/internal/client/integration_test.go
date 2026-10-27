//go:build integration

package client_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lastro/app"
	"lastro/cmd/lastro/internal/client"
	"lastro/modules/portfolio"
	"lastro/money"
	"lastro/platform/config"
	"lastro/platform/db/dbtest"
)

// The client against the real handler, auth off as in dev: if a field the
// client reads is renamed on the server, this fails.
func TestClientAgainstTheServer(t *testing.T) {
	ctx := context.Background()
	a, err := app.New(ctx, config.Config{Env: "dev", DatabaseURL: dbtest.NewURL(t), Auth: config.Auth{Disabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	a.Version = "integration"
	err = a.Portfolio.SetTargets(ctx, map[portfolio.Class]money.Percent{
		portfolio.ClassCDI: money.FromBps(4000), portfolio.ClassIPCA: money.FromBps(3000),
		portfolio.ClassBR: money.FromBps(1000), portfolio.ClassGlobal: money.FromBps(2000),
	}, 500)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	l, err := client.StartLogin(ctx, srv.URL)
	if err != nil || l.Device != nil {
		t.Fatalf("connect to a dev server: %+v, %v", l, err)
	}
	cfg, err := l.Finish(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c := client.New(ctx, cfg, nil)

	me, err := c.Me(ctx)
	if err != nil || me.Email != "dev@localhost" || me.Permission != "write" || me.Version != "integration" {
		t.Fatalf("me: %+v, %v", me, err)
	}

	st, err := c.SyncStatus(ctx)
	if err != nil || st.Running {
		t.Fatalf("sync status: %+v, %v", st, err)
	}
	// No Pluggy credentials here: the server says so.
	_, err = c.StartSync(ctx)
	var ae *client.APIError
	if !errors.As(err, &ae) || ae.Status != http.StatusServiceUnavailable || !strings.Contains(ae.Message, "Pluggy") {
		t.Fatalf("sync without Pluggy: %v", err)
	}

	b, err := c.Budget(ctx)
	if err != nil || len(b.Month) != len("2026-10") {
		t.Fatalf("budget: %+v, %v", b, err)
	}

	p, err := c.Contribute(ctx, 300000)
	if err != nil {
		t.Fatal(err)
	}
	if p.AmountCents != 300000 || len(p.Splits) != len(portfolio.Classes) {
		t.Fatalf("plan: %+v", p)
	}
	for _, s := range p.Splits {
		if s.Class == "" || s.Why == "" {
			t.Fatalf("split without class or reason: %+v", s)
		}
	}
}
