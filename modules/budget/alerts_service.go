package budget

import (
	"context"
	"fmt"
	"net/url"
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
	for _, l := range v.Lines {
		f := Figures{
			Limited: true, Fund: l.Fund, Spent: money.BRLCents(l.SpentCents), Available: money.BRLCents(l.AvailableCents),
			Left: money.BRLCents(l.LeftCents), Average: money.BRLCents(l.AverageCents), Pace: l.Pace,
		}
		f.Line, _ = ParseLineID(l.ID)
		if len(l.ByCategory) == 1 {
			f.Category, _ = ledger.NewCategoryID(l.ByCategory[0].Category)
		} else if l.Name != nil {
			f.Name = *l.Name
		}
		figures = append(figures, f)
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
	for _, a := range FindAlerts(m, today, figures, rec, v.DaysLeft) {
		av := AlertView{Key: a.Key, Kind: a.Kind, Link: "/budget"}
		name := names[a.Category.String()]
		if !a.Category.IsZero() {
			id := a.Category.String()
			av.Category = &id
		} else if a.Name != "" {
			name = a.Name
		}
		switch a.Kind {
		case AlertOver:
			av.Severity = "danger"
			av.Title = fmt.Sprintf("%s is over its limit by %s", name, a.Amount)
			av.Detail = fmt.Sprintf("%s spent in %s, with %s available.", money.BRLCents(a.Compare.Cents()+a.Amount.Cents()), monthName, a.Compare)
		case AlertOverdrawn:
			av.Severity = "warning"
			av.Title = fmt.Sprintf("%s is %s in the red", name, a.Amount)
			av.Detail = fmt.Sprintf("%s spent in %s from a fund that had %s; the months after pay it back.", money.BRLCents(a.Compare.Cents()+a.Amount.Cents()), monthName, a.Compare)
		case AlertAhead:
			av.Severity = "warning"
			av.Title = fmt.Sprintf("%s has %s left for %d %s", name, a.Amount, a.DaysLeft, plural(a.DaysLeft, "day", "days"))
			av.Detail = fmt.Sprintf("About %s a day, ahead of the month's pace.", money.BRLCents(a.Amount.Cents()/int64(max(a.DaysLeft, 1))))
		case AlertUnusual:
			av.Severity = "warning"
			pct := (a.Amount.Cents() - a.Compare.Cents()) * 100 / a.Compare.Cents()
			av.Title = fmt.Sprintf("%s is %d%% above its usual month", name, pct)
			av.Detail = fmt.Sprintf("%s so far in %s, against %s on average in the six months before.", a.Amount, monthName, a.Compare)
			if !a.Category.IsZero() {
				av.Link = "/transactions?category=" + a.Category.String()
			}
		case AlertPriceChange:
			av.Severity = "info"
			dir := "up"
			if a.Amount.Cents() < a.Compare.Cents() {
				dir = "down"
			}
			av.Title = fmt.Sprintf("%s went %s to %s", TitleCase(a.Name), dir, a.Amount)
			av.Detail = fmt.Sprintf("It was %s a month.", a.Compare)
			av.Link = merchantLink(a.Name)
		case AlertNewCharge:
			av.Severity = "info"
			av.Title = fmt.Sprintf("%s is a new recurring charge", TitleCase(a.Name))
			av.Detail = fmt.Sprintf("%s a month, adding to the fixed costs.", a.Amount)
			av.Link = merchantLink(a.Name)
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

// merchantLink lists a recurring charge's transactions in every month, so a
// new price shows next to the old one.
func merchantLink(name string) string {
	return "/transactions?" + url.Values{"q": {TitleCase(name)}, "scope": {"all"}}.Encode()
}

// TitleCase turns a merchant's pattern into a name: "SMART FIT" is "Smart
// Fit", "PAO DE ACUCAR" is "Pao de Acucar".
func TitleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if i > 0 && (w == "de" || w == "da" || w == "do" || w == "e") {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
