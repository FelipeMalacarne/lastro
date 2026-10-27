package portfolio

import (
	"errors"
	"testing"

	"lastro/money"
)

func alloc(t *testing.T, cdi, ipca, br, global int64) Allocation {
	t.Helper()
	a, err := NewAllocation(map[Class]money.Percent{ClassCDI: money.FromBps(cdi), ClassIPCA: money.FromBps(ipca), ClassBR: money.FromBps(br), ClassGlobal: money.FromBps(global)})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func vals(cdi, ipca, br, global int64) map[Class]money.Money {
	return map[Class]money.Money{ClassCDI: money.BRLCents(cdi), ClassIPCA: money.BRLCents(ipca), ClassBR: money.BRLCents(br), ClassGlobal: money.BRLCents(global)}
}

func TestNewAllocation(t *testing.T) {
	if _, err := NewAllocation(map[Class]money.Percent{ClassCDI: money.FromBps(5000), ClassIPCA: money.FromBps(4999)}); !errors.Is(err, ErrInvalid) {
		t.Errorf("99,99%%: %v", err)
	}
	if _, err := NewAllocation(map[Class]money.Percent{ClassCDI: money.FromBps(10100), ClassIPCA: money.FromBps(-100)}); !errors.Is(err, ErrInvalid) {
		t.Errorf("negative: %v", err)
	}
	if _, err := NewAllocation(map[Class]money.Percent{"crypto": money.FromBps(10000)}); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown class: %v", err)
	}
}

func TestPlanContribution(t *testing.T) {
	band, _ := NewBand(200)
	target := alloc(t, 2500, 2500, 1000, 4000)
	tests := []struct {
		name   string
		values map[Class]money.Money
		amount int64
		want   map[Class]int64
		why    map[Class]Why
	}{
		{
			// The canvas: R$ 100.000 invested, R$ 3.000 to put in. IPCA+ (22,9%) is
			// below its band and first gets R$ 790, up to 23% of R$ 103.000. The
			// other R$ 2.210 goes to the gaps left (CDI 1.750, IPCA+ 2.060, BR
			// 1.000) in proportion. Global (43,8%) is above its band: nothing.
			name:   "below and above the band",
			values: vals(2400000, 2290000, 930000, 4380000),
			amount: 300000,
			want:   map[Class]int64{ClassIPCA: 79000 + 94649, ClassCDI: 80405, ClassBR: 45946, ClassGlobal: 0},
			why:    map[Class]Why{ClassIPCA: WhyBelowBand, ClassCDI: WhyGap, ClassBR: WhyGap, ClassGlobal: WhyAboveBand},
		},
		{
			name:   "first contribution follows the targets",
			values: vals(0, 0, 0, 0),
			amount: 100001,
			want:   map[Class]int64{ClassCDI: 25000, ClassIPCA: 25000, ClassBR: 10000, ClassGlobal: 40001},
			why:    map[Class]Why{ClassCDI: WhyGap, ClassIPCA: WhyGap, ClassBR: WhyGap, ClassGlobal: WhyGap},
		},
		{
			name:   "on target: by the targets",
			values: vals(250000, 250000, 100000, 400000),
			amount: 100000,
			want:   map[Class]int64{ClassCDI: 25000, ClassIPCA: 25000, ClassBR: 10000, ClassGlobal: 40000},
		},
		{
			name:   "nothing to contribute",
			values: vals(250000, 200000, 100000, 450000),
			amount: 0,
			want:   map[Class]int64{ClassCDI: 0, ClassIPCA: 0, ClassBR: 0, ClassGlobal: 0},
		},
	}
	for _, tt := range tests {
		p, err := PlanContribution(tt.values, target, band, money.BRLCents(tt.amount))
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		var sum int64
		for _, s := range p.Splits {
			sum += s.Amount.Cents()
			if s.Amount.IsNegative() {
				t.Errorf("%s: %s is told to sell", tt.name, s.Class)
			}
			if got := s.Amount.Cents(); got != tt.want[s.Class] {
				t.Errorf("%s: %s gets %d, want %d", tt.name, s.Class, got, tt.want[s.Class])
			}
			if w, ok := tt.why[s.Class]; ok && s.Why != w {
				t.Errorf("%s: %s because %s, want %s", tt.name, s.Class, s.Why, w)
			}
		}
		if sum != tt.amount {
			t.Errorf("%s: splits add up to %d, not %d", tt.name, sum, tt.amount)
		}
	}
}

func TestPlanContributionReachesTheBand(t *testing.T) {
	band, _ := NewBand(200)
	p, _ := PlanContribution(vals(2400000, 2290000, 930000, 4380000), alloc(t, 2500, 2500, 1000, 4000), band, money.BRLCents(300000))
	for _, s := range p.Splits {
		if s.Class == ClassIPCA && s.After.Bps() < 2300 {
			t.Errorf("IPCA+ ends at %d bps, below its band", s.After.Bps())
		}
		if s.Class == ClassGlobal && s.After.Bps() >= s.Before.Bps() {
			t.Errorf("global grows from %d to %d bps", s.Before.Bps(), s.After.Bps())
		}
	}
}

func TestTickerAndQuantity(t *testing.T) {
	for _, tt := range []struct{ in, want string }{{"itub4f", "ITUB4"}, {"IVVB11", "IVVB11"}, {" petr4 ", "PETR4"}, {"AAPL34", "AAPL34"}} {
		if got, err := NewTicker(tt.in); err != nil || got.String() != tt.want {
			t.Errorf("%q = %q, %v", tt.in, got, err)
		}
	}
	for _, bad := range []string{"PETR", "PETR123", "4PETR", ""} {
		if _, err := NewTicker(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	q, _ := ParseQuantity("68.44")
	if q.String() != "68.44" || q.Of(money.BRLCents(4921)).Cents() != 336793 {
		t.Errorf("68.44 × 49,21 = %d", q.Of(money.BRLCents(4921)).Cents())
	}
	if q, _ := ParseQuantity("343615.4774965"); q.Micro() != 343615477497 {
		t.Errorf("rounding: %d", q.Micro())
	}
}
