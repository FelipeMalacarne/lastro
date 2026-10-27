package categorize

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"lastro/modules/ledger"
	"lastro/platform/httpx"
)

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// categoryJSON is a category as the API answers it.
type categoryJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Color    Color  `json:"color"`
	Budgeted bool   `json:"budgeted"`
	Archived bool   `json:"archived"`
}

func toJSON(c Category) categoryJSON {
	return categoryJSON{ID: c.ID.String(), Name: c.Name, Color: c.Color, Budgeted: c.Budgeted, Archived: c.Archived}
}

// categoryBody is what POST and PATCH accept; a missing field is left as it
// is on PATCH.
type categoryBody struct {
	Name     *string `json:"name"`
	Color    *Color  `json:"color"`
	Budgeted *bool   `json:"budgeted"`
}

// RegisterRoutes adds categorize's endpoints to the authenticated API.
func (s *Service) RegisterRoutes(mux *http.ServeMux, logger *slog.Logger) {
	s.registerReviewRoutes(mux, logger)
	s.registerRuleRoutes(mux, logger)

	mux.HandleFunc("GET /api/v1/categories", func(w http.ResponseWriter, r *http.Request) {
		cs, err := s.Categories(r.Context())
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		out := make([]categoryJSON, len(cs))
		for i, c := range cs {
			out[i] = toJSON(c)
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"categories": out})
	})

	// POST /api/v1/categories {"name": "Pets", "color": "note-5", "budgeted": true}
	mux.HandleFunc("POST /api/v1/categories", func(w http.ResponseWriter, r *http.Request) {
		var in categoryBody
		if err := decode(w, r, &in, "name, color and budgeted"); err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		if in.Name == nil || in.Color == nil {
			httpx.WriteError(w, r, logger, httpx.BadRequest("name and color are required"))
			return
		}
		budgeted := in.Budgeted == nil || *in.Budgeted
		c, err := s.CreateCategory(r.Context(), *in.Name, *in.Color, budgeted)
		if err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		httpx.JSON(w, http.StatusCreated, map[string]any{"category": toJSON(c)})
	})

	// PATCH /api/v1/categories/{id} with any of name, color and budgeted.
	mux.HandleFunc("PATCH /api/v1/categories/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		var in categoryBody
		if err := decode(w, r, &in, "name, color and budgeted"); err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		c, err := s.UpdateCategory(r.Context(), id, CategoryChange(in))
		if err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"category": toJSON(c)})
	})

	// DELETE /api/v1/categories/{id} archives it and deletes its rules.
	mux.HandleFunc("DELETE /api/v1/categories/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		n, err := s.ArchiveCategory(r.Context(), id)
		if err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"rulesRemoved": n})
	})
}

func pathID(r *http.Request) (ledger.CategoryID, error) {
	s := r.PathValue("id")
	if !uuidRe.MatchString(s) {
		return ledger.CategoryID{}, httpx.BadRequest("id must be a category id")
	}
	return ledger.NewCategoryID(s)
}

// decode reads one small JSON object with only the known fields.
func decode(w http.ResponseWriter, r *http.Request, v any, shape string) error {
	return decodeUpTo(w, r, v, shape, 4<<10)
}

func decodeUpTo(w http.ResponseWriter, r *http.Request, v any, shape string, limit int64) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return httpx.BadRequest("the body must be a JSON object with " + shape)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return httpx.BadRequest("the body must hold a single JSON object")
	}
	return nil
}

// apiError gives the domain's errors their HTTP status.
func apiError(err error) error {
	switch {
	case errors.Is(err, ErrInvalid):
		return httpx.BadRequest(err.Error())
	case errors.Is(err, ErrNotFound):
		return httpx.NotFound("no such category")
	case errors.Is(err, ErrNameTaken):
		return httpx.Conflict("a category with this name exists")
	case errors.Is(err, ErrArchived):
		return httpx.Conflict("the category is archived")
	case errors.Is(err, ErrRuleNotFound):
		return httpx.NotFound("no such rule")
	case errors.Is(err, ErrRuleTaken):
		return httpx.Conflict("a rule with this pattern and account exists")
	case errors.Is(err, ledger.ErrNotFound):
		return httpx.NotFound("no such transaction")
	case errors.Is(err, ledger.ErrRemoved):
		return httpx.Conflict("the transaction no longer exists at the bank")
	case errors.Is(err, ledger.ErrInvalid):
		return httpx.BadRequest(err.Error())
	}
	return err
}

// patternJSON is a rule's pattern as the API reads and answers it.
type patternJSON struct {
	Kind PatternKind `json:"kind"`
	Text string      `json:"text"`
}

