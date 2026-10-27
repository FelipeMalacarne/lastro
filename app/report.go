package app

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"lastro/modules/budget"
	"lastro/modules/ledger"
	"lastro/money"
	"lastro/platform/httpx"
)

// ReportView is a month in review (F10 P2; canvas: "Month in review"):
// the month in four numbers, the limits, what is worth a look and the
// investments. It reads every module, so it lives in app.
type ReportView struct {
	Month    string `json:"month"`
	Over     bool   `json:"over"` // the month has ended
	Sentence string `json:"sentence"`

	IncomeCents       int64  `json:"incomeCents"`
	SpentCents        int64  `json:"spentCents"`
	ContributionCents int64  `json:"contributionCents"`
	SavingsRateBp     int64  `json:"savingsRateBp"`
	PrevSpentCents    *int64 `json:"prevSpentCents"`
	SpendingChangeBp  *int64 `json:"spendingChangeBp"` // against the month before

	Categories []ReportCategory `json:"categories"`
	// Unlimited is the spending in categories without a limit, the largest
	// first; Category is empty for what still waits for one.
	Unlimited []ReportCategory `json:"unlimited"`
	Notes     []string         `json:"notes"`

	NetWorthStartCents *int64 `json:"netWorthStartCents"`
	NetWorthEndCents   *int64 `json:"netWorthEndCents"`
	PutInCents         *int64 `json:"putInCents"`     // into investments, net of what was taken out
	ReturnsCents       *int64 `json:"returnsCents"`   // what the investments earned in the month
	DividendsCents     int64  `json:"dividendsCents"` // paid in the month
	// OffTarget is the class furthest under its target now, and by how many
	// basis points; empty when every class is on its target.
	OffTarget    string `json:"offTarget"`
	OffTargetBps int64  `json:"offTargetBps"`
}

// ReportCategory is a limited category's month.
type ReportCategory struct {
	Category       string `json:"category"`
	Name           string `json:"name"`
	SpentCents     int64  `json:"spentCents"`
	AvailableCents int64  `json:"availableCents"`
}

// reportInput is what the modules said about the month.
type reportInput struct {
	month        budget.Month
	today        time.Time
	view         budget.MonthView
	prev         *budget.MonthView
	names        map[string]string
	recurring    budget.RecurringView
	waiting      int
	start        *money.Money // net worth at the end of the month before
	end          *money.Money
	putIn        *money.Money
	returns      *money.Money
	dividends    int64
	offTarget    string
	offTargetBps int64
}

