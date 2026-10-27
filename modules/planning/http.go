package planning

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"lastro/money"
	"lastro/platform/httpx"
)

// RegisterRoutes adds the planning endpoint to the authenticated API.
func (s *Service) RegisterRoutes(mux *http.ServeMux, logger *slog.Logger) {
	// GET /api/v1/planning?monthly=300000&years=15&target=100000000, in cents.
	mux.HandleFunc("GET /api/v1/planning", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		num := func(k string, def int64) (int64, bool) {
			v := q.Get(k)
			if v == "" {
				return def, true
			}
			n, err := strconv.ParseInt(v, 10, 64)
			return n, err == nil && n >= 0
		}
		monthly, ok1 := num("monthly", 0)
		years, ok2 := num("years", 15)
		target, ok3 := num("target", 0)
		if !ok1 || !ok2 || !ok3 {
			httpx.WriteError(w, r, logger, httpx.BadRequest("monthly, years and target must be whole non-negative numbers"))
			return
		}
		v, err := s.Project(r.Context(), Ask{Monthly: money.BRLCents(monthly), Years: int(years), Target: money.BRLCents(target)})
		if errors.Is(err, ErrInvalid) {
			httpx.WriteError(w, r, logger, httpx.BadRequest(err.Error()))
			return
		}
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		httpx.JSON(w, http.StatusOK, v)
	})
}