func (p patternJSON) parse() (Pattern, error) {
	kind := p.Kind
	if kind == "" {
		kind = PatternContains
	}
	return NewPattern(kind, p.Text)
}

type reviewTxJSON struct {
	ID          string `json:"id"`
	Date        string `json:"date"`
	Description string `json:"description"`
	AmountCents int64  `json:"amountCents"`
	AccountID   string `json:"accountId"`
}

type reviewGroupJSON struct {
	// Pattern is null when the description is too short to suggest one.
	Pattern      *patternJSON   `json:"pattern"`
	Count        int            `json:"count"`
	TotalCents   int64          `json:"totalCents"`
	Latest       string         `json:"latest"`
	Transactions []reviewTxJSON `json:"transactions"` // the newest few
	// Suggestion is the local model's answer for the merchant, when it named one.
	Suggestion *suggestionJSON `json:"suggestion"`
}

type suggestionJSON struct {
	Category   string `json:"category"`
	Confidence int    `json:"confidence"` // percent
}

// maxGroupSample is how many of a group's transactions the queue lists.
const maxGroupSample = 5

func (s *Service) registerReviewRoutes(mux *http.ServeMux, logger *slog.Logger) {
	// GET /api/v1/review: every transaction waiting for a category, grouped by
	// the rule a correction would teach.
	mux.HandleFunc("GET /api/v1/review", func(w http.ResponseWriter, r *http.Request) {
		rv, err := s.Review(r.Context())
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		groups := make([]reviewGroupJSON, len(rv.Groups))
		for i, g := range rv.Groups {
			out := reviewGroupJSON{Count: g.Count, TotalCents: g.TotalCents, Latest: g.Latest.Format(time.DateOnly)}
			if !g.Pattern.IsZero() {
				out.Pattern = &patternJSON{Kind: g.Pattern.Kind(), Text: g.Pattern.Text()}
				if sug, ok := rv.Suggestions[g.Pattern.Text()]; ok && !sug.Category.IsZero() {
					out.Suggestion = &suggestionJSON{Category: sug.Category.String(), Confidence: sug.Confidence}
				}
			}
			for _, t := range g.Transactions[:min(len(g.Transactions), maxGroupSample)] {
				out.Transactions = append(out.Transactions, reviewTxJSON{
					ID: string(t.ID), Date: t.Date.Format(time.DateOnly), Description: t.Description,
					AmountCents: t.AmountCents, AccountID: string(t.Account),
				})
			}
			groups[i] = out
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"waiting": rv.Waiting, "groups": groups})
	})

	// GET /api/v1/patterns/suggest?description=PAG*IFOOD%20SP: the rule a
	// correction of that description would teach, or null.
	mux.HandleFunc("GET /api/v1/patterns/suggest", func(w http.ResponseWriter, r *http.Request) {
		var out *patternJSON
		if p, ok := SuggestPattern(r.URL.Query().Get("description")); ok {
			out = &patternJSON{Kind: p.Kind(), Text: p.Text()}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"pattern": out})
	})

	// POST /api/v1/review {"pattern": {"kind": "contains", "text": "IFOOD"}, "category": "<id>"}
	// teaches the rule and answers how many transactions it categorized.
	mux.HandleFunc("POST /api/v1/review", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Pattern  patternJSON `json:"pattern"`
			Category string      `json:"category"`
		}
		if err := decode(w, r, &in, "pattern and category"); err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		p, err := in.Pattern.parse()
		if err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		c, err := categoryID(in.Category)
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		ch, err := s.Correct(r.Context(), p, "", c)
		if err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		httpx.JSON(w, http.StatusOK, changeJSON(ch))
	})

	// PUT /api/v1/transactions/{id}/category {"category": "<id>", "teach": {"kind": "contains", "text": "IFOOD"}}
	// is my choice for one transaction; with teach, it also becomes a rule.
	mux.HandleFunc("PUT /api/v1/transactions/{id}/category", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !uuidRe.MatchString(id) {
			httpx.WriteError(w, r, logger, httpx.BadRequest("id must be a transaction id"))
			return
		}
		var in struct {
			Category string       `json:"category"`
			Teach    *patternJSON `json:"teach"`
		}
		if err := decode(w, r, &in, "category and, optionally, teach"); err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		c, err := categoryID(in.Category)
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		var teach *Pattern
		if in.Teach != nil {
			p, err := in.Teach.parse()
			if err != nil {
				httpx.WriteError(w, r, logger, apiError(err))
				return
			}
			teach = &p
		}
		ch, err := s.SetCategory(r.Context(), ledger.TransactionID(id), c, teach)
		if err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		httpx.JSON(w, http.StatusOK, changeJSON(ch))
	})

	// PUT /api/v1/transactions/{id}/split {"parts": [{"category": "<id>", "amountCents": -41230}, ...]}
	// divides a transaction across live categories; the parts add up to its amount.
	mux.HandleFunc("PUT /api/v1/transactions/{id}/split", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !uuidRe.MatchString(id) {
			httpx.WriteError(w, r, logger, httpx.BadRequest("id must be a transaction id"))
			return
		}
		var in struct {
			Parts []struct {
				Category    string `json:"category"`
				AmountCents int64  `json:"amountCents"`
			} `json:"parts"`
		}
		if err := decode(w, r, &in, "parts"); err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		parts := make([]SplitPart, len(in.Parts))
		for i, p := range in.Parts {
			c, err := categoryID(p.Category)
			if err != nil {
				httpx.WriteError(w, r, logger, err)
				return
			}
			parts[i] = SplitPart{Category: c, AmountCents: p.AmountCents}
		}
		if err := s.Split(r.Context(), ledger.TransactionID(id), parts); err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// POST /api/v1/review/transfer {"pattern": {...}} or {"description": "..."}
	// marks a review group as transfers; answers {"marked", "ids"}.
	mux.HandleFunc("POST /api/v1/review/transfer", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Pattern     *patternJSON `json:"pattern"`
			Description string       `json:"description"`
		}
		if err := decode(w, r, &in, "pattern or description"); err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		var p Pattern
		if in.Pattern != nil {
			var err error
			if p, err = in.Pattern.parse(); err != nil {
				httpx.WriteError(w, r, logger, apiError(err))
				return
			}
		} else if in.Description == "" {
			httpx.WriteError(w, r, logger, httpx.BadRequest("pattern or description is required"))
			return
		}
		ids, err := s.MarkTransfers(r.Context(), p, in.Description)
		if err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		out := make([]string, len(ids))
		for i, id := range ids {
			out[i] = string(id)
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"marked": len(ids), "ids": out})
	})

	// POST /api/v1/undo {"rule": "<id>", "wasRule": "<category id>", "before": [...]}
	// takes back a choice: the body is the "undo" a choice answered with.
	mux.HandleFunc("POST /api/v1/undo", func(w http.ResponseWriter, r *http.Request) {
		var in undoJSON
		if err := decodeUpTo(w, r, &in, "the undo a choice answered with", 1<<20); err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		u, err := in.parse()
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		n, err := s.Undo(r.Context(), u)
		if err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"restored": n})
	})
}

