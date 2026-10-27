package categorize

import (
	"testing"
	"time"

	"lastro/modules/ledger"
)

// Each case is decided by one step of the order alone: without that step,
// the next ones would pick another rule.
func TestEngineMostSpecificWins(t *testing.T) {
	cat := func(name string) Category { return category(t, name) }
	rule := func(id RuleID, kind PatternKind, text string, account ledger.AccountID, c Category, taught time.Time) *Rule {
		r, err := NewRule(pattern(t, kind, text), account, c, taught)
		if err != nil {
			t.Fatal(err)
		}
		r.id = id
		return r
	}
	restaurants, leisure, groceries, transport, other := cat("Restaurants"), cat("Leisure"), cat("Groceries"), cat("Transport"), cat("Other")
	later := t0.Add(time.Hour)

	rules := []*Rule{
		rule("r01", PatternContains, "ifood", "", restaurants, t0),
		// The same words as an exact rule, taught later: only "exact before
		// contains" keeps the exact one first.
		rule("r02", PatternContains, "pag ifood", "", restaurants, later),
		rule("r03", PatternExact, "pag ifood", "", other, t0),
		rule("r04", PatternContains, "ifood", card, leisure, t0),
		// One long word against two short ones.
		rule("r05", PatternContains, "hortifruti", "", other, t0),
		rule("r06", PatternContains, "loja 12", "", leisure, t0),
		// One word each: the longer against the later.
		rule("r07", PatternContains, "mercado", "", groceries, t0),
		rule("r10", PatternContains, "bairro", "", groceries, later),
		// A full tie but the date; the ids would pick the other one.
		rule("r09", PatternContains, "centro", "", transport, t0),
	}
	tests := []struct {
		name        string
		description string
		account     ledger.AccountID
		want        Category
	}{
		{"one rule meets it", "IFOOD *RESTAURANTE", checking, restaurants},
		{"exact before contains", "PAG*IFOOD", checking, other},
		{"the account's own rule before an exact one for every account", "PAG*IFOOD", card, leisure},
		{"a rule for another account does not apply", "IFOOD *RESTAURANTE", ledger.AccountID("1e2d3c4b-5a69-4788-9a0b-1c2d3e4f5a6b"), restaurants},
		{"more words before more characters", "HORTIFRUTI LOJA 12", checking, leisure},
		{"more characters before the latest", "MERCADO HORTIFRUTI BAIRRO", checking, other},
		{"the latest correction wins a full tie", "CENTRO BAIRRO", checking, groceries},
		{"nothing meets it", "Netflix.com", checking, Category{}},
	}
	e := NewEngine(rules)
	for _, tt := range tests {
		got := e.Match(tt.description, tt.account)
		switch {
		case tt.want.ID.IsZero() && got != nil:
			t.Errorf("%s: want no rule, got %+v", tt.name, got)
		case !tt.want.ID.IsZero() && (got == nil || got.category != tt.want.ID):
			t.Errorf("%s: want %s, got %+v", tt.name, tt.want.Name, got)
		}
	}
	if rules[0].id != "r01" {
		t.Error("the engine orders a copy, not the caller's slice")
	}
	if NewEngine(nil).Match("IFOOD", checking) != nil {
		t.Error("no rules, no match")
	}
}
