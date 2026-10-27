// Package app is the composition root: it wires config, platform and modules.
//
// It is the only place that knows every module. Modules never assemble
// themselves or reach for globals: everything arrives through the constructor.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"

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
	"lastro/web"
)

type App struct {
	Config config.Config
	Logger *slog.Logger
	DB     *pgxpool.Pool
	Bus    *events.Bus
	LLM    *llm.Client
	// Version is the build's, for the Settings screen; the CLI sets it.
	Version string

	auth func(http.Handler) http.Handler

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
	rep, merr := a.Market.Refresh(ctx, append(held, extra...))
	// The day is valued even without new prices: the banks' values stand.
	if _, err := a.Portfolio.Valuate(ctx, a.Portfolio.Today()); err != nil {
		return rep, err
	}
	return rep, merr
}

// NewLogger uses readable text in dev and JSON in prod.
func NewLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.IsDev() {
		return slog.New(slog.NewTextHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

func newAuth(cfg config.Config, logger *slog.Logger) (func(http.Handler) http.Handler, error) {
	if cfg.Auth.Disabled {
		logger.Warn("authentication disabled: dev only")
		return auth.Disabled(), nil
	}
	v, err := auth.NewVerifier(cfg.Auth.TeamDomain, cfg.Auth.Audience)
	if err != nil {
		return nil, err
	}
	return v.Middleware(logger), nil
}

// Close releases the app resources.
func (a *App) Close() { a.DB.Close() }

// Handler builds the HTTP routes. Probes stay outside authentication because
// the kubelet calls them directly, without going through Cloudflare Access.
func (a *App) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /api/v1/me", a.handleMe)
	api.HandleFunc("GET /api/v1/settings", a.handleSettings)
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
	mux.Handle("/api/", a.auth(api))
	mux.Handle("/", a.auth(web.Handler()))

	return httpx.Recover(a.Logger, mux)
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFrom(r.Context())
	httpx.JSON(w, http.StatusOK, map[string]string{"email": id.Email, "version": a.Version, "env": a.Config.Env})
}
