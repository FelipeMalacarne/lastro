// Package money is the shared kernel for monetary values.
//
// Money is always an integer number of cents plus a currency. Never use
// float64 for money in this project. The zero value of Money has no
// currency; use Zero or BRLCents to create values.
package money

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// Currency is an ISO 4217 currency code.
type Currency string

const (
	BRL Currency = "BRL"
	USD Currency = "USD"
)

var (
	ErrCurrencyMismatch = errors.New("money: currency mismatch")
	ErrUnknownCurrency  = errors.New("money: unknown currency")
	ErrInvalidAmount    = errors.New("money: invalid amount")
	ErrInvalidWeights   = errors.New("money: invalid weights")
)

func (c Currency) valid() bool { return c == BRL || c == USD }

func (c Currency) symbol() string {
	switch c {
	case BRL:
		return "R$"
	case USD:
		return "US$"
	}
	return string(c)
}

// Money is an immutable monetary value in cents.
type Money struct {
	cents    int64
	currency Currency
}

// New creates a value from cents.
func New(cents int64, c Currency) (Money, error) {
	if !c.valid() {
		return Money{}, fmt.Errorf("%w: %q", ErrUnknownCurrency, c)
	}
	return Money{cents: cents, currency: c}, nil
}

// BRLCents creates a BRL value from cents. Shortcut for the common case.
func BRLCents(cents int64) Money { return Money{cents: cents, currency: BRL} }

// Zero returns zero in the given currency.
func Zero(c Currency) (Money, error) { return New(0, c) }

// ParseDecimal parses dot-decimal values as they come from APIs
// ("1234.56", "-10", "0.5"). At most two decimal places are accepted.
func ParseDecimal(s string, c Currency) (Money, error) {
	if !c.valid() {
		return Money{}, fmt.Errorf("%w: %q", ErrUnknownCurrency, c)
	}
	cents, err := parseFixed2(s)
	if err != nil {
		return Money{}, err
	}
	return Money{cents: cents, currency: c}, nil
}

// parseFixed2 converts a decimal with up to two places into hundredths.
func parseFixed2(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("%w: empty", ErrInvalidAmount)
	}
	neg := false
	switch s[0] {
	case '-':
		neg, s = true, s[1:]
	case '+':
		s = s[1:]
	}
	whole, frac, hasDot := strings.Cut(s, ".")
	if !isDigits(whole) || (hasDot && !isDigits(frac)) {
		return 0, fmt.Errorf("%w: %q", ErrInvalidAmount, s)
	}
	if len(frac) > 2 {
		return 0, fmt.Errorf("%w: more than two decimal places in %q", ErrInvalidAmount, s)
	}
	for len(frac) < 2 {
		frac += "0"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidAmount, s)
	}
	f, _ := strconv.ParseInt(frac, 10, 64)
	if w > (math.MaxInt64-f)/100 {
		return 0, fmt.Errorf("%w: value too large", ErrInvalidAmount)
	}
	v := w*100 + f
	if neg {
		v = -v
	}
	return v, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (m Money) Cents() int64        { return m.cents }
func (m Money) Currency() Currency  { return m.currency }
func (m Money) IsZero() bool        { return m.cents == 0 }
func (m Money) IsNegative() bool    { return m.cents < 0 }
func (m Money) IsPositive() bool    { return m.cents > 0 }
func (m Money) Neg() Money          { return Money{cents: -m.cents, currency: m.currency} }
func (m Money) Equal(o Money) bool  { return m == o }
func (m Money) Times(n int64) Money { return Money{cents: m.cents * n, currency: m.currency} }

func (m Money) Abs() Money {
	if m.cents < 0 {
		return m.Neg()
	}
	return m
}

// Add sums two values in the same currency.
func (m Money) Add(o Money) (Money, error) {
	if err := m.sameCurrency(o); err != nil {
		return Money{}, err
	}
	return Money{cents: m.cents + o.cents, currency: m.currency}, nil
}

// Sub subtracts two values in the same currency.
func (m Money) Sub(o Money) (Money, error) {
	if err := m.sameCurrency(o); err != nil {
		return Money{}, err
	}
	return Money{cents: m.cents - o.cents, currency: m.currency}, nil
}

// Cmp returns -1, 0 or 1. Different currencies are not comparable.
func (m Money) Cmp(o Money) (int, error) {
	if err := m.sameCurrency(o); err != nil {
		return 0, err
	}
	switch {
	case m.cents < o.cents:
		return -1, nil
	case m.cents > o.cents:
		return 1, nil
	}
	return 0, nil
}

func (m Money) sameCurrency(o Money) error {
	if m.currency != o.currency {
		return fmt.Errorf("%w: %s and %s", ErrCurrencyMismatch, m.currency, o.currency)
	}
	return nil
}

// Allocate splits m proportionally to the weights without losing cents: the
// parts always add up to m. Cents left over from rounding go to the parts
// with the largest remainder; ties go to the earlier part.
func (m Money) Allocate(weights ...int64) ([]Money, error) {
	if len(weights) == 0 {
		return nil, fmt.Errorf("%w: no weights", ErrInvalidWeights)
	}
	var total int64
	for _, w := range weights {
		if w < 0 {
			return nil, fmt.Errorf("%w: negative weight", ErrInvalidWeights)
		}
		total += w
	}
	if total == 0 {
		return nil, fmt.Errorf("%w: weights add up to zero", ErrInvalidWeights)
	}

	abs, sign := m.cents, int64(1)
	if abs < 0 {
		abs, sign = -abs, -1
	}

	bigAbs, bigTotal := big.NewInt(abs), big.NewInt(total)
	parts := make([]Money, len(weights))
	rems := make([]int64, len(weights))
	var allocated int64
	for i, w := range weights {
		q, r := new(big.Int).QuoRem(new(big.Int).Mul(bigAbs, big.NewInt(w)), bigTotal, new(big.Int))
		parts[i] = Money{cents: q.Int64(), currency: m.currency}
		rems[i] = r.Int64()
		allocated += q.Int64()
	}

	order := make([]int, len(weights))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return rems[order[a]] > rems[order[b]] })
	for k := int64(0); k < abs-allocated; k++ {
		parts[order[k]].cents++
	}

	if sign < 0 {
		for i := range parts {
			parts[i].cents = -parts[i].cents
		}
	}
	return parts, nil
}

// String formats using the Brazilian convention: "R$ 1.234,56", "-R$ 10,00".
func (m Money) String() string {
	abs, sign := m.cents, ""
	if abs < 0 {
		abs, sign = -abs, "-"
	}
	return fmt.Sprintf("%s%s %s,%02d", sign, m.currency.symbol(), groupThousands(abs/100), abs%100)
}

func groupThousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	head := len(s) % 3
	if head > 0 {
		b.WriteString(s[:head])
	}
	for i := head; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
