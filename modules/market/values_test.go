package market

import (
	"errors"
	"testing"
)

func TestParsePercentRate(t *testing.T) {
	tests := []struct {
		in   string
		want int64
		err  bool
	}{
		{"0.050788", 507880, false},
		{"0.82", 8200000, false},
		{"-0.32", -3200000, false},
		{"0,5", 5000000, false},
		{"0.00000001", 0, true},
		{"abc", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		got, err := ParsePercentRate(tt.in)
		if tt.err {
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("%q: %v, want ErrInvalid", tt.in, err)
			}
			continue
		}
		if err != nil || got.PPB() != tt.want {
			t.Errorf("%q = %d (%v), want %d", tt.in, got.PPB(), err, tt.want)
		}
	}
}

func TestCompound(t *testing.T) {
	r := func(s string) Rate { x, _ := ParsePercentRate(s); return x }
	tests := []struct {
		name  string
		rates []Rate
		bps   int64
	}{
		{"none", nil, 0},
		{"1% then 2%", []Rate{r("1"), r("2")}, 302},
		{"deflation", []Rate{r("0.5"), r("-0.32")}, 18},
		// 252 business days at 0,050788% a day: about 13,65% a year.
		{"a year of CDI", repeat(r("0.050788"), 252), 1365},
	}
	for _, tt := range tests {
		if got := Compound(tt.rates).Percent().Bps(); got != tt.bps {
			t.Errorf("%s = %d bps, want %d", tt.name, got, tt.bps)
		}
	}
}

func repeat(r Rate, n int) []Rate {
	out := make([]Rate, n)
	for i := range out {
		out[i] = r
	}
	return out
}