// buildReport turns what the modules said into the report: plain words,
// no cheering, the worst first.
func buildReport(in reportInput) ReportView {
	v := in.view
	r := ReportView{
		Month: in.month.String(), Over: !in.month.Add(1).First().After(in.today),
		IncomeCents: v.Summary.IncomeCents, SpentCents: v.Summary.SpentCents,
		ContributionCents: v.Summary.ContributionCents, SavingsRateBp: v.Summary.SavingsRateBp,
		Categories: []ReportCategory{}, Unlimited: []ReportCategory{}, Notes: []string{}, DividendsCents: in.dividends,
	}
	if in.prev != nil && in.prev.Summary.SpentCents > 0 {
		p := in.prev.Summary.SpentCents
		r.PrevSpentCents = &p
		ch := (v.Summary.SpentCents - p) * 10000 / p
		r.SpendingChangeBp = &ch
	}
	var over []string
	for _, c := range v.Categories {
		name := in.names[c.Category]
		r.Categories = append(r.Categories, ReportCategory{Category: c.Category, Name: name, SpentCents: c.SpentCents, AvailableCents: c.AvailableCents})
		if c.SpentCents > c.AvailableCents {
			over = append(over, name)
			r.Notes = append(r.Notes, fmt.Sprintf("%s went %s over.", name, money.BRLCents(c.SpentCents-c.AvailableCents)))
		}
	}
	sort.SliceStable(r.Categories, func(i, j int) bool {
		return r.Categories[i].SpentCents*r.Categories[j].AvailableCents > r.Categories[j].SpentCents*r.Categories[i].AvailableCents
	})
	for _, u := range v.Unlimited {
		if u.SpentCents == 0 {
			continue
		}
		c := ReportCategory{Name: "Waiting for a category", SpentCents: u.SpentCents}
		if u.Category != nil {
			c.Category, c.Name = *u.Category, in.names[*u.Category]
		}
		r.Unlimited = append(r.Unlimited, c)
	}
	sort.SliceStable(r.Unlimited, func(i, j int) bool { return r.Unlimited[i].SpentCents > r.Unlimited[j].SpentCents })
	first, last := in.month.First(), in.month.Add(1).First()
	inMonth := func(day string) bool {
		d, err := time.Parse(time.DateOnly, day)
		return err == nil && !d.Before(first) && d.Before(last)
	}
	for _, c := range in.recurring.Charges {
		switch {
		case c.New && inMonth(c.First):
			r.Notes = append(r.Notes, fmt.Sprintf("A new charge: %s, %s a month.", c.Name, money.BRLCents(c.MonthlyCents)))
		case c.Change != nil && inMonth(c.Change.Since):
			dir := "up"
			if c.Change.ToCents < c.Change.FromCents {
				dir = "down"
			}
			r.Notes = append(r.Notes, fmt.Sprintf("%s went %s to %s.", c.Name, dir, money.BRLCents(c.Change.ToCents)))
		}
	}
	if in.waiting > 0 {
		r.Notes = append(r.Notes, fmt.Sprintf("%d %s still waiting in To review.", in.waiting, plural(in.waiting, "transaction is", "transactions are")))
	}
	if in.start != nil && in.end != nil {
		s, e := in.start.Cents(), in.end.Cents()
		r.NetWorthStartCents, r.NetWorthEndCents = &s, &e
	}
	if in.putIn != nil {
		p := in.putIn.Cents()
		r.PutInCents = &p
	}
	if in.returns != nil {
		x := in.returns.Cents()
		r.ReturnsCents = &x
	}
	r.OffTarget, r.OffTargetBps = in.offTarget, in.offTargetBps

	var parts []string
	if r.IncomeCents > 0 {
		parts = append(parts, fmt.Sprintf("You kept %s of what came in", whole(r.SavingsRateBp)))
	}
	if r.SpendingChangeBp != nil {
		ch := *r.SpendingChangeBp
		prev := in.month.Add(-1).First().Format("January")
		switch {
		case ch <= -100:
			parts = append(parts, fmt.Sprintf("spending was %s below %s", whole(-ch), prev))
		case ch >= 100:
			parts = append(parts, fmt.Sprintf("spending was %s above %s", whole(ch), prev))
		default:
			parts = append(parts, fmt.Sprintf("spending was about the same as %s", prev))
		}
	}
	switch len(over) {
	case 0:
		if len(v.Categories) > 0 {
			parts = append(parts, "every category stayed within its limit")
		}
	case 1:
		parts = append(parts, over[0]+" was the only category over its limit")
	default:
		parts = append(parts, fmt.Sprintf("%d categories went over their limits", len(over)))
	}
	if len(parts) > 0 {
		r.Sentence = joinSentence(parts) + "."
	}
	if r.NetWorthStartCents != nil {
		d := money.BRLCents(*r.NetWorthEndCents - *r.NetWorthStartCents)
		verb := "grew"
		if d.IsNegative() {
			verb, d = "fell", d.Neg()
		}
		s := fmt.Sprintf(" Net worth %s %s", verb, d)
		if r.PutInCents != nil && *r.PutInCents > 0 {
			s += fmt.Sprintf(", with %s put in", money.BRLCents(*r.PutInCents))
		}
		r.Sentence = strings.TrimSpace(r.Sentence + s + ".")
	}
	return r
}

