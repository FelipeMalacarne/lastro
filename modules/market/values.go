package market

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"lastro/money"
)

// ErrInvalid means a value broke one of the market's rules.
var ErrInvalid = errors.New("market: invalid")

// Series is an index Banco Central publishes in SGS.
type Series string

const (
	CDI  Series = "CDI"  // a business day's rate (SGS 12)
	IPCA Series = "IPCA" // a month's inflation (SGS 433), dated on the month's first day
)

// Rate is a period's rate as a fraction in parts per billion: CDI's 0,050788%
// a day is 507880, IPCA's 0,82% a month is 8200000. Basis points are too
// coarse for a daily rate.
type Rate struct{ ppb int64 }

func RateFromPPB(ppb int64) Rate { return Rate{ppb: ppb} }
func (r Rate) PPB() int64        { return r.ppb }

// ParsePercentRate reads a rate written in percent with a dot, as SGS sends
// it: "0.050788" is 0,050788%. Up to seven decimal places.
func ParsePercentRate(s string) (Rate, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	r, ok := new(big.Rat).SetString(s)
	if !ok || s == "" {
		return Rate{}, fmt.Errorf("%w: rate %q", ErrInvalid, s)
	}
	// percent to ppb: × 10^7.
	r.Mul(r, big.NewRat(10_000_000, 1))
	if !r.IsInt() {
		return Rate{}, fmt.Errorf("%w: rate %q has more than seven decimal places", ErrInvalid, s)
	}
	return Rate{ppb: r.Num().Int64()}, nil
}

// Percent rounds the rate to basis points, for display.
func (r Rate) Percent() money.Percent { return money.FromBps(roundDiv(r.ppb, 100_000)) }

func roundDiv(a, b int64) int64 {
	if a >= 0 {
		return (a + b/2) / b
	}
	return -((-a + b/2) / b)
}

// Compound chains rates period after period: 1% and then 2% is 3,02%.
func Compound(rates []Rate) Rate {
	acc := big.NewRat(1, 1)
	for _, r := range rates {
		acc.Mul(acc, big.NewRat(1_000_000_000+r.ppb, 1_000_000_000))
	}
	acc.Sub(acc, big.NewRat(1, 1))
	acc.Mul(acc, big.NewRat(1_000_000_000, 1))
	n, d := acc.Num(), acc.Denom()
	q := new(big.Int).Quo(n, d)
	if rem := new(big.Int).Rem(n, d); new(big.Int).Abs(new(big.Int).Mul(rem, big.NewInt(2))).Cmp(d) >= 0 {
		if n.Sign() >= 0 {
			q.Add(q, big.NewInt(1))
		} else {
			q.Sub(q, big.NewInt(1))
		}
	}
	return Rate{ppb: q.Int64()}
}

// Point is one value of a series.
type Point struct {
	Series Series
	Date   time.Time // the day for CDI, the month's first day for IPCA
	Rate   Rate
}

// Quote is a ticker's closing price on a day; for today, the latest price.
type Quote struct {
	Symbol string
	Date   time.Time
	Close  money.Money
	Source string
}

// Bond is one Tesouro Direto title's rate and price on a day.
type Bond struct {
	Name     string    // "Tesouro IPCA+"
	Maturity time.Time // 2035-05-15
	Date     time.Time
	BuyRate  money.Percent // a year, over the index for IPCA+ and Selic
	Price    money.Money   // the buy price of one title
}

// Title is how Tesouro Direto names it on its site: "Tesouro IPCA+ 2035".
func (b Bond) Title() string { return fmt.Sprintf("%s %d", b.Name, b.Maturity.Year()) }

// lastWeekday is the weekday before day: the last day Tesouro Direto can
// have published. Holidays are not known, so after one the file is read
// again for nothing.
func lastWeekday(day time.Time) time.Time {
	d := day.AddDate(0, 0, -1)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	return d
}
