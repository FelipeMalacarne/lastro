// Package app is the composition root: it wires config, platform and modules.
//
// It is the only place that knows every module. Modules never assemble
// themselves or reach for globals: everything arrives through the constructor.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"lastro/modules/budget"
	"lastro/modules/categorize"
	"lastro/modules/ingest"
	"lastro/modules/ledger"
	"lastro/modules/market"
	"lastro/modules/planning"
	"lastro/modules/portfolio"
	"lastro/platform/auth"
	"lastro/platform/config"
	"lastro/platform/db"
	"lastro/platform/events"
	"lastro/platform/httpx"
	"lastro/platform/llm"
	"lastro/platform/notify"
	"lastro/web"
)

type App struct {
	Config config.Config
	Logger *slog.Logger
	DB     *pgxpool.Pool
	Bus    *events.Bus
	LLM    *llm.Client
	Notify *notify.Telegram
	// Version is the build's, for the Settings screen; the CLI sets it.
	Version string

	auth authWiring
	sync syncState

	// One field per module, wired in New in dependency order.
	Ledger     *ledger.Service
	Ingest     *ingest.Service
	Categorize *categorize.Service
	Budget     *budget.Service
	Market     *market.Service
	Portfolio  *portfolio.Service
	Planning   *planning.Service
}

// New wires the app. The caller is responsible for calling Close.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	logger := NewLogger(cfg)

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	authMW, err := newAuth(cfg, logger)
	if err != nil {
		pool.Close()
		return nil, err
	}

	a := &App{
		Config: cfg,
		Logger: logger,
		DB:     pool,
		Bus:    events.NewBus(logger),
		LLM:    llm.NewOllama(cfg.Ollama.URL, cfg.Ollama.Model),
		Notify: notify.NewTelegram(cfg.Telegram.BotToken, cfg.Telegram.ChatID),
		auth:   authMW,
	}

	a.Ledger = ledger.New(ledger.NewRepository(pool), a.Bus)
	series, quotes, bonds := market.Sources(cfg.BrapiToken)
	a.Market = market.New(market.NewRepository(pool), series, quotes, bonds, logger)
	a.Portfolio = portfolio.New(portfolio.NewRepository(pool), a.Ledger, a.Market)
	a.Planning = planning.New(a.Portfolio)
	a.Ingest = ingest.New(ingest.NewRepository(pool), cfg.Pluggy, a.Ledger, a.Portfolio, a.Bus, logger)
	a.Categorize = categorize.New(categorize.NewRepository(pool), a.Ledger, a.LLM, logger)
	a.Budget = budget.New(budget.NewRepository(pool), a.Ledger, a.Categorize)

	// The rules categorize each import as it commits. After every sync they
	// also reach whatever is still without a category: transactions imported
	// before a rule existed, or while the rules could not run.
	events.Subscribe(a.Bus, a.Categorize.OnTransactionsImported)
	events.Subscribe(a.Bus, a.categorizeAfterSync)

	return a, nil
}

func (a *App) categorizeAfterSync(ctx context.Context, _ ingest.SyncCompleted) error {
	n, err := a.Categorize.CategorizeUncategorized(ctx)
	if n > 0 {
		a.Logger.InfoContext(ctx, "categorized by rules after the sync", "transactions", n)
	}
	return err
}

// RefreshMarket brings the indices, Tesouro Direto and the quotes of every
// ticker held up to date, plus any extra symbols, and values the day again
// at the new prices.
func (a *App) RefreshMarket(ctx context.Context, extra ...string) (market.Report, error) {
	held, err := a.Portfolio.Tickers(ctx)
	if err != nil {
		return market.Report{}, err
	}
	rep, merr := a.Market.Refresh(ctx, append(append(held, portfolio.Ibovespa), extra...))
	// The day is valued even without new prices: the banks' values stand.
	if _, err := a.Portfolio.Valuate(ctx, a.Portfolio.Today()); err != nil {
		return rep, err
	}
	return rep, merr
}

// SendAlerts pushes the alerts no channel has seen yet, in one message, and
// remembers them. Without a channel it sends nothing and says how many wait.
func (a *App) SendAlerts(ctx context.Context) (int, error) {
	all, err := a.Alerts(ctx)
	if err != nil {
		return 0, err
	}
	fresh, err := a.Budget.Unsent(ctx, all)
	if err != nil || len(fresh) == 0 || !a.Notify.Enabled() {
		return 0, err
	}
	var b strings.Builder
	b.WriteString("lastro\n")
	for _, al := range fresh {
		fmt.Fprintf(&b, "\n• %s. %s", al.Title, al.Detail)
	}
	if err := a.Notify.Send(ctx, b.String()); err != nil {
		return 0, err
	}
	return len(fresh), a.Budget.MarkSent(ctx, fresh)
}

