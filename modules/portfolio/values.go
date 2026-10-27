package portfolio

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"lastro/money"
)

// ErrInvalid means a value broke one of the portfolio's rules.
var ErrInvalid = errors.New("portfolio: invalid")

// Ticker is a B3 ticker: four letters and a share-class number (PETR4,
// IVVB11, AAPL34). The odd-lot ticker is the same asset, so ITUB4F becomes
// ITUB4.
type Ticker struct{ s string }

var tickerRe = regexp.MustCompile(`^[A-Z]{4}[0-9]{1,2}$`)

func NewTicker(s string) (Ticker, error) {
	t := strings.ToUpper(strings.TrimSpace(s))
	if strings.HasSuffix(t, "F") && tickerRe.MatchString(t[:len(t)-1]) {
		t = t[:len(t)-1]
	}
	if !tickerRe.MatchString(t) {
		return Ticker{}, fmt.Errorf("%w: ticker %q", ErrInvalid, s)
	}
	return Ticker{s: t}, nil
}

func (t Ticker) String() string { return t.s }
func (t Ticker) IsZero() bool   { return t.s == "" }

// Quantity is how many units are held, in millionths: banks report CDBs
// and fractional shares with decimals.
type Quantity struct{ micro int64 }

// ParseQuantity reads "118", "68.44" or "343615.477496"; past six decimal
// places it rounds half up.
func ParseQuantity(s string) (Quantity, error) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok || r.Sign() < 0 {
		return Quantity{}, fmt.Errorf("%w: quantity %q", ErrInvalid, s)
	}
	r.Mul(r, big.NewRat(1_000_000, 1))
	r.Add(r, big.NewRat(1, 2))
	return Quantity{micro: new(big.Int).Quo(r.Num(), r.Denom()).Int64()}, nil
}

func QuantityFromMicro(m int64) Quantity { return Quantity{micro: m} }
func (q Quantity) Micro() int64          { return q.micro }
func (q Quantity) IsZero() bool          { return q.micro == 0 }

// Of values the quantity at a unit price, rounding to the cent.
func (q Quantity) Of(price money.Money) money.Money {
	v := new(big.Int).Mul(big.NewInt(q.micro), big.NewInt(price.Cents()))
	v.Add(v, big.NewInt(500_000))
	return money.BRLCents(new(big.Int).Quo(v, big.NewInt(1_000_000)).Int64())
}

func (q Quantity) String() string {
	r := new(big.Rat).SetFrac64(q.micro, 1_000_000)
	s := r.FloatString(6)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s
}

// Class is an asset class of the target allocation. The emergency fund is
// apart and never one of them.
type Class string

const (
	ClassCDI    Class = "cdi"       // post-fixed income: CDBs at % of CDI, Tesouro Selic, cash funds
	ClassIPCA   Class = "ipca"      // inflation-linked: Tesouro IPCA+, CDBs at IPCA + x
	ClassGlobal Class = "global"    // stocks abroad, through ETFs and BDRs
	ClassBR     Class = "br_stocks" // Brazilian stocks, FIIs and their ETFs
)

// Classes are the allocation's classes, in display order.
var Classes = []Class{ClassCDI, ClassIPCA, ClassBR, ClassGlobal}

// ParseClass accepts the four classes.
func ParseClass(s string) (Class, error) {
	for _, c := range Classes {
		if string(c) == s {
			return c, nil
		}
	}
	return "", fmt.Errorf("%w: class %q", ErrInvalid, s)
}

// Allocation is the target weight of each class, adding up to exactly 100%.
type Allocation struct{ w map[Class]money.Percent }

// NewAllocation checks that every class is known, no weight is negative and
// the weights add up to 100%. A class left out weighs zero.
func NewAllocation(weights map[Class]money.Percent) (Allocation, error) {
	var sum int64
	w := make(map[Class]money.Percent, len(Classes))
	for c, p := range weights {
		if _, err := ParseClass(string(c)); err != nil {
			return Allocation{}, err
		}
		if p.Bps() < 0 {
			return Allocation{}, fmt.Errorf("%w: negative target for %s", ErrInvalid, c)
		}
		w[c] = p
		sum += p.Bps()
	}
	if sum != 10000 {
		return Allocation{}, fmt.Errorf("%w: targets add up to %d bps, not 100%%", ErrInvalid, sum)
	}
	return Allocation{w: w}, nil
}

func (a Allocation) Of(c Class) money.Percent { return a.w[c] }
func (a Allocation) IsZero() bool             { return len(a.w) == 0 }

// Weights is a copy of the targets, every class included.
func (a Allocation) Weights() map[Class]money.Percent {
	out := make(map[Class]money.Percent, len(Classes))
	for _, c := range Classes {
		out[c] = a.w[c]
	}
	return out
}

// Band is how far a class may drift from its target, either side, before it
// gets new money first.
type Band struct{ bps int64 }

// NewBand accepts zero to ten points.
func NewBand(bps int64) (Band, error) {
	if bps < 0 || bps > 1000 {
		return Band{}, fmt.Errorf("%w: band of %d bps, want 0 to 10 points", ErrInvalid, bps)
	}
	return Band{bps: bps}, nil
}

func (b Band) Bps() int64 { return b.bps }
