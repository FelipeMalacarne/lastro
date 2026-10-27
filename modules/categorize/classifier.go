package categorize

import (
	"context"
	"fmt"
	"strings"
	"time"

	"lastro/modules/ledger"
)

// LLM is the little of the local model categorize needs. app injects the
// Ollama client; nothing else in the module knows it.
type LLM interface {
	Enabled() bool
	Model() string
	GenerateJSON(ctx context.Context, system, prompt string, out any) error
}

// confidentEnough is how sure the model must be (percent) for its answer to
// categorize without asking me; below it, the answer waits in the queue as a
// suggestion.
const confidentEnough = 80

// Suggestion is what the local model said about a merchant.
type Suggestion struct {
	Pattern    string            // the merchant, as a contains pattern's text
	Category   ledger.CategoryID // zero: the model did not know
	Confidence int               // percent
	Model      string
	AskedAt    time.Time
}

// Confident reports whether the answer may categorize on its own.
func (s Suggestion) Confident() bool { return !s.Category.IsZero() && s.Confidence >= confidentEnough }

// answer is the JSON the model must reply with.
type answer struct {
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"`
}

const classifySystem = `You categorize transactions from Brazilian bank statements for a personal finance app.
Reply with JSON only: {"category": "<one of the given category names, exactly>", "confidence": <a number from 0 to 1>}.
If none fits or you cannot tell, reply {"category": "unknown", "confidence": 0}. Never invent a category.`

// classifyPrompt asks about one merchant, with the categories that exist
// and the rules I taught as examples of how I categorize.
func classifyPrompt(merchant string, example ledger.Categorizable, cats []Category, rules []*Rule) string {
	var b strings.Builder
	names := make([]string, 0, len(cats))
	byID := map[ledger.CategoryID]string{}
	for _, c := range cats {
		if c.Archived {
			continue
		}
		names = append(names, c.Name)
		byID[c.ID] = c.Name
	}
	fmt.Fprintf(&b, "Categories: %s.\n", strings.Join(names, ", "))
	shown := 0
	for _, r := range rules {
		if name, ok := byID[r.category]; ok && shown < 30 {
			if shown == 0 {
				b.WriteString("How I categorized other merchants:\n")
			}
			fmt.Fprintf(&b, "- %s: %s\n", r.pattern.text, name)
			shown++
		}
	}
	direction := "money out (spending)"
	if example.AmountCents > 0 {
		direction = "money in"
	}
	fmt.Fprintf(&b, "Transaction: description %q (merchant %s), %s, %d,%02d reais.\n",
		example.Description, merchant, direction, abs(example.AmountCents)/100, abs(example.AmountCents)%100)
	b.WriteString("Which category is it?")
	return b.String()
}

// parseAnswer accepts only a category that exists and is live, by name
// (ignoring case and accents), and a confidence between 0 and 1. Anything
// else is "did not know".
func parseAnswer(a answer, cats []Category) (ledger.CategoryID, int) {
	want := normalize(a.Category)
	for _, c := range cats {
		if !c.Archived && normalize(c.Name) == want {
			conf := a.Confidence
			if conf != conf || conf < 0 { // NaN or negative
				conf = 0
			}
			return c.ID, int(min(conf, 1)*100 + 0.5)
		}
	}
	return ledger.CategoryID{}, 0
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
