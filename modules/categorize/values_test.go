package categorize

import (
	"errors"
	"strings"
	"testing"
)

func TestParseColor(t *testing.T) {
	for _, s := range []string{"note-1", "note-2", "note-5", "note-10", "note-20", "note-50", "note-100", "note-200"} {
		if c, err := ParseColor(s); err != nil || c.String() != s {
			t.Errorf("%s: got %q, %v", s, c, err)
		}
	}
	for _, bad := range []string{"", "note-3", "NOTE-1", "red", "note-500"} {
		if _, err := ParseColor(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: want ErrInvalid, got %v", bad, err)
		}
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct{ in, want string }{
		{"iFood", "IFOOD"},
		{"  Pág*iFood   SP  ", "PAG IFOOD SP"},
		{"Pão de Açúcar", "PAO DE ACUCAR"},
		{"UBER *TRIP HELP.UBER.COM", "UBER TRIP HELP UBER COM"},
		{"Pix enviado - Fulano", "PIX ENVIADO FULANO"},
		{"***", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalize(tt.in); got != tt.want {
			t.Errorf("normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNewPattern(t *testing.T) {
	p, err := NewPattern(PatternContains, " pag*ifood ")
	if err != nil || p.Text() != "PAG IFOOD" || p.Kind() != PatternContains {
		t.Fatalf("got %+v, %v", p, err)
	}
	for _, tt := range []struct {
		kind PatternKind
		text string
	}{
		{PatternContains, "ab"},
		{PatternContains, "a*b"}, // "A B" has only two letters
		{PatternExact, "**"},
		{"starts", "IFOOD"},
		{PatternContains, strings.Repeat("a", 101)},
	} {
		if _, err := NewPattern(tt.kind, tt.text); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s %q: want ErrInvalid, got %v", tt.kind, tt.text, err)
		}
	}
}

func TestPatternMatches(t *testing.T) {
	tests := []struct {
		kind        PatternKind
		pattern     string
		description string
		want        bool
	}{
		{PatternContains, "ifood", "PAG*IFOOD SP", true},
		{PatternContains, "ifood", "iFood", true},
		{PatternContains, "Uber Trip", "UBER *TRIP HELP.UBER.COM", true},
		{PatternContains, "bar", "Barbearia do Zé", false}, // whole words only
		{PatternContains, "trip uber", "UBER *TRIP", false},  // words in order
		{PatternContains, "pao de acucar", "Pão de Açúcar Loja 12", true},
		{PatternExact, "Pagamento de fatura", "Pagamento de fatura", true},
		{PatternExact, "Pagamento de fatura", "pagamento  de   FATURA", true},
		{PatternExact, "Netflix", "Netflix.com", false},
	}
	for _, tt := range tests {
		p, err := NewPattern(tt.kind, tt.pattern)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.Matches(tt.description); got != tt.want {
			t.Errorf("%s %q on %q: got %v", tt.kind, tt.pattern, tt.description, got)
		}
	}
	if (Pattern{}).Matches("") {
		t.Error("the zero pattern matches nothing")
	}
}

func TestCleanName(t *testing.T) {
	if n, err := cleanName("  Pets   and  vet "); err != nil || n != "Pets and vet" {
		t.Errorf("got %q, %v", n, err)
	}
	for _, bad := range []string{"", "   ", strings.Repeat("x", 41), "Pets\x00"} {
		if _, err := cleanName(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: want ErrInvalid, got %v", bad, err)
		}
	}
}
