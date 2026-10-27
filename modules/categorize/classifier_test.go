package categorize

import (
	"strings"
	"testing"

	"lastro/modules/ledger"
)

func TestParseAnswer(t *testing.T) {
	health, food := category(t, "Health"), category(t, "Restaurants")
	gone := category(t, "Pets")
	gone.Archived = true
	cats := []Category{health, food, gone}
	tests := []struct {
		name string
		in   answer
		want ledger.CategoryID
		conf int
	}{
		{"an existing category", answer{"Health", 0.92}, health.ID, 92},
		{"case and accents do not matter", answer{"restaurants", 0.5}, food.ID, 50},
		{"never an invented one", answer{"Pharmacy", 0.99}, ledger.CategoryID{}, 0},
		{"never an archived one", answer{"Pets", 0.99}, ledger.CategoryID{}, 0},
		{"unknown", answer{"unknown", 0}, ledger.CategoryID{}, 0},
		{"confidence is clamped", answer{"Health", 7}, health.ID, 100},
		{"a negative confidence is none", answer{"Health", -1}, health.ID, 0},
	}
	for _, tt := range tests {
		got, conf := parseAnswer(tt.in, cats)
		if got != tt.want || conf != tt.conf {
			t.Errorf("%s: got %v %d, want %v %d", tt.name, got, conf, tt.want, tt.conf)
		}
	}
	if (Suggestion{Category: health.ID, Confidence: 80}).Confident() != true || (Suggestion{Category: health.ID, Confidence: 79}).Confident() {
		t.Error("confident from 80%")
	}
	if (Suggestion{Confidence: 100}).Confident() {
		t.Error("never confident without a category")
	}
}

func TestClassifyPrompt(t *testing.T) {
	health, food := category(t, "Health"), category(t, "Restaurants")
	r, _ := NewRule(pattern(t, PatternContains, "ifood"), "", food, OriginCorrection, t0)
	p := classifyPrompt("DROGASIL", ledger.Categorizable{Description: "DROGASIL 2104 SAO PAULO", AmountCents: -8735}, []Category{health, food}, []*Rule{r})
	for _, want := range []string{"Categories: Health, Restaurants.", "- IFOOD: Restaurants", `"DROGASIL 2104 SAO PAULO"`, "87,35 reais", "money out"} {
		if !strings.Contains(p, want) {
			t.Errorf("the prompt lacks %q:\n%s", want, p)
		}
	}
}
