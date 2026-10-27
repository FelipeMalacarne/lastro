package categorize

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// ErrInvalid wraps every value object constructor error, so callers can
// answer "bad input" with errors.Is(err, categorize.ErrInvalid).
var ErrInvalid = errors.New("categorize: invalid value")

// Color is the banknote a category is drawn with: one of the eight notes of
// the real, from the 1-real coin to the 200-real note.
type Color string

const (
	Note1   Color = "note-1"
	Note2   Color = "note-2"
	Note5   Color = "note-5"
	Note10  Color = "note-10"
	Note20  Color = "note-20"
	Note50  Color = "note-50"
	Note100 Color = "note-100"
	Note200 Color = "note-200"
)

// ParseColor accepts the spellings above.
func ParseColor(s string) (Color, error) {
	switch c := Color(s); c {
	case Note1, Note2, Note5, Note10, Note20, Note50, Note100, Note200:
		return c, nil
	}
	return "", fmt.Errorf("%w: colour %q (want note-1, note-2, note-5, note-10, note-20, note-50, note-100 or note-200)", ErrInvalid, s)
}

func (c Color) String() string { return string(c) }

// PatternKind says how a rule's pattern meets a description.
type PatternKind string

const (
	// PatternExact matches a description that is the pattern, word for word.
	PatternExact PatternKind = "exact"
	// PatternContains matches a description that has the pattern's words in
	// a row: "IFOOD" meets "PAG*IFOOD SP", but "BAR" never meets "BARBEARIA".
	PatternContains PatternKind = "contains"
)

// ParsePatternKind accepts the spellings above.
func ParsePatternKind(s string) (PatternKind, error) {
	switch k := PatternKind(s); k {
	case PatternExact, PatternContains:
		return k, nil
	}
	return "", fmt.Errorf("%w: pattern kind %q", ErrInvalid, s)
}

const (
	minPattern = 3
	maxPattern = 100
)

// Pattern is what a rule looks for in a description, kept normalized.
type Pattern struct {
	kind PatternKind
	text string
}

// NewPattern normalizes text (see normalize) and requires at least 3
// letters or digits and at most 100 characters.
func NewPattern(kind PatternKind, text string) (Pattern, error) {
	if _, err := ParsePatternKind(string(kind)); err != nil {
		return Pattern{}, err
	}
	t := normalize(text)
	if len([]rune(strings.ReplaceAll(t, " ", ""))) < minPattern || len([]rune(t)) > maxPattern {
		return Pattern{}, fmt.Errorf("%w: pattern %q needs %d to %d letters or digits", ErrInvalid, text, minPattern, maxPattern)
	}
	return Pattern{kind: kind, text: t}, nil
}

func (p Pattern) Kind() PatternKind { return p.kind }
func (p Pattern) Text() string      { return p.text }
func (p Pattern) IsZero() bool      { return p.text == "" }

// Matches reports whether the description, normalized the same way, is the
// pattern (exact) or has its words in a row (contains).
func (p Pattern) Matches(description string) bool {
	if p.IsZero() {
		return false
	}
	d := normalize(description)
	if p.kind == PatternExact {
		return d == p.text
	}
	return strings.Contains(" "+d+" ", " "+p.text+" ")
}

// normalize reduces a description to its words: accents dropped, upper case,
// and anything that is not a letter or a digit read as a space, so
// "Pág*iFood  SP" and "PAG IFOOD SP" are the same.
func normalize(s string) string {
	var b strings.Builder
	space := true // no leading space
	for _, r := range norm.NFD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r): // combining marks
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToUpper(r))
			space = false
		case !space:
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSuffix(b.String(), " ")
}

const maxName = 40

// cleanName trims a category name and collapses its inner spaces; it must
// have 1 to 40 characters and no control characters.
func cleanName(s string) (string, error) {
	n := strings.Join(strings.Fields(s), " ")
	if n == "" || len([]rune(n)) > maxName || strings.ContainsFunc(n, unicode.IsControl) {
		return "", fmt.Errorf("%w: category name %q needs 1 to %d characters", ErrInvalid, s, maxName)
	}
	return n, nil
}

// noise are the words banks put before the merchant ("PAG*IFOOD", "Pix
// enviado Fulano"); a suggestion skips them.
var noise = map[string]bool{
	"PAG": true, "PAGTO": true, "PAGAMENTO": true, "COMPRA": true, "COMPRAS": true, "DEBITO": true, "CREDITO": true,
	"PIX": true, "ENVIADO": true, "ENVIADA": true, "RECEBIDO": true, "RECEBIDA": true, "TRANSF": true, "TRANSFERENCIA": true,
	"TED": true, "DOC": true, "ESTORNO": true, "NO": true, "DE": true, "EM": true, "PG": true, "EC": true, "MP": true,
}

// stop are the words that end a merchant's name: what banks append after it
// (a city, a store number, a site, a status).
var stop = map[string]bool{
	"COM": true, "WWW": true, "HTTP": true, "HTTPS": true, "BR": true, "HELP": true, "PENDING": true, "PENDENTE": true,
	"LOJA": true, "FILIAL": true, "UNIDADE": true, "LTDA": true, "SA": true, "ME": true, "EIRELI": true,
	"SAO": true, "PAULO": true, "RIO": true, "JANEIRO": true, "BELO": true, "HORIZONTE": true, "CURITIBA": true,
	"PORTO": true, "ALEGRE": true, "BRASILIA": true, "SALVADOR": true, "RECIFE": true, "FORTALEZA": true, "CAMPINAS": true,
	"AC": true, "AL": true, "AM": true, "AP": true, "BA": true, "CE": true, "DF": true, "ES": true, "GO": true, "MA": true,
	"MG": true, "MS": true, "MT": true, "PA": true, "PB": true, "PE": true, "PI": true, "PR": true, "RJ": true, "RN": true,
	"RO": true, "RR": true, "RS": true, "SC": true, "SE": true, "SP": true, "TO": true,
}

const maxSuggestedWords = 3

// SuggestPattern reads the merchant out of a description as the bank sends
// it, for a correction to teach: "PAG*IFOOD SP" suggests contains IFOOD,
// "DROGASIL 2104 SAO PAULO" suggests contains DROGASIL. It skips the words
// banks put before the merchant and stops at what they append after it (a
// number, a city or state, a site), up to three words. When that leaves too
// little, it falls back to the description's first words. It reports false
// when the description has too few letters for any pattern.
func SuggestPattern(description string) (Pattern, bool) {
	words := strings.Fields(normalize(description))
	var picked []string
	for _, w := range words {
		if len(picked) == 0 && noise[w] {
			continue
		}
		if len(picked) > 0 && (stop[w] || strings.ContainsFunc(w, unicode.IsDigit)) {
			break
		}
		if len(picked) == 0 && strings.ContainsFunc(w, unicode.IsDigit) {
			continue
		}
		if len(picked) > 0 && picked[len(picked)-1] == w { // "IFOOD *IFOOD"
			continue
		}
		picked = append(picked, w)
		if len(picked) == maxSuggestedWords {
			break
		}
	}
	if p, err := NewPattern(PatternContains, strings.Join(picked, " ")); err == nil {
		return p, true
	}
	if len(words) > maxSuggestedWords {
		words = words[:maxSuggestedWords]
	}
	p, err := NewPattern(PatternContains, strings.Join(words, " "))
	return p, err == nil
}
