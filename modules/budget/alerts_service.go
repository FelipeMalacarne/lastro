package budget

import (
	"context"
	"fmt"
	"strings"

	"lastro/modules/ledger"
	"lastro/money"
)

// Alerts is what is worth telling this month (F10), newest worries first.
func (s *Service) Alerts(ctx context.Context) ([]AlertView, error) {
	today := s.today()
	m := MonthOf(today)
	v, err := s.Month(ctx, m)
	if err != nil {
		return nil, err
	}
	rec, err := s.recurring(ctx, today)
	if err != nil {
		return nil, fmt.Errorf("budget: alerts: %w", err)
	}
	cats, err := s.categorize.Categories(ctx)
	if err != nil {
		return nil, fmt.Errorf("budget: alerts: %w", err)
	}
	names := map[string]string{}
	for _, c := range cats {
		names[c.ID.String()] = c.Name
	}
	var figures []Figures
	for _, c := range v.Categories {
		id, _ := ledger.NewCategoryID(c.Category)
		figures = append(figures, Figures{
			Category: id, Limited: true, Spent: money.BRLCents(c.SpentCents), Available: money.BRLCents(c.AvailableCents),
			Left: money.BRLCents(c.LeftCents), Average: money.BRLCents(c.AverageCents), Pace: c.Pace,
		})
	}
	for _, u := range v.Unlimited {
		f := Figures{Spent: money.BRLCents(u.SpentCents), Average: money.BRLCents(u.AverageCents)}
		if u.Category != nil {
			f.Category, _ = ledger.NewCategoryID(*u.Category)
		}
		figures = append(figures, f)
	}
	monthName := today.Format("January")
	out := []AlertView{}
	if v.Prev != nil {
		prev, _ := ParseMonth(v.Prev.Month)
		r := Rest{Month: prev, Closed: v.Prev.Closed, Left: money.BRLCents(v.Prev.LeftCents), Choice: v.Prev.Choice}
		if a, ok := RestAlert(r); ok {
			name := prev.First().Format("January")
			av := AlertView{Key: a.Key, Kind: a.Kind, Link: "/budget"}
			if a.Kind == AlertRest {
				av.Severity = "info"
				av.Title = fmt.Sprintf("%s left %s", name, a.Amount)
				av.Detail = "Invest it or keep it to spend."
			} else {
				av.Severity = "warning"
				av.Title = fmt.Sprintf("%s went %s past what came in", name, a.Amount)
				av.Detail = "After pay yourself first. Nothing is left to invest or keep; it came out of what you had saved."
				av.Link = "/budget?month=" + prev.String()
			}
			out = append(out, av)
		}
	}
	for _, a := range FindAlerts(m, today, figures, rec, v.DaysLeft) {
		av := AlertView{Key: a.Key, Kind: a.Kind, Link: "/budget"}
		name := names[a.Category.String()]
		if !a.Category.IsZero() {
			id := a.Category.String()
			av.Category = &id
		}
		switch a.Kind {
		case AlertOver:
			av.Severity = "danger"
			av.Title = fmt.Sprintf("%s is over its limit by %s", name, a.Amount)
			av.Detail = fmt.Sprintf("%s spent in %s, with %s available.", money.BRLCents(a.Compare.Cents()+a.Amount.Cents()), monthName, a.Compare)
		case AlertAhead:
			av.Severity = "warning"
			av.Title = fmt.Sprintf("%s has %s left for %d %s", name, a.Amount, a.DaysLeft, plural(a.DaysLeft, "day", "days"))
			av.Detail = fmt.Sprintf("About %s a day, ahead of the month's pace.", money.BRLCents(a.Amount.Cents()/int64(max(a.DaysLeft, 1))))
		case AlertUnusual:
			av.Severity = "warning"
			pct := (a.Amount.Cents() - a.Compare.Cents()) * 100 / a.Compare.Cents()
			av.Title = fmt.Sprintf("%s is %d%% above its usual month", name, pct)
			av.Detail = fmt.Sprintf("%s so far in %s, against %s on average in the six months before.", a.Amount, monthName, a.Compare)
			av.Link = "/transactions?category=" + a.Category.String()
		case AlertPriceChange:
			av.Severity = "info"
			dir := "up"
			if a.Amount.Cents() < a.Compare.Cents() {
				dir = "down"
			}
			av.Title = fmt.Sprintf("%s went %s to %s", title(a.Name), dir, a.Amount)
			av.Detail = fmt.Sprintf("It was %s a month.", a.Compare)
			av.Link = "/dashboard"
		case AlertNewCharge:
			av.Severity = "info"
			av.Title = fmt.Sprintf("%s is a new recurring charge", title(a.Name))
			av.Detail = fmt.Sprintf("%s a month, adding to the fixed costs.", a.Amount)
			av.Link = "/dashboard"
		}
		out = append(out, av)
	}
	return out, nil
}

// Unsent are the alerts no channel was sent yet.
func (s *Service) Unsent(ctx context.Context, alerts []AlertView) ([]AlertView, error) {
	keys := make([]string, len(alerts))
	for i, a := range alerts {
		keys[i] = a.Key
	}
	sent, err := s.repo.sent(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("budget: unsent alerts: %w", err)
	}
	var out []AlertView
	for _, a := range alerts {
		if !sent[a.Key] {
			out = append(out, a)
		}
	}
	return out, nil
}

// MarkSent remembers alerts as pushed, so they are never pushed again.
func (s *Service) MarkSent(ctx context.Context, alerts []AlertView) error {
	keys := make([]string, len(alerts))
	for i, a := range alerts {
		keys[i] = a.Key
	}
	if err := s.repo.markSent(ctx, keys, s.now()); err != nil {
		return fmt.Errorf("budget: mark alerts sent: %w", err)
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// title turns a merchant's pattern into a name: "SMART FIT" is "Smart Fit".
func title(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if i > 0 && (w == "de" || w == "da" || w == "do" || w == "e") {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
