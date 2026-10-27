package ingest

import (
	"log/slog"
	"net/http"
	"time"

	"lastro/modules/ledger"
	"lastro/platform/httpx"
)

// connectionJSON is a connection as the API answers it.
type connectionJSON struct {
	ItemID           string             `json:"itemId"`
	Institution      string             `json:"institution"`
	Health           Health             `json:"health"`
	Detail           string             `json:"detail"`
	Short            string             `json:"short"`
	DataUpdatedAt    *time.Time         `json:"dataUpdatedAt"`
	ConsentExpiresAt *time.Time         `json:"consentExpiresAt"`
	ExpiresSoon      bool               `json:"expiresSoon"`
	ReadFailing      bool               `json:"readFailing"`
	AccountIDs       []ledger.AccountID `json:"accountIds"`
}

// RegisterRoutes adds ingest's endpoints to the authenticated API.
func (s *Service) RegisterRoutes(mux *http.ServeMux, logger *slog.Logger) {
	mux.HandleFunc("GET /api/v1/connections", func(w http.ResponseWriter, r *http.Request) {
		conns, err := s.Connections(r.Context())
		if err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		out := make([]connectionJSON, len(conns))
		for i, c := range conns {
			out[i] = connectionJSON{
				ItemID:           c.ItemID,
				Institution:      c.Institution,
				Health:           c.Health,
				Detail:           c.Detail,
				Short:            c.Short,
				DataUpdatedAt:    timePtr(c.DataUpdatedAt),
				ConsentExpiresAt: timePtr(c.ConsentExpiresAt),
				ExpiresSoon:      c.ExpiresSoon,
				ReadFailing:      c.ReadFailing,
				AccountIDs:       append([]ledger.AccountID{}, c.AccountIDs...),
			}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"connections": out})
	})
}
