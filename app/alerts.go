package app

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"lastro/modules/budget"
	"lastro/modules/ledger"
	"lastro/money"
	"lastro/platform/httpx"
)

const (
	// AlertPriceDrop is a held stock that fell more than its alert in a day.
	AlertPriceDrop budget.AlertKind = "price_drop"
	// AlertPossibleDuplicate is a transaction I added by hand that the bank
	// may have sent since.
	AlertPossibleDuplicate budget.AlertKind = "possible_duplicate"
)

// Alerts is everything worth telling now: transactions I added that the bank
// may have sent too, the budget's (over a limit, ahead of pace, unusual,
// changed charges) and the price alerts that fired.
func (a *App) Alerts(ctx context.Context) ([]budget.AlertView, error) {
	dups, err := a.Ledger.PossibleDuplicates(ctx)
	if err != nil {
		return nil, err
	}
	var out []budget.AlertView
	for _, d := range dups {
		out = append(out, duplicateAlert(d))
	}
	bud, err := a.Budget.Alerts(ctx)
	if err != nil {
		return nil, err
	}
	out = append(out, bud...)
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
	rank := map[string]int{"danger": 0, "warning": 1, "info": 2}
	slices.SortStableFunc(out, func(x, y budget.AlertView) int { return rank[x.Severity] - rank[y.Severity] })
	return out, nil
}

// duplicateAlert asks whether a transaction I added and the synced one
// that looks like it are the same.
func duplicateAlert(d ledger.DuplicateView) budget.AlertView {
	day := func(s string) string {
		t, err := time.Parse(time.DateOnly, s)
		if err != nil {
			return s
		}
		return t.Format("2 Jan")
	}
	m, s := d.Manual, d.Synced
	return budget.AlertView{
		Key:      fmt.Sprintf("duplicate:%s:%s", m.ID, s.ID),
		Kind:     AlertPossibleDuplicate,
		Severity: "warning",
		Title:    fmt.Sprintf("The %s you added on %s may have come in from the bank", money.BRLCents(abs(m.AmountCents)), day(m.Date)),
		Detail:   fmt.Sprintf("\"%s\" on %s has the same amount. Open it to say whether they are the same transaction.", s.Description, day(s.Date)),
		Link:     fmt.Sprintf("/transactions?month=%s&tx=%s", m.Date[:7], m.ID),
	}
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
