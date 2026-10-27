package portfolio

import (
	"context"
	"fmt"
	"sort"
	"time"

	"lastro/modules/market"
)

// Ibovespa is the index's symbol at brapi; the market refreshes it with the
// tickers held.
const Ibovespa = "^BVSP"

// Scoreboard compares the stock picks with the Ibovespa and CDI, year by
// year, from the stored valuations: each year from the last valuation of
// the year before (or the first one) to the last of the year (or today).
func (s *Service) Scoreboard(ctx context.Context) (ScoreboardView, error) {
	hist, err := s.repo.valuations(ctx, time.Time{})
	if err != nil {
		return ScoreboardView{}, fmt.Errorf("portfolio: scoreboard: %w", err)
	}
	sat := map[time.Time]Amounts{}
	var days []time.Time
	for _, h := range hist {
		if h.Class == string(ClassBR) {
			sat[h.Date] = h.Amounts
			days = append(days, h.Date)
		}
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	out := ScoreboardView{Years: []ScoreView{}}
	if len(days) < 2 {
		return out, nil
	}
	index, err := s.market.Quotes(ctx, Ibovespa, days[0].AddDate(0, 0, -10))
	if err != nil {
		return ScoreboardView{}, fmt.Errorf("portfolio: scoreboard: %w", err)
	}
	out.Ibovespa = len(index) > 0
	// closeOn is the index's last close at or before a day.
	closeOn := func(d time.Time) (int64, bool) {
		var c int64
		for _, q := range index {
			if q.Date.After(d) {
				break
			}
			c = q.Close.Cents()
		}
		return c, c > 0
	}
	// lastOn is the last valuation day at or before d.
	lastOn := func(d time.Time) (time.Time, bool) {
		var found time.Time
		for _, x := range days {
			if x.After(d) {
				break
			}
			found = x
		}
		return found, !found.IsZero()
	}
	today := s.Today()
	for y := days[0].Year(); y <= today.Year(); y++ {
		yearEnd := time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC)
		from, ok := lastOn(time.Date(y-1, 12, 31, 0, 0, 0, 0, time.UTC))
		partial := !ok || yearEnd.After(today)
		if !ok {
			from = days[0]
		}
		to, _ := lastOn(yearEnd)
		if !to.After(from) {
			continue
		}
		sv := ScoreView{Year: y, From: day(from), To: day(to), Partial: partial}
		if r, ok := Return(sat[from], sat[to]); ok && sat[from].Value.Cents() > 0 {
			sv.SatelliteBp = ptr(r.Bps())
		}
		if a, ok1 := closeOn(from); ok1 {
			if b, ok2 := closeOn(to); ok2 {
				sv.IbovespaBp = ptr((b - a) * 10000 / a)
			}
		}
		if r, last, err := s.market.Accumulated(ctx, market.CDI, from.AddDate(0, 0, 1), to); err == nil && !last.IsZero() {
			sv.CDIBp = ptr(r.Percent().Bps())
		}
		out.Years = append(out.Years, sv)
	}
	return out, nil
}
