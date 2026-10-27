package portfolio

import (
	"errors"
	"testing"
	"time"

	"lastro/money"
)

func TestThesisAndPurchase(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	wege, _ := NewTicker("WEGE3")
	if _, err := NewThesis(wege, "Grows with electrification", "", nil, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("a thesis without what breaks it: %v", err)
	}
	report := now.AddDate(0, 0, 2)
	th, err := NewThesis(wege, " Grows with electrification ", "Margins below 15% for two quarters", &report, now)
	if err != nil || th.Thesis != "Grows with electrification" {
		t.Fatalf("%+v, %v", th, err)
	}
	if d, ok := th.ReportIn(now); !ok || d != 2 {
		t.Errorf("report in %d (%v)", d, ok)
	}

	if _, err := NewPurchase(now, "WEGE3", ClassBR, money.BRLCents(0), "", now); !errors.Is(err, ErrInvalid) {
		t.Errorf("nothing to buy: %v", err)
	}
	p, err := NewPurchase(now, "WEGE3", ClassBR, money.BRLCents(40000), "after the report", now)
	if err != nil || p.Status != Planned || len(p.ID) != 36 {
		t.Fatalf("%+v, %v", p, err)
	}
	if err := p.Decide(Cancelled, "net debt rising", now); err != nil || p.Status != Cancelled {
		t.Fatalf("cancel: %v", err)
	}
	if err := p.Decide(Done, "", now); !errors.Is(err, ErrDecided) {
		t.Errorf("decided twice: %v", err)
	}
}
