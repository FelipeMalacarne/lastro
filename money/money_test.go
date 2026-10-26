package money

import (
	"errors"
	"testing"
)

func TestNewRejectsUnknownCurrency(t *testing.T) {
	if _, err := New(100, "XYZ"); !errors.Is(err, ErrUnknownCurrency) {
		t.Fatalf("want ErrUnknownCurrency, got %v", err)
	}
}

func TestParseDecimal(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"1234.56", 123456, false},
		{"-10", -1000, false},
		{"0.5", 50, false},
		{"+3.07", 307, false},
		{" 42.00 ", 4200, false},
		{"1.234", 0, true},
		{"1,50", 0, true},
		{"", 0, true},
		{".5", 0, true},
		{"5.", 0, true},
		{"abc", 0, true},
		{"99999999999999999999", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseDecimal(tt.in, BRL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Cents() != tt.want {
				t.Fatalf("cents = %d, want %d", got.Cents(), tt.want)
			}
		})
	}
}

func TestAddRequiresSameCurrency(t *testing.T) {
	usd, _ := New(100, USD)
	if _, err := BRLCents(100).Add(usd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("want ErrCurrencyMismatch, got %v", err)
	}
	sum, err := BRLCents(150).Add(BRLCents(-50))
	if err != nil || sum != BRLCents(100) {
		t.Fatalf("sum = %v, %v", sum, err)
	}
}

func TestCmp(t *testing.T) {
	c, err := BRLCents(1).Cmp(BRLCents(2))
	if err != nil || c != -1 {
		t.Fatalf("Cmp = %d, %v", c, err)
	}
}

func TestString(t *testing.T) {
	usd, _ := New(123456789, USD)
	tests := []struct {
		m    Money
		want string
	}{
		{BRLCents(0), "R$ 0,00"},
		{BRLCents(5), "R$ 0,05"},
		{BRLCents(123456), "R$ 1.234,56"},
		{BRLCents(-1000), "-R$ 10,00"},
		{BRLCents(100000000), "R$ 1.000.000,00"},
		{usd, "US$ 1.234.567,89"},
	}
	for _, tt := range tests {
		if got := tt.m.String(); got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
	}
}

func TestAllocate(t *testing.T) {
	tests := []struct {
		name    string
		m       Money
		weights []int64
		want    []int64
	}{
		{"exact split", BRLCents(400000), []int64{60, 30, 10}, []int64{240000, 120000, 40000}},
		{"leftover cent goes to the largest remainder", BRLCents(100), []int64{1, 1, 1}, []int64{34, 33, 33}},
		{"negative value", BRLCents(-100), []int64{1, 1, 1}, []int64{-34, -33, -33}},
		{"zero weight gets nothing", BRLCents(101), []int64{0, 1, 1}, []int64{0, 51, 50}},
		{"basis-point weights", BRLCents(123457), []int64{6000, 3000, 1000}, []int64{74074, 37037, 12346}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts, err := tt.m.Allocate(tt.weights...)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			var sum int64
			for i, p := range parts {
				sum += p.Cents()
				if p.Cents() != tt.want[i] {
					t.Errorf("part %d = %d, want %d", i, p.Cents(), tt.want[i])
				}
			}
			if sum != tt.m.Cents() {
				t.Fatalf("parts add up to %d, want %d", sum, tt.m.Cents())
			}
		})
	}
}

func TestAllocateRejectsBadWeights(t *testing.T) {
	for _, w := range [][]int64{nil, {0, 0}, {1, -1}} {
		if _, err := BRLCents(100).Allocate(w...); !errors.Is(err, ErrInvalidWeights) {
			t.Errorf("weights %v: want ErrInvalidWeights, got %v", w, err)
		}
	}
}
