package money

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// ErrInvalidPercent signals a percentage that could not be parsed.
var ErrInvalidPercent = errors.New("money: invalid percent")

// Percent is a percentage in basis points: 1 bp = 0.01% and 10,000 bps = 100%.
// Integers avoid rounding errors in allocations and rates.
type Percent struct {
	bps int64
}

// Hundred is 100%.
var Hundred = Percent{bps: 10_000}

// FromBps creates a percentage from basis points.
func FromBps(bps int64) Percent { return Percent{bps: bps} }

// ParsePercent accepts "60", "60%", "12.5", "12,5%" and "0.25". At most two decimal places.
func ParsePercent(s string) (Percent, error) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	s = strings.Replace(s, ",", ".", 1)
	v, err := parseFixed2(s)
	if err != nil {
		return Percent{}, fmt.Errorf("%w: %q", ErrInvalidPercent, s)
	}
	return Percent{bps: v}, nil
}

func (p Percent) Bps() int64            { return p.bps }
func (p Percent) IsZero() bool          { return p.bps == 0 }
func (p Percent) Add(o Percent) Percent { return Percent{bps: p.bps + o.bps} }
func (p Percent) Sub(o Percent) Percent { return Percent{bps: p.bps - o.bps} }
func (p Percent) Equal(o Percent) bool  { return p == o }
func (p Percent) Less(o Percent) bool   { return p.bps < o.bps }

// Of applies the percentage to a value, rounding half away from zero.
func (p Percent) Of(m Money) Money {
	prod := new(big.Int).Mul(big.NewInt(m.cents), big.NewInt(p.bps))
	q, r := new(big.Int).QuoRem(prod, big.NewInt(10_000), new(big.Int))
	if new(big.Int).Abs(r).Cmp(big.NewInt(5_000)) >= 0 {
		if prod.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return Money{cents: q.Int64(), currency: m.currency}
}

// String formats using the Brazilian convention: "60%", "12,5%", "0,25%".
func (p Percent) String() string {
	abs, sign := p.bps, ""
	if abs < 0 {
		abs, sign = -abs, "-"
	}
	whole, frac := abs/100, abs%100
	if frac == 0 {
		return fmt.Sprintf("%s%d%%", sign, whole)
	}
	f := strings.TrimRight(fmt.Sprintf("%02d", frac), "0")
	return fmt.Sprintf("%s%d,%s%%", sign, whole, f)
}
