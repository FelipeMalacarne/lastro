package ledger

import (
	"encoding/json"
	"errors"
	"io"
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

	mux.HandleFunc("GET /api/v1/accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := accountID(w, r, logger)
		if !ok {
			return
		}
		v, err := s.Account(r.Context(), id)
		writeView(w, r, logger, v, err)
	})

	// The balance at the end of each day, rebuilt from the bank's latest.
	mux.HandleFunc("GET /api/v1/accounts/{id}/balance", func(w http.ResponseWriter, r *http.Request) {
		id, ok := accountID(w, r, logger)
		if !ok {
			return
		}
		v, err := s.BalanceHistory(r.Context(), id)
		writeView(w, r, logger, v, err)
	})

	mux.HandleFunc("GET /api/v1/accounts/{id}/months", func(w http.ResponseWriter, r *http.Request) {
		id, ok := accountID(w, r, logger)
		if !ok {
			return
		}
		v, err := s.Months(r.Context(), id)
		writeView(w, r, logger, map[string]any{"months": v}, err)
	})

	mux.HandleFunc("GET /api/v1/accounts/{id}/installments", func(w http.ResponseWriter, r *http.Request) {
		id, ok := accountID(w, r, logger)
		if !ok {
			return
		}
		v, err := s.Installments(r.Context(), id)
		writeView(w, r, logger, map[string]any{"purchases": v}, err)
	})

	// GET /api/v1/transactions/days?month=2026-10: spending day by day.
	mux.HandleFunc("GET /api/v1/transactions/days", func(w http.ResponseWriter, r *http.Request) {
		month, err := time.Parse("2006-01", r.URL.Query().Get("month"))
		if err != nil {
			httpx.WriteError(w, r, logger, httpx.BadRequest(`month must look like "2026-10"`))
			return
		}
		v, err := s.Days(r.Context(), month)
		writeView(w, r, logger, v, err)
	})

	// GET /api/v1/transactions/categories?month=2026-10: the month's expenses
	// and income by category (an empty category: still without one).
	mux.HandleFunc("GET /api/v1/transactions/categories", func(w http.ResponseWriter, r *http.Request) {
		month, err := time.Parse("2006-01", r.URL.Query().Get("month"))
		if err != nil {
			httpx.WriteError(w, r, logger, httpx.BadRequest(`month must look like "2026-10"`))
			return
		}
		totals, err := s.CategoryTotals(r.Context(), month)
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"month": month.Format("2006-01"), "categories": totals})
	})

	mux.HandleFunc("GET /api/v1/transactions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !uuidRe.MatchString(id) {
			httpx.WriteError(w, r, logger, httpx.BadRequest("id must be a transaction id"))
			return
		}
		v, err := s.Transaction(r.Context(), TransactionID(id))
		writeView(w, r, logger, v, err)
	})

	// PUT /api/v1/transactions/{id}/note {"note": "..."}: an empty note clears it.
	mux.HandleFunc("PUT /api/v1/transactions/{id}/note", func(w http.ResponseWriter, r *http.Request) {
		id, ok := transactionID(w, r, logger)
		if !ok {
			return
		}
		var in struct {
			Note string `json:"note"`
		}
		if err := decode(w, r, &in, "note"); err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		writeNoContent(w, r, logger, s.SetNote(r.Context(), id, in.Note))
	})

	// PUT /api/v1/transactions/{id}/tags {"tags": ["trip-rio"]} replaces them.
	mux.HandleFunc("PUT /api/v1/transactions/{id}/tags", func(w http.ResponseWriter, r *http.Request) {
		id, ok := transactionID(w, r, logger)
		if !ok {
			return
		}
		var in struct {
			Tags []string `json:"tags"`
		}
		if err := decode(w, r, &in, "tags"); err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		writeNoContent(w, r, logger, s.SetTags(r.Context(), id, in.Tags))
	})

	// DELETE /api/v1/transactions/{id}/split: the largest part's category stays.
	mux.HandleFunc("DELETE /api/v1/transactions/{id}/split", func(w http.ResponseWriter, r *http.Request) {
		id, ok := transactionID(w, r, logger)
		if !ok {
			return
		}
		writeNoContent(w, r, logger, s.Unsplit(r.Context(), id))
	})

	mux.HandleFunc("GET /api/v1/tags", func(w http.ResponseWriter, r *http.Request) {
		tags, err := s.Tags(r.Context())
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"tags": tags})
	})

	// GET /api/v1/tags/{tag}: the tag's money by category over every month.
	mux.HandleFunc("GET /api/v1/tags/{tag}", func(w http.ResponseWriter, r *http.Request) {
		cs, err := s.TagCategories(r.Context(), r.PathValue("tag"))
		if errors.Is(err, ErrInvalid) {
			err = httpx.BadRequest(err.Error())
		}
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"categories": cs})
	})

	// GET /api/v1/transactions?month=2026-10&day=2026-10-22&account=<id>&flow=in|out|transfer&category=<id>|none&tag=trip-rio&q=ifood
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
	if t := q.Get("tag"); t != "" {
		tag, err := NewTag(t)
		if err != nil {
			return TransactionFilter{}, httpx.BadRequest(err.Error())
		}
		f.Tag = string(tag)
	}
	if sc := q.Get("scope"); sc != "" {
		if sc != "all" || (f.Search == "" && f.Tag == "") {
			return TransactionFilter{}, httpx.BadRequest(`scope can only be "all", with a search or a tag`)
		}
		f.AllMonths = true
	}
	if c := q.Get("category"); c != "" {
		if c != CategoryNone && !uuidRe.MatchString(c) {
			return TransactionFilter{}, httpx.BadRequest(`category must be a category id or "none"`)
		}
		f.Category = c
	}
	if d := q.Get("day"); d != "" {
		day, err := time.Parse(time.DateOnly, d)
		if err != nil || day.Format("2006-01") != month.Format("2006-01") {
			return TransactionFilter{}, httpx.BadRequest(`day must be a day of the month, like "2026-10-22"`)
		}
		f.Day = day
	}
	return f, nil
}

func accountID(w http.ResponseWriter, r *http.Request, logger *slog.Logger) (AccountID, bool) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		httpx.WriteError(w, r, logger, httpx.BadRequest("id must be an account id"))
		return "", false
	}
	return AccountID(id), true
}

// writeView answers v, or the error: 404 for an unknown id.
func writeView(w http.ResponseWriter, r *http.Request, logger *slog.Logger, v any, err error) {
	if errors.Is(err, ErrNotFound) {
		err = httpx.NotFound("no such account or transaction")
	}
	if err != nil {
		httpx.WriteError(w, r, logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

func transactionID(w http.ResponseWriter, r *http.Request, logger *slog.Logger) (TransactionID, bool) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		httpx.WriteError(w, r, logger, httpx.BadRequest("id must be a transaction id"))
		return "", false
	}
	return TransactionID(id), true
}

// writeNoContent answers 204, or the error: 400 for an invalid value, 404
// for an unknown id.
func writeNoContent(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		err = httpx.BadRequest(err.Error())
	case errors.Is(err, ErrNotFound):
		err = httpx.NotFound("no such transaction")
	}
	if err != nil {
		httpx.WriteError(w, r, logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decode(w http.ResponseWriter, r *http.Request, v any, shape string) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return httpx.BadRequest("the body must be a JSON object with " + shape)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return httpx.BadRequest("the body must hold a single JSON object")
	}
	return nil
}
