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
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
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
		n, err := s.Correct(r.Context(), p, "", c)
		if err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"categorized": n})
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
		n, err := s.SetCategory(r.Context(), ledger.TransactionID(id), c, teach)
		if err != nil {
			httpx.WriteError(w, r, logger, apiError(err))
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"categorized": n})
	})
}

func categoryID(s string) (ledger.CategoryID, error) {
	if !uuidRe.MatchString(s) {
		return ledger.CategoryID{}, httpx.BadRequest("category must be a category id")
	}
	return ledger.NewCategoryID(s)
}