// NewLogger uses readable text in dev and JSON in prod.
func NewLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.IsDev() {
		return slog.New(slog.NewTextHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

// authWiring is what Handler needs from authentication: the guards, and the
// web login and the provider when they are configured.
type authWiring struct {
	require func(http.Handler) http.Handler
	bearer  func(resource string) func(http.Handler) http.Handler
	web     *auth.Web
	oidc    *auth.Provider
}

func newAuth(cfg config.Config, logger *slog.Logger) (authWiring, error) {
	if cfg.Auth.Disabled {
		logger.Warn("authentication disabled: dev only")
		return authWiring{require: auth.Disabled(), bearer: func(string) func(http.Handler) http.Handler { return auth.Disabled() }}, nil
	}
	a := &auth.Authenticator{Logger: logger, PublicURL: cfg.Auth.PublicURL}
	if o := cfg.Auth.OIDC; o.Enabled() {
		p, err := auth.NewProvider(auth.OIDCConfig{Issuer: o.Issuer, Audience: o.Audience, OwnerSubject: o.OwnerSubject, WriteClients: o.Writers()})
		if err != nil {
			return authWiring{}, err
		}
		a.OIDC = p
		if o.WebClientID != "" {
			sealer, err := auth.NewSealer(cfg.Auth.SessionKey)
			if err != nil {
				return authWiring{}, err
			}
			if a.Web, err = auth.NewWeb(p, auth.WebConfig{PublicURL: cfg.Auth.PublicURL, ClientID: o.WebClientID, ClientSecret: o.WebClientSecret, Scopes: o.Scopes}, sealer, logger); err != nil {
				return authWiring{}, err
			}
		}
	}
	if cfg.Auth.CloudflareAccess() {
		v, err := auth.NewVerifier(cfg.Auth.TeamDomain, cfg.Auth.Audience)
		if err != nil {
			return authWiring{}, err
		}
		a.CF = v
	}
	return authWiring{require: a.Require, bearer: a.RequireBearer, web: a.Web, oidc: a.OIDC}, nil
}

// resourceMetadata describes lastro, or one of its resources, to OAuth
// clients (RFC 9728). Only the root names the CLI's client.
func (a *App) resourceMetadata(resource string) auth.ResourceMetadata {
	m := auth.ResourceMetadata{
		Resource:             a.Config.Auth.PublicURL + resource,
		AuthorizationServers: []string{a.auth.oidc.Issuer()},
		ScopesSupported:      a.Config.Auth.OIDC.Scopes,
		ResourceName:         "lastro",
	}
	if resource == "" {
		m.CLIClientID = a.Config.Auth.OIDC.CLIClientID
	}
	return m
}

// Close releases the app resources.
func (a *App) Close() { a.DB.Close() }

// Handler builds the HTTP routes. Probes stay outside authentication because
// the kubelet calls them directly; so do the login and the metadata that tell
// a client where to sign in.
func (a *App) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /api/v1/me", a.handleMe)
	api.HandleFunc("GET /api/v1/settings", a.handleSettings)
	api.HandleFunc("GET /api/v1/alerts", a.handleAlerts)
	api.HandleFunc("GET /api/v1/report", a.handleReport)
	api.HandleFunc("GET /api/v1/sync", a.handleSync)
	api.HandleFunc("POST /api/v1/sync", a.handleSync)
	a.Ledger.RegisterRoutes(api, a.Logger)
	a.Ingest.RegisterRoutes(api, a.Logger)
	a.Categorize.RegisterRoutes(api, a.Logger)
	a.Budget.RegisterRoutes(api, a.Logger)
	a.Market.RegisterRoutes(api, a.Logger)
	a.Portfolio.RegisterRoutes(api, a.Logger)
	a.Planning.RegisterRoutes(api, a.Logger)

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", httpx.Healthz())
	mux.Handle("GET /readyz", httpx.Readyz(a.DB.Ping))
	if w := a.auth.web; w != nil {
		mux.HandleFunc("GET "+auth.LoginPath, w.Login)
		mux.HandleFunc("GET "+auth.CallbackPath, w.Callback)
		mux.HandleFunc("POST "+auth.LogoutPath, w.Logout)
	}
	if a.auth.oidc != nil {
		mux.Handle("GET "+auth.MetadataPath, auth.ProtectedResource(a.resourceMetadata("")))
		mux.Handle("GET "+auth.MetadataPath+"/mcp", auth.ProtectedResource(a.resourceMetadata("/mcp")))
	}
	mux.Handle("/api/", a.auth.require(api))
	mux.Handle("/", a.auth.require(web.Handler()))

	return httpx.Recover(a.Logger, mux)
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFrom(r.Context())
	httpx.JSON(w, http.StatusOK, map[string]string{
		"email": id.Email, "subject": id.Subject, "client": id.ClientID, "permission": id.Permission.String(),
		"version": a.Version, "env": a.Config.Env,
	})
}
