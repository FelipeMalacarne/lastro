package categorize

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"

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
		if err := decode(w, r, &in); err != nil {
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
		if err := decode(w, r, &in); err != nil {
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
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return httpx.BadRequest("the body must be a JSON object with name, color and budgeted")
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
	}
	return err
}
