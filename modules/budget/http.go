package budget

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"lastro/modules/ledger"
	"lastro/money"
	"lastro/platform/httpx"
)

// RegisterRoutes adds the budget's endpoints to the authenticated API.
func (s *Service) RegisterRoutes(mux *http.ServeMux, logger *slog.Logger) {
	// GET /api/v1/budget?month=2026-10; this month when it is left out.
	mux.HandleFunc("GET /api/v1/budget", func(w http.ResponseWriter, r *http.Request) {
		m := MonthOf(s.today())
		if q := r.URL.Query().Get("month"); q != "" {
			var err error
			if m, err = ParseMonth(q); err != nil {
				httpx.WriteError(w, r, logger, httpx.BadRequest("month must be YYYY-MM"))
				return
			}
		}
		v, err := s.Month(r.Context(), m)
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		httpx.JSON(w, http.StatusOK, v)
	})

	// GET /api/v1/recurring: the fixed costs panel.
	mux.HandleFunc("GET /api/v1/recurring", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Recurring(r.Context())
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		httpx.JSON(w, http.StatusOK, v)
	})

	// PUT /api/v1/budget/{month}/limits/{category} {"limitCents", "rollover"}:
	// from that month on.
	mux.HandleFunc("PUT /api/v1/budget/{month}/limits/{category}", func(w http.ResponseWriter, r *http.Request) {
		m, c, err := monthAndCategory(r)
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		var in struct {
			LimitCents *int64 `json:"limitCents"`
			Rollover   bool   `json:"rollover"`
		}
		if err := decode(w, r, &in, "limitCents and rollover"); err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		if in.LimitCents == nil {
			httpx.WriteError(w, r, logger, httpx.BadRequest("limitCents is required"))
			return
		}
		if err := s.SetLimit(r.Context(), m, c, money.BRLCents(*in.LimitCents), in.Rollover); err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /api/v1/budget/{month}/limits/{category}", func(w http.ResponseWriter, r *http.Request) {
		m, c, err := monthAndCategory(r)
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		if err := s.RemoveLimit(r.Context(), m, c); err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// PUT /api/v1/budget/{month}/pay-yourself-first {"amountCents", "payday"}.
	mux.HandleFunc("PUT /api/v1/budget/{month}/pay-yourself-first", func(w http.ResponseWriter, r *http.Request) {
		m, err := ParseMonth(r.PathValue("month"))
		if err != nil {
			httpx.WriteError(w, r, logger, httpx.BadRequest("month must be YYYY-MM"))
			return
		}
		var in struct {
			AmountCents *int64 `json:"amountCents"`
			Payday      int    `json:"payday"`
		}
		if err := decode(w, r, &in, "amountCents and payday"); err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		if in.AmountCents == nil {
			httpx.WriteError(w, r, logger, httpx.BadRequest("amountCents is required"))
			return
		}
		p, err := NewPayday(in.Payday)
		if err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		if err := s.SetPayYourselfFirst(r.Context(), m, money.BRLCents(*in.AmountCents), p); err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func monthAndCategory(r *http.Request) (Month, ledger.CategoryID, error) {
	m, err := ParseMonth(r.PathValue("month"))
	if err != nil {
		return Month{}, ledger.CategoryID{}, httpx.BadRequest("month must be YYYY-MM")
	}
	c, err := ledger.NewCategoryID(r.PathValue("category"))
	if err != nil {
		return Month{}, ledger.CategoryID{}, httpx.BadRequest("no category id")
	}
	return m, c, nil
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

// apiError gives the domain's errors their HTTP status.
func apiError(err error) error {
	switch {
	case errors.Is(err, ErrInvalid):
		return httpx.BadRequest(err.Error())
	case errors.Is(err, ErrNotBudgeted):
		return httpx.BadRequest("that category does not count toward the budget")
	}
	return err
}
