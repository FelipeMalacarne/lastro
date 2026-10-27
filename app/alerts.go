package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"lastro/modules/budget"
	"lastro/money"
	"lastro/platform/httpx"
)

// AlertPriceDrop is a held stock that fell more than its alert in a day.
const AlertPriceDrop budget.AlertKind = "price_drop"

// Alerts is everything worth telling now: the budget's (over a limit, ahead
// of pace, unusual, changed charges) and the price alerts that fired.
func (a *App) Alerts(ctx context.Context) ([]budget.AlertView, error) {
	out, err := a.Budget.Alerts(ctx)
	if err != nil {
		return nil, err
	}
	drops, err := a.Portfolio.PriceDrops(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range drops {
		detail := fmt.Sprintf("Closed at %s. Your alert is at %s a day.", money.BRLCents(d.CloseCents), money.FromBps(d.ThresholdBps))
		if d.Thesis != "" {
			detail += " Thesis: " + d.Thesis
		}
		out = append(out, budget.AlertView{
			Key:      fmt.Sprintf("price:%s:%s", d.Ticker, d.Date.Format(time.DateOnly)),
			Kind:     AlertPriceDrop,
			Severity: "info",
			Title:    fmt.Sprintf("%s fell %s on %s", d.Ticker, money.FromBps((-d.ChangeBps+5)/10*10), d.Date.Format("2 Jan")),
			Detail:   detail,
			Link:     "/portfolio",
		})
	}
	return out, nil
}

// GET /api/v1/alerts: what is worth telling now, the most severe first.
func (a *App) handleAlerts(w http.ResponseWriter, r *http.Request) {
	v, err := a.Alerts(r.Context())
	if err != nil {
		httpx.WriteError(w, r, a.Logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"alerts": v})
}
