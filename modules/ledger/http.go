package ledger

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"lastro/platform/httpx"
)

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// RegisterRoutes adds the ledger's read endpoints to the authenticated API.
func (s *Service) RegisterRoutes(mux *http.ServeMux, logger *slog.Logger) {
	mux.HandleFunc("GET /api/v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		accounts, err := s.Accounts(r.Context())
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"accounts": accounts})
	})

	// GET /api/v1/transactions?month=2026-10&account=<id>&flow=in|out|transfer&q=ifood
	mux.HandleFunc("GET /api/v1/transactions", func(w http.ResponseWriter, r *http.Request) {
		f, err := parseFilter(r)
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		list, err := s.Transactions(r.Context(), f)
		if errors.Is(err, ErrInvalid) {
			err = httpx.BadRequest(err.Error())
		}
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		httpx.JSON(w, http.StatusOK, list)
	})
}

func parseFilter(r *http.Request) (TransactionFilter, error) {
	q := r.URL.Query()
	month, err := time.Parse("2006-01", q.Get("month"))
	if err != nil {
		return TransactionFilter{}, httpx.BadRequest(`month must look like "2026-10"`)
	}
	f := TransactionFilter{Month: month}
	if a := q.Get("account"); a != "" {
		if !uuidRe.MatchString(a) {
			return TransactionFilter{}, httpx.BadRequest("account must be an account id")
		}
		f.Account = AccountID(a)
	}
	if f.Flow, err = ParseFlow(q.Get("flow")); err != nil {
		return TransactionFilter{}, httpx.BadRequest(`flow must be "in", "out" or "transfer"`)
	}
	if f.Search = strings.TrimSpace(q.Get("q")); len([]rune(f.Search)) > 100 {
		return TransactionFilter{}, httpx.BadRequest("search is limited to 100 characters")
	}
	return f, nil
}
