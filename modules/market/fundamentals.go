package market

import (
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Dividend is one cash payment per share a company approved. Whoever held
// the share at the end of ComDate gets it on PaymentDate (zero while not
// set).
type Dividend struct {
	Symbol      string
	ComDate     time.Time
	Label       string // "DIVIDENDO", "JCP" (before its 15% tax), "RENDIMENTO" (a fund's)
	RateMicro   int64  // reais per share, in millionths
	PaymentDate time.Time
}

// Fundamental is what the source said about a company on a day. Any figure
// may be missing (an ETF has no earnings).
type Fundamental struct {
	Symbol    string
	Date      time.Time
	EPSMicro  *int64 // earnings per share over the last 12 months
	BookMicro *int64 // book value per share
	PEMilli   *int64 // the source's own P/E, when it sent one
	PBMilli   *int64
}

// Valuation is a company's multiples on a day, at that day's close. A
// multiple over earnings or book value that is not positive is nil: a
// negative P/E says nothing useful.
type Valuation struct {
	Date     time.Time
	PEMilli  *int64
	PBMilli  *int64
	YieldBps int64 // the last 12 months of cash dividends over the close
}

// TrailingPerShare adds the cash dividends whose com date falls in the
// twelve months up to at (included).
func TrailingPerShare(divs []Dividend, at time.Time) int64 {
	from := at.AddDate(-1, 0, 0)
	var sum int64
	for _, d := range divs {
		if d.ComDate.After(from) && !d.ComDate.After(at) {
			sum += d.RateMicro
		}
	}
	return sum
}

// YieldBps is a per-share amount over a price, in basis points.
func YieldBps(perShareMicro, closeCents int64) int64 {
	if closeCents <= 0 || perShareMicro <= 0 {
		return 0
	}
	// micro / 1e6 reais over cents / 100 reais, times 10.000 basis points.
	return roundDiv(perShareMicro, closeCents)
}

// Multiple is a price over a per-share figure, in thousandths; nil when the
// figure is not positive.
func Multiple(closeCents, perShareMicro int64) *int64 {
	if perShareMicro <= 0 || closeCents <= 0 {
		return nil
	}
	m := roundDiv(closeCents*10_000_000, perShareMicro)
	return &m
}

// ValueOn computes a day's multiples from its fundamentals, its close and
// the company's dividends. Per-share figures win over the source's own
// multiples, which were computed at a price lastro may not have.
func ValueOn(f Fundamental, closeCents int64, divs []Dividend) Valuation {
	v := Valuation{Date: f.Date, YieldBps: YieldBps(TrailingPerShare(divs, f.Date), closeCents)}
	switch {
	case f.EPSMicro != nil:
		v.PEMilli = Multiple(closeCents, *f.EPSMicro)
	case f.PEMilli != nil && *f.PEMilli > 0:
		v.PEMilli = f.PEMilli
	}
	switch {
	case f.BookMicro != nil:
		v.PBMilli = Multiple(closeCents, *f.BookMicro)
	case f.PBMilli != nil && *f.PBMilli > 0:
		v.PBMilli = f.PBMilli
	}
	return v
}

// scaled reads a decimal ("0.157843", "28.4") times a power of ten,
// rounded half away from zero.
func scaled(s string, by int64) (int64, error) {
	r, ok := new(big.Rat).SetString(strings.ReplaceAll(strings.TrimSpace(s), ",", "."))
	if !ok {
		return 0, fmt.Errorf("%w: number %q", ErrInvalid, s)
	}
	r.Mul(r, new(big.Rat).SetInt64(by))
	n, d := r.Num(), r.Denom()
	q, rem := new(big.Int).QuoRem(n, d, new(big.Int))
	if new(big.Int).Mul(new(big.Int).Abs(rem), big.NewInt(2)).Cmp(d) >= 0 {
		q.Add(q, big.NewInt(int64(n.Sign())))
	}
	if !q.IsInt64() {
		return 0, fmt.Errorf("%w: number %q", ErrInvalid, s)
	}
	return q.Int64(), nil
}

// optional reads an optional figure: nil when the source left it out.
func optional(s string, by int64) (*int64, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	v, err := scaled(s, by)
	if err != nil {
		return nil, err
	}
	return &v, nil
}