// joinSentence capitalizes the first part and joins the rest with commas
// and "and".
func joinSentence(parts []string) string {
	s := parts[0]
	if len(parts) > 1 {
		s += ". " + strings.ToUpper(parts[1][:1]) + parts[1][1:]
		if len(parts) > 2 {
			s += ", and " + strings.Join(parts[2:], ", and ")
		}
	}
	return s
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Report gathers a month's report from every module.
func (a *App) Report(ctx context.Context, m budget.Month) (ReportView, error) {
	in := reportInput{month: m, today: a.Portfolio.Today(), names: map[string]string{}}
	var err error
	if in.view, err = a.Budget.Month(ctx, m); err != nil {
		return ReportView{}, err
	}
	if prev, err := a.Budget.Month(ctx, m.Add(-1)); err == nil {
		in.prev = &prev
	}
	cats, err := a.Categorize.Categories(ctx)
	if err != nil {
		return ReportView{}, err
	}
	for _, c := range cats {
		in.names[c.ID.String()] = c.Name
	}
	if in.recurring, err = a.Budget.Recurring(ctx); err != nil {
		return ReportView{}, err
	}
	list, err := a.Ledger.Transactions(ctx, ledger.TransactionFilter{Month: m.First(), Category: ledger.CategoryNone})
	if err != nil {
		return ReportView{}, err
	}
	in.waiting = list.Summary.Count

	points, err := a.Portfolio.NetWorthHistory(ctx, m.First().AddDate(0, -1, 0))
	if err != nil {
		return ReportView{}, err
	}
	first, next := m.First(), m.Add(1).First()
	var start, end *int
	for i, p := range points {
		d, err := time.Parse(time.DateOnly, p.Date)
		if err != nil {
			continue
		}
		if d.Before(first) {
			start = &i
		}
		if d.Before(next) {
			end = &i
		}
	}
	if end != nil {
		e := money.BRLCents(points[*end].TotalCents)
		in.end = &e
		if start != nil {
			s := money.BRLCents(points[*start].TotalCents)
			in.start = &s
			p := money.BRLCents(points[*end].BasisCents - points[*start].BasisCents)
			in.putIn = &p
			// Invested and the emergency fund are what the basis covers.
			last, first := points[*end], points[*start]
			ret := money.BRLCents((last.InvestedCents + last.EmergencyCents - first.InvestedCents - first.EmergencyCents) - p.Cents())
			in.returns = &ret
		}
	}
	if o, err := a.Portfolio.Overview(ctx); err == nil {
		for _, c := range o.Classes {
			// Under half a point is on target, for the report's purpose.
			if gap := c.TargetBp - c.NowBp; gap >= 50 && gap > in.offTargetBps {
				in.offTarget, in.offTargetBps = c.Class.Name(), gap
			}
		}
	}
	div, err := a.Portfolio.Dividends(ctx)
	if err != nil {
		return ReportView{}, err
	}
	for _, p := range div.Paid {
		if p.PaymentDate != nil && strings.HasPrefix(*p.PaymentDate, m.String()) {
			in.dividends += p.Cents
		}
	}
	return buildReport(in), nil
}

// ReportText is the report as plain text for a push channel: totals only,
// never a transaction.
func ReportText(r ReportView, link string) string {
	var b strings.Builder
	m, _ := time.Parse("2006-01", r.Month)
	fmt.Fprintf(&b, "lastro · %s\n\n", m.Format("January 2006"))
	fmt.Fprintf(&b, "Income     %s\n", money.BRLCents(r.IncomeCents))
	spent := fmt.Sprintf("Spending   %s", money.BRLCents(r.SpentCents))
	if r.SpendingChangeBp != nil {
		spent += fmt.Sprintf(" (%s%s vs %s)", sign(*r.SpendingChangeBp), whole(abs(*r.SpendingChangeBp)), m.AddDate(0, -1, 0).Format("Jan"))
	}
	fmt.Fprintf(&b, "%s\nKept       %s\n", spent, whole(r.SavingsRateBp))
	if len(r.Notes) > 0 {
		b.WriteString("\n")
		for _, n := range r.Notes {
			fmt.Fprintf(&b, "• %s\n", n)
		}
	}
	if r.NetWorthEndCents != nil {
		fmt.Fprintf(&b, "\nNet worth %s", money.BRLCents(*r.NetWorthEndCents))
		if r.NetWorthStartCents != nil {
			d := *r.NetWorthEndCents - *r.NetWorthStartCents
			fmt.Fprintf(&b, " (%s%s)", sign(d), money.BRLCents(abs(d)))
		}
		b.WriteString("\n")
	}
	if link != "" {
		fmt.Fprintf(&b, "\n%s\n", link)
	}
	return b.String()
}

// whole is a percentage rounded to a whole number, for prose: "31%".
func whole(bps int64) money.Percent {
	if bps < 0 {
		return money.FromBps(-((-bps + 50) / 100 * 100))
	}
	return money.FromBps((bps + 50) / 100 * 100)
}

func sign(v int64) string {
	if v < 0 {
		return "−"
	}
	return "+"
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// SendReport pushes last month's report once, on the first sync from the
// 2nd of a month on (the last day's card and Pix items reach Pluggy late),
// when a channel is set up. It reports whether it sent one.
func (a *App) SendReport(ctx context.Context) (bool, error) {
	today := a.Portfolio.Today()
	if !a.Notify.Enabled() || today.Day() < 2 {
		return false, nil
	}
	last := budget.MonthOf(today).Add(-1)
	key := []budget.AlertView{{Key: "report:" + last.String()}}
	fresh, err := a.Budget.Unsent(ctx, key)
	if err != nil || len(fresh) == 0 {
		return false, err
	}
	r, err := a.Report(ctx, last)
	if err != nil {
		return false, err
	}
	if err := a.Notify.Send(ctx, ReportText(r, "")); err != nil {
		return false, err
	}
	return true, a.Budget.MarkSent(ctx, key)
}

// GET /api/v1/report?month=2026-09: a month in review; last month when it is left out.
func (a *App) handleReport(w http.ResponseWriter, r *http.Request) {
	m := budget.MonthOf(a.Portfolio.Today()).Add(-1)
	if q := r.URL.Query().Get("month"); q != "" {
		var err error
		if m, err = budget.ParseMonth(q); err != nil {
			httpx.WriteError(w, r, a.Logger, httpx.BadRequest("month must be YYYY-MM"))
			return
		}
	}
	v, err := a.Report(r.Context(), m)
	if err != nil {
		httpx.WriteError(w, r, a.Logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}
