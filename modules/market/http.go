package market

import (
	"log/slog"
	"net/http"
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
