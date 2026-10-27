package ledger

import (
	"errors"
	"strings"
	"testing"
	"time"

	"lastro/money"
)

func TestNewExternalID(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "c0000000-0000-4000-8000-000000000001", want: "c0000000-0000-4000-8000-000000000001"},
		{in: "  abc  ", want: "abc"},
		{in: "", wantErr: true},
		{in: "   ", wantErr: true},
		{in: "a\nb", wantErr: true},
		{in: strings.Repeat("x", 201), wantErr: true},
	}
	for _, tt := range tests {
		got, err := NewExternalID(tt.in)
		if tt.wantErr {
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("NewExternalID(%q): want ErrInvalid, got %v", tt.in, err)
			}
			continue
		}
		if err != nil || got.String() != tt.want {
			t.Errorf("NewExternalID(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
	if !(ExternalID{}).IsZero() {
		t.Error("the zero ExternalID must be zero")
	}
}

func TestKind(t *testing.T) {
	tests := []struct {
		in     string
		counts bool
	}{
		{"expense", true},
		{"income", true},
		{"transfer", false},
		{"bill_payment", false},
	}
	for _, tt := range tests {
		k, err := ParseKind(tt.in)
		if err != nil {
			t.Fatalf("ParseKind(%q): %v", tt.in, err)
		}
		if k.String() != tt.in {
			t.Errorf("Kind(%q).String() = %q", tt.in, k)
		}
		if k.Counts() != tt.counts {
			t.Errorf("Kind(%q).Counts() = %v, want %v", tt.in, k.Counts(), tt.counts)
		}
	}
	for _, bad := range []string{"", "Expense", "refund"} {
		if _, err := ParseKind(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseKind(%q): want ErrInvalid, got %v", bad, err)
		}
	}
}

func TestNewInstallment(t *testing.T) {
	tests := []struct {
		n, m    int
		want    string
		wantErr bool
	}{
		{n: 3, m: 10, want: "3 of 10"},
		{n: 1, m: 2, want: "1 of 2"},
		{n: 12, m: 12, want: "12 of 12"},
		{n: 0, m: 10, wantErr: true},
		{n: 11, m: 10, wantErr: true},
		{n: 1, m: 1, wantErr: true}, // a purchase paid at once is not an installment
		{n: -1, m: 3, wantErr: true},
		{n: 1, m: 1000, wantErr: true},
	}
	for _, tt := range tests {
		got, err := NewInstallment(tt.n, tt.m)
		if tt.wantErr {
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("NewInstallment(%d, %d): want ErrInvalid, got %v", tt.n, tt.m, err)
			}
			continue
		}
		if err != nil || got.String() != tt.want || got.Number() != tt.n || got.Total() != tt.m || got.IsZero() {
			t.Errorf("NewInstallment(%d, %d) = %v, %v", tt.n, tt.m, got, err)
		}
	}
	if !(Installment{}).IsZero() || (Installment{}).String() != "" {
		t.Error("the zero Installment means no installment")
	}
}

func TestNewCategoryID(t *testing.T) {
	if c, err := NewCategoryID(" groceries "); err != nil || c.String() != "groceries" {
		t.Errorf("NewCategoryID: %q, %v", c, err)
	}
	for _, bad := range []string{"", "  ", strings.Repeat("c", 65), "a b"} {
		if _, err := NewCategoryID(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("NewCategoryID(%q): want ErrInvalid, got %v", bad, err)
		}
	}
}

func TestSourceCanReplace(t *testing.T) {
	tests := []struct {
		next, current Source
		want          bool
	}{
		{SourceRule, "", true},
		{SourceLLM, SourceRule, true},
		{SourceRule, SourceLLM, true},
		{SourceManual, SourceRule, true},
		{SourceManual, SourceManual, true},
		{SourceRule, SourceManual, false},
		{SourceLLM, SourceManual, false},
	}
	for _, tt := range tests {
		if got := tt.next.CanReplace(tt.current); got != tt.want {
			t.Errorf("%q.CanReplace(%q) = %v, want %v", tt.next, tt.current, got, tt.want)
		}
	}
	for _, s := range []string{"rule", "llm", "manual"} {
		if got, err := ParseSource(s); err != nil || got.String() != s {
			t.Errorf("ParseSource(%q) = %q, %v", s, got, err)
		}
	}
	if _, err := ParseSource("ollama"); !errors.Is(err, ErrInvalid) {
		t.Errorf("ParseSource: want ErrInvalid, got %v", err)
	}
}

func TestStatusAndAccountType(t *testing.T) {
	for _, s := range []string{"pending", "posted"} {
		if got, err := ParseStatus(s); err != nil || got.String() != s {
			t.Errorf("ParseStatus(%q) = %q, %v", s, got, err)
		}
	}
	if _, err := ParseStatus("PENDING"); !errors.Is(err, ErrInvalid) {
		t.Error("ParseStatus is case-sensitive: the ledger has one spelling")
	}
	for _, tt := range []struct {
		in   string
		card bool
	}{{"checking", false}, {"savings", false}, {"credit_card", true}} {
		got, err := ParseAccountType(tt.in)
		if err != nil || got.String() != tt.in || got.IsCard() != tt.card {
			t.Errorf("ParseAccountType(%q) = %q, %v", tt.in, got, err)
		}
	}
	if _, err := ParseAccountType("investment"); !errors.Is(err, ErrInvalid) {
		t.Error("investments are portfolio positions, not ledger accounts")
	}
}

func TestNewCardDetails(t *testing.T) {
	brl := func(c int64) *money.Money { m := money.BRLCents(c); return &m }
	usd := func(c int64) *money.Money { m, _ := money.New(c, money.USD); return &m }
	due := time.Date(2026, 11, 10, 3, 0, 0, 0, time.UTC)

	c, err := NewCardDetails(brl(800000), brl(164560), due, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	limit, available, ok := c.Limit()
	if !ok || limit != money.BRLCents(800000) || available != money.BRLCents(164560) || c.Due().Format(time.DateOnly) != "2026-11-10" || !c.Closes().IsZero() || c.IsZero() {
		t.Fatalf("got %+v", c)
	}
	if over, err := NewCardDetails(brl(100000), brl(-2500), time.Time{}, time.Time{}); err != nil || over.IsZero() {
		t.Errorf("a card over its limit has a negative available amount: %v", err)
	}
	if none, err := NewCardDetails(nil, nil, time.Time{}, time.Time{}); err != nil || !none.IsZero() {
		t.Errorf("nothing reported is the zero value: %+v %v", none, err)
	}
	for name, tt := range map[string]struct{ limit, available *money.Money }{
		"limit without available": {brl(800000), nil},
		"available without limit": {nil, brl(1000)},
		"negative limit":          {brl(-1), brl(0)},
		"two currencies":          {brl(800000), usd(1000)},
	} {
		if _, err := NewCardDetails(tt.limit, tt.available, time.Time{}, time.Time{}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
}
