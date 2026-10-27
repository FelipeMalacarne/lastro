package budget

import (
	"errors"
	"testing"
	"time"

	"lastro/money"
)

func TestDecide(t *testing.T) {
	today := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	sep, oct := month(t, "2026-09"), month(t, "2026-10")
	left := money.BRLCents(100000)
	tests := []struct {
		name   string
		m      Month
		left   money.Money
		choice RestChoice
		invest int64
		ok     bool
	}{
		{"invest a closed month", sep, left, RestInvest, 0, true},
		{"keep it", sep, left, RestKeep, 0, true},
		{"split", sep, left, RestSplit, 40000, true},
		{"split all of it", sep, left, RestSplit, 100000, true},
		{"split more than left", sep, left, RestSplit, 100001, false},
		{"split a negative amount", sep, left, RestSplit, -1, false},
		{"the month is not over", oct, left, RestInvest, 0, false},
		{"nothing left", sep, money.BRLCents(0), RestInvest, 0, false},
		{"ask is a default, not a choice", sep, left, RestAsk, 0, false},
	}
	for _, tt := range tests {
		_, err := Decide(tt.m, tt.left, tt.choice, money.BRLCents(tt.invest), today, now)
		if tt.ok && err != nil {
			t.Errorf("%s: %v", tt.name, err)
		}
		if !tt.ok && !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", tt.name, err)
		}
	}
	for _, s := range []string{"ask", "invest", "keep"} {
		if _, err := ParseRestDefault(s); err != nil {
			t.Errorf("default %q: %v", s, err)
		}
	}
	if _, err := ParseRestDefault("split"); !errors.Is(err, ErrInvalid) {
		t.Errorf("split as a default: %v", err)
	}
}

func TestRests(t *testing.T) {
	today := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	aug, sep, oct := month(t, "2026-08"), month(t, "2026-09"), month(t, "2026-10")
	// From August: pay yourself first R$ 3.000, food R$ 1.000 rolling over,
	// fun R$ 500 not rolling over.
	budget := func(t *testing.T) []*MonthlyBudget {
		b := saved(t, map[string]bool{"food": true}, map[string]map[string]int64{"2026-08": {"food": 100000, "fun": 50000}})
		_ = b[0].SetPayYourselfFirst(money.BRLCents(300000), b[0].Payday(), now)
		return b
	}
	income := map[Month]money.Money{aug: money.BRLCents(1000000), sep: money.BRLCents(1000000), oct: money.BRLCents(1000000)}
	cents := money.BRLCents

	tests := []struct {
		name      string
		spend     map[string]map[string]int64
		decisions map[Month]Decision
		def       RestChoice
		check     func(t *testing.T, r map[Month]Rest)
	}{
		{
			name: "a closed month frees what a limit that does not roll over left; one that rolls over keeps it",
			// August: food 600 of 1.000 (400 rolls), fun 200 of 500 (300 freed).
			spend: map[string]map[string]int64{"2026-08": {"food": 60000, "fun": 20000}},
			def:   RestAsk,
			check: func(t *testing.T, r map[Month]Rest) {
				a := r[aug]
				// 10.000 − 3.000 − 1.500 + 300 = 5.800.
				if a.Left.Cents() != 580000 || a.Unspent.Cents() != 30000 || a.Choice != "" || a.KeepForNext.Cents() != 0 {
					t.Errorf("August = %+v", a)
				}
			},
		},
		{
			name: "an overspend and spending without a limit come out of what is left",
			// September: fun 800 of 500 (300 over), 250 uncategorized, 150 in a category without a limit.
			spend: map[string]map[string]int64{"2026-09": {"fun": 80000, "": 25000, "gifts": 15000}},
			def:   RestAsk,
			check: func(t *testing.T, r map[Month]Rest) {
				s := r[sep]
				// 10.000 − 3.000 − 1.500 − 300 − 400; food rolls over, so it keeps what it left.
				if s.Over.Cents() != 30000 || s.Unlimited.Cents() != 40000 || s.Left.Cents() != 1000000-300000-150000-30000-40000 {
					t.Errorf("September = %+v", s)
				}
			},
		},
		{
			name: "what was kept is part of the next month; the open month counts every limit",
			spend: map[string]map[string]int64{
				"2026-08": {"food": 100000, "fun": 50000},
				"2026-09": {"food": 100000, "fun": 50000},
				"2026-10": {"food": 20000},
			},
			decisions: map[Month]Decision{aug: {Month: aug, Choice: RestSplit, Invest: cents(500000)}, sep: {Month: sep, Choice: RestKeep}},
			def:       RestAsk,
			check: func(t *testing.T, r map[Month]Rest) {
				// August left 5.500: 5.000 invested, 500 kept.
				if a := r[aug]; a.Left.Cents() != 550000 || a.Invest.Cents() != 500000 || a.KeepForNext.Cents() != 50000 || !a.Decided {
					t.Errorf("August = %+v", a)
				}
				// September: 5.500 + 500 kept = 6.000, all kept.
				if s := r[sep]; s.Kept.Cents() != 50000 || s.Left.Cents() != 600000 || s.KeepForNext.Cents() != 600000 {
					t.Errorf("September = %+v", s)
				}
				// October runs: 10.000 + 6.000 − 3.000 − 1.500, food's 200 spent changes nothing.
				if o := r[oct]; o.Closed || o.Kept.Cents() != 600000 || o.Left.Cents() != 1150000 || o.Choice != "" {
					t.Errorf("October = %+v", o)
				}
			},
		},
		{
			name:      "the default decides the months I did not",
			spend:     map[string]map[string]int64{},
			decisions: map[Month]Decision{sep: {Month: sep, Choice: RestKeep}},
			def:       RestInvest,
			check: func(t *testing.T, r map[Month]Rest) {
				if a := r[aug]; a.Choice != RestInvest || a.Decided || a.Invest.Cents() != a.Left.Cents() || a.KeepForNext.Cents() != 0 {
					t.Errorf("August = %+v", a)
				}
				if s := r[sep]; s.Choice != RestKeep || !s.Decided || s.Kept.Cents() != 0 {
					t.Errorf("September = %+v", s)
				}
			},
		},
		{
			name: "a month that went past what came in leaves nothing to decide and no debt",
			// August: 9.000 spent without a limit.
			spend:     map[string]map[string]int64{"2026-08": {"": 900000}},
			decisions: map[Month]Decision{aug: {Month: aug, Choice: RestInvest}},
			def:       RestKeep,
			check: func(t *testing.T, r map[Month]Rest) {
				a := r[aug]
				if a.Left.Cents() >= 0 || a.Choice != "" || a.Invest.Cents() != 0 || a.KeepForNext.Cents() != 0 {
					t.Errorf("August = %+v", a)
				}
				if s := r[sep]; s.Kept.Cents() != 0 {
					t.Errorf("September starts with %v kept, want nothing", s.Kept)
				}
			},
		},
		{
			name: "a split larger than what is left after a late change invests all of it and keeps nothing",
			// The split was 6.000, but August now leaves 5.500.
			spend:     map[string]map[string]int64{"2026-08": {"food": 100000, "fun": 50000}},
			decisions: map[Month]Decision{aug: {Month: aug, Choice: RestSplit, Invest: cents(600000)}},
			def:       RestAsk,
			check: func(t *testing.T, r map[Month]Rest) {
				if a := r[aug]; a.Invest.Cents() != 550000 || a.KeepForNext.Cents() != 0 {
					t.Errorf("August = %+v", a)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, Rests(budget(t), spend(t, tt.spend), income, tt.decisions, tt.def, oct, today))
		})
	}
}