// undoJSON is an Undo on the wire: a choice answers with it, and the undo
// endpoint takes it back.
type undoJSON struct {
	Rule    string       `json:"rule,omitempty"`
	WasRule string       `json:"wasRule,omitempty"`
	Before  []beforeJSON `json:"before"`
}

type beforeJSON struct {
	Transaction string `json:"transaction"`
	Category    string `json:"category,omitempty"`
	Source      string `json:"source,omitempty"`
}

func changeJSON(ch Change) map[string]any {
	u := undoJSON{Rule: string(ch.Undo.Rule), WasRule: ch.Undo.WasRule.String(), Before: []beforeJSON{}}
	for _, b := range ch.Undo.Before {
		u.Before = append(u.Before, beforeJSON{Transaction: string(b.Transaction), Category: b.Category.String(), Source: string(b.Source)})
	}
	return map[string]any{"categorized": ch.Categorized, "undo": u}
}

func (in undoJSON) parse() (Undo, error) {
	var u Undo
	if in.Rule != "" {
		if !uuidRe.MatchString(in.Rule) {
			return Undo{}, httpx.BadRequest("rule must be a rule id")
		}
		u.Rule = RuleID(in.Rule)
	}
	if in.WasRule != "" {
		c, err := categoryID(in.WasRule)
		if err != nil {
			return Undo{}, err
		}
		u.WasRule = c
	}
	if len(in.Before) > 5000 {
		return Undo{}, httpx.BadRequest("too many transactions to undo")
	}
	for _, b := range in.Before {
		if !uuidRe.MatchString(b.Transaction) {
			return Undo{}, httpx.BadRequest("before needs transaction ids")
		}
		out := Before{Transaction: ledger.TransactionID(b.Transaction)}
		if b.Category != "" {
			c, err := categoryID(b.Category)
			if err != nil {
				return Undo{}, err
			}
			src, err := ledger.ParseSource(b.Source)
			if err != nil {
				return Undo{}, httpx.BadRequest("source must be rule, llm or manual")
			}
			out.Category, out.Source = c, src
		}
		u.Before = append(u.Before, out)
	}
	return u, nil
}

