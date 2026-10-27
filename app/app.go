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

	"lastro/modules/categorize"
	"lastro/modules/ingest"
	"lastro/modules/ledger"
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

	auth func(http.Handler) http.Handler

	// One field per module, wired in New in dependency order.
	Ledger     *ledger.Service
	Ingest     *ingest.Service
	Categorize *categorize.Service
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
	a.Ingest = ingest.New(ingest.NewRepository(pool), cfg.Pluggy, a.Ledger, a.Bus, logger)
	a.Categorize = categorize.New(categorize.NewRepository(pool))
	// M2 on: subscriptions, e.g. events.Subscribe(a.Bus, a.Categorize.OnTransactionsImported)

	return a, nil
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
	a.Ledger.RegisterRoutes(api, a.Logger)
	a.Ingest.RegisterRoutes(api, a.Logger)
	a.Categorize.RegisterRoutes(api, a.Logger)

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", httpx.Healthz())
	mux.Handle("GET /readyz", httpx.Readyz(a.DB.Ping))
	mux.Handle("/api/", a.auth(api))
	mux.Handle("/", a.auth(web.Handler()))

	return httpx.Recover(a.Logger, mux)
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFrom(r.Context())
	httpx.JSON(w, http.StatusOK, map[string]string{"email": id.Email})
}
