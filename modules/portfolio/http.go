package portfolio

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"lastro/money"
	"lastro/platform/httpx"
)

// RegisterRoutes adds the portfolio's endpoints to the authenticated API.
func (s *Service) RegisterRoutes(mux *http.ServeMux, logger *slog.Logger) {
	fail := func(w http.ResponseWriter, r *http.Request, err error) { httpx.WriteError(w, r, logger, apiError(err)) }

	mux.HandleFunc("GET /api/v1/portfolio", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Overview(r.Context())
		if err != nil {
			fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, v)
	})

	// GET /api/v1/scoreboard: the stock picks against the Ibovespa and CDI, by year.
	mux.HandleFunc("GET /api/v1/scoreboard", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Scoreboard(r.Context())
		if err != nil {
			fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, v)
	})

	// GET /api/v1/net-worth?months=12: one point per stored day, and today.
	mux.HandleFunc("GET /api/v1/net-worth", func(w http.ResponseWriter, r *http.Request) {
		months := 12
		if q := r.URL.Query().Get("months"); q != "" {
			n, err := strconv.Atoi(q)
			if err != nil || n < 1 || n > 120 {
				fail(w, r, httpx.BadRequest("months must be 1 to 120"))
				return
			}
			months = n
		}
		v, err := s.NetWorthHistory(r.Context(), s.Today().AddDate(0, -months, 0))
		if err != nil {
			fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"points": v})
	})

	// PUT /api/v1/portfolio/targets {"targets": {"cdi": 2500, ...}, "bandBp": 200}.
	mux.HandleFunc("PUT /api/v1/portfolio/targets", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Targets map[Class]int64 `json:"targets"`
			BandBp  int64           `json:"bandBp"`
		}
		if err := decode(w, r, &in, "targets and bandBp"); err != nil {
			fail(w, r, err)
			return
		}
		ws := make(map[Class]money.Percent, len(in.Targets))
		for c, bp := range in.Targets {
			ws[c] = money.FromBps(bp)
		}
		if err := s.SetTargets(r.Context(), ws, in.BandBp); err != nil {
			fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// PATCH /api/v1/positions/{id} {"class": "ipca" | "" (automatic), "emergency": true}.
	mux.HandleFunc("PATCH /api/v1/positions/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Class     *string `json:"class"`
			Emergency *bool   `json:"emergency"`
		}
		if err := decode(w, r, &in, "class and emergency"); err != nil {
			fail(w, r, err)
			return
		}
		if err := s.UpdatePosition(r.Context(), r.PathValue("id"), in.Class, in.Emergency); err != nil {
			fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /api/v1/contribute?amount=300000: the split, in cents.
	mux.HandleFunc("GET /api/v1/contribute", func(w http.ResponseWriter, r *http.Request) {
		cents, err := strconv.ParseInt(r.URL.Query().Get("amount"), 10, 64)
		if err != nil || cents < 0 {
			fail(w, r, httpx.BadRequest("amount must be a whole number of cents"))
			return
		}
		v, err := s.Plan(r.Context(), money.BRLCents(cents))
		if err != nil {
			fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, v)
	})

	// PUT /api/v1/theses/{ticker} {"thesis", "breaks", "nextReport": "2026-10-29" | null}.
	mux.HandleFunc("PUT /api/v1/theses/{ticker}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Thesis     string  `json:"thesis"`
			Breaks     string  `json:"breaks"`
			NextReport *string `json:"nextReport"`
		}
		if err := decode(w, r, &in, "thesis, breaks and nextReport"); err != nil {
			fail(w, r, err)
			return
		}
		var next *time.Time
		if in.NextReport != nil && *in.NextReport != "" {
			d, err := time.Parse(time.DateOnly, *in.NextReport)
			if err != nil {
				fail(w, r, httpx.BadRequest("nextReport must be YYYY-MM-DD"))
				return
			}
			next = &d
		}
		if err := s.SaveThesis(r.Context(), r.PathValue("ticker"), in.Thesis, in.Breaks, next); err != nil {
			fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /api/v1/theses/{ticker}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.DeleteThesis(r.Context(), r.PathValue("ticker")); err != nil {
			fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /api/v1/reminders: earnings reports in the next week for stocks held.
	mux.HandleFunc("GET /api/v1/reminders", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Reminders(r.Context(), 7)
		if err != nil {
			fail(w, r, err)
			return
		}
		if v == nil {
			v = []Reminder{}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"reminders": v})
	})

	mux.HandleFunc("GET /api/v1/purchases", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Purchases(r.Context())
		if err != nil {
			fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"purchases": v})
	})

	// POST /api/v1/purchases {"plannedOn", "what", "class", "amountCents", "condition"}.
	mux.HandleFunc("POST /api/v1/purchases", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			PlannedOn   string `json:"plannedOn"`
			What        string `json:"what"`
			Class       Class  `json:"class"`
			AmountCents int64  `json:"amountCents"`
			Condition   string `json:"condition"`
		}
		if err := decode(w, r, &in, "plannedOn, what, class, amountCents and condition"); err != nil {
			fail(w, r, err)
			return
		}
		on, err := time.Parse(time.DateOnly, in.PlannedOn)
		if err != nil {
			fail(w, r, httpx.BadRequest("plannedOn must be YYYY-MM-DD"))
			return
		}
		v, err := s.PlanPurchase(r.Context(), on, in.What, in.Class, money.BRLCents(in.AmountCents), in.Condition)
		if err != nil {
			fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, v)
	})

	// PATCH /api/v1/purchases/{id} {"status": "done" | "cancelled", "note"}.
	mux.HandleFunc("PATCH /api/v1/purchases/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Status PurchaseStatus `json:"status"`
			Note   string         `json:"note"`
		}
		if err := decode(w, r, &in, "status and note"); err != nil {
			fail(w, r, err)
			return
		}
		if err := s.DecidePurchase(r.Context(), r.PathValue("id"), in.Status, in.Note); err != nil {
			fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func decode(w http.ResponseWriter, r *http.Request, v any, shape string) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
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
	case errors.Is(err, ErrNotFound):
		return httpx.NotFound("not found")
	case errors.Is(err, ErrDecided):
		return httpx.Conflict("the purchase was already decided")
	}
	return err
}