func categoryID(s string) (ledger.CategoryID, error) {
	if !uuidRe.MatchString(s) {
		return ledger.CategoryID{}, httpx.BadRequest("category must be a category id")
	}
	return ledger.NewCategoryID(s)
}

type ruleJSON struct {
	ID        string      `json:"id"`
	Order     int         `json:"order"`
	Pattern   patternJSON `json:"pattern"`
	Account   *string     `json:"account"` // null: every account
	Category  string      `json:"category"`
	Meets     int         `json:"meets"`
	Decides   int         `json:"decides"`
	Last      *string     `json:"last"` // the latest transaction it meets
	Origin    Origin      `json:"origin"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

// ruleBody is what POST and PATCH accept. On PATCH a missing field stays;
// "account": "" means every account.
type ruleBody struct {
	Pattern  *patternJSON `json:"pattern"`
	Account  *string      `json:"account"`
	Category *string      `json:"category"`
}

func (b ruleBody) change() (RuleChange, error) {
	var ch RuleChange
	if b.Pattern != nil {
		p, err := b.Pattern.parse()
		if err != nil {
			return ch, apiError(err)
		}
		ch.Pattern = &p
	}
	if b.Account != nil {
		if *b.Account != "" && !uuidRe.MatchString(*b.Account) {
			return ch, httpx.BadRequest("account must be an account id, or empty for every account")
		}
		a := ledger.AccountID(*b.Account)
		ch.Account = &a
	}
	if b.Category != nil {
		c, err := categoryID(*b.Category)
		if err != nil {
			return ch, err
		}
		ch.Category = &c
	}
	return ch, nil
}

func (s *Service) registerRuleRoutes(mux *http.ServeMux, logger *slog.Logger) {
	// GET /api/v1/rules: every rule, in the order the engine tries them.
	mux.HandleFunc("GET /api/v1/rules", func(w http.ResponseWriter, r *http.Request) {
		views, err := s.Rules(r.Context())
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		out := make([]ruleJSON, len(views))
		for i, v := range views {
			j := ruleJSON{
				ID: string(v.Rule.ID()), Order: v.Order, Category: v.Rule.Category().String(),
				Pattern: patternJSON{Kind: v.Rule.Pattern().Kind(), Text: v.Rule.Pattern().Text()},
				Meets:   v.Meets, Decides: v.Decides, CreatedAt: v.Rule.CreatedAt(), UpdatedAt: v.Rule.UpdatedAt(), Origin: v.Rule.Origin(),
			}
			if a := v.Rule.Account(); a != "" {
				s := string(a)
				j.Account = &s
			}
			if !v.Last.IsZero() {
				d := v.Last.Format(time.DateOnly)
				j.Last = &d
			}
			out[i] = j
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"rules": out})
	})

	// POST /api/v1/rules {"pattern": {...}, "account": "", "category": "<id>"}
	mux.HandleFunc("POST /api/v1/rules", func(w http.ResponseWriter, r *http.Request) {
		var in ruleBody
		if err := decode(w, r, &in, "pattern, account and category"); err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		ch, err := in.change()
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		if ch.Pattern == nil || ch.Category == nil {
			httpx.WriteError(w, r, logger, httpx.BadRequest("pattern and category are required"))
			return
		}
		var account ledger.AccountID
		if ch.Account != nil {
			account = *ch.Account
		}
		done, err := s.CreateRule(r.Context(), *ch.Pattern, account, *ch.Category)
		if err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		httpx.JSON(w, http.StatusCreated, map[string]any{"categorized": done.Categorized})
	})

	// PATCH /api/v1/rules/{id} with any of pattern, account and category.
	mux.HandleFunc("PATCH /api/v1/rules/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !uuidRe.MatchString(id) {
			httpx.WriteError(w, r, logger, httpx.BadRequest("id must be a rule id"))
			return
		}
		var in ruleBody
		if err := decode(w, r, &in, "pattern, account and category"); err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		ch, err := in.change()
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		n, err := s.UpdateRule(r.Context(), RuleID(id), ch)
		if err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"categorized": n})
	})

	// DELETE /api/v1/rules/{id}
	mux.HandleFunc("DELETE /api/v1/rules/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !uuidRe.MatchString(id) {
			httpx.WriteError(w, r, logger, httpx.BadRequest("id must be a rule id"))
			return
		}
		if err := s.DeleteRule(r.Context(), RuleID(id)); err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
