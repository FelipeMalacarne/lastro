package market

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lastro/platform/httpx"
)

type indexJSON struct {
	Last12mBp int64   `json:"last12mBp"` // compounded over the twelve months to the last value
	Last      *string `json:"last"`      // the last day (CDI) or month (IPCA) with a value
	LatestBp  int64   `json:"latestBp"`  // CDI: the latest day's rate a year (252 business days); IPCA: the latest month
}

type bondJSON struct {
	Title      string `json:"title"`
	Name       string `json:"name"`
	Maturity   string `json:"maturity"`
	Date       string `json:"date"`
	BuyRateBp  int64  `json:"buyRateBp"`
	PriceCents int64  `json:"priceCents"`
}

// RegisterRoutes adds the market's endpoints to the authenticated API.
func (s *Service) RegisterRoutes(mux *http.ServeMux, logger *slog.Logger) {
	// GET /api/v1/quotes/{symbol}?months=12: the daily closes, oldest first.
	mux.HandleFunc("GET /api/v1/quotes/{symbol}", func(w http.ResponseWriter, r *http.Request) {
		months := 12
		if q := r.URL.Query().Get("months"); q != "" {
			n, err := strconv.Atoi(q)
			if err != nil || n < 1 || n > 60 {
				httpx.WriteError(w, r, logger, httpx.BadRequest("months must be 1 to 60"))
				return
			}
			months = n
		}
		qs, err := s.Quotes(r.Context(), strings.ToUpper(r.PathValue("symbol")), s.today().AddDate(0, -months, 0))
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		type point struct {
			Date       string `json:"date"`
			CloseCents int64  `json:"closeCents"`
		}
		out := make([]point, len(qs))
		for i, q := range qs {
			out[i] = point{Date: q.Date.Format(time.DateOnly), CloseCents: q.Close.Cents()}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"closes": out})
	})

	// GET /api/v1/market: CDI and IPCA over twelve months, and Tesouro Direto's titles.
	mux.HandleFunc("GET /api/v1/market", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		today := s.today()
		out := map[string]any{}
		for _, series := range []Series{CDI, IPCA} {
			pts, err := s.Rates(ctx, series, today.AddDate(-1, -1, 0), today)
			if err != nil {
				httpx.WriteError(w, r, logger, err)
				return
			}
			var v indexJSON
			if n := len(pts); n > 0 {
				last := pts[n-1]
				d := last.Date.Format(time.DateOnly)
				v.Last = &d
				var window []Rate
				for _, p := range pts {
					if p.Date.After(last.Date.AddDate(-1, 0, 0)) {
						window = append(window, p.Rate)
					}
				}
				v.Last12mBp = Compound(window).Percent().Bps()
				v.LatestBp = last.Rate.Percent().Bps()
				if series == CDI {
					year := make([]Rate, 252)
					for i := range year {
						year[i] = last.Rate
					}
					v.LatestBp = Compound(year).Percent().Bps()
				}
			}
			out[string(series)] = v
		}
		bonds, err := s.Bonds(ctx)
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		bs := make([]bondJSON, len(bonds))
		for i, b := range bonds {
			bs[i] = bondJSON{Title: b.Title(), Name: b.Name, Maturity: b.Maturity.Format(time.DateOnly), Date: b.Date.Format(time.DateOnly), BuyRateBp: b.BuyRate.Bps(), PriceCents: b.Price.Cents()}
		}
		out["bonds"] = bs
		httpx.JSON(w, http.StatusOK, out)
	})
}
