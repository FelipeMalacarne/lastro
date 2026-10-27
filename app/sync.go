package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"lastro/modules/ingest"
	"lastro/platform/httpx"
)

// ErrSyncRunning means a sync started from the app is still running.
var ErrSyncRunning = errors.New("app: a sync is running")

// syncState is the sync started from the screens: one at a time, run in the
// background, its last outcome kept for the screens to read.
type syncState struct {
	mu       sync.Mutex
	running  bool
	started  time.Time
	finished time.Time
	err      string
	summary  string
}

// SyncStatus is what the screens show about the sync started from them.
type SyncStatus struct {
	Running  bool    `json:"running"`
	Started  *string `json:"started"`
	Finished *string `json:"finished"`
	OK       bool    `json:"ok"`
	Message  string  `json:"message"`
}

// SyncNow reads every item from Pluggy, then the market, values the day and
// sends what is due, as `lastrod sync` does on its schedule. It runs in the
// background and returns at once; ErrSyncRunning when one already runs.
func (a *App) SyncNow() error {
	if err := a.Config.Pluggy.Validate(); err != nil {
		return fmt.Errorf("app: sync now: %w", err)
	}
	s := &a.sync
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrSyncRunning
	}
	s.running, s.started = true, time.Now()
	s.mu.Unlock()

	go func() {
		// Detached from the request: the sync outlives it.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		summary, err := a.syncAll(ctx)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.running, s.finished, s.summary, s.err = false, time.Now(), summary, ""
		if err != nil {
			s.err = err.Error()
			a.Logger.WarnContext(ctx, "sync from the app failed", "err", err)
		}
	}()
	return nil
}

func (a *App) syncAll(ctx context.Context) (string, error) {
	rep, err := a.Ingest.Sync(ctx, ingest.SyncOptions{})
	if err != nil {
		return "", err
	}
	created, updated, failed := 0, 0, 0
	for _, it := range rep.Items {
		if it.Err != nil {
			failed++
			continue
		}
		created += it.Created
		updated += it.Updated
	}
	if _, err := a.RefreshMarket(ctx); err != nil {
		a.Logger.WarnContext(ctx, "market refresh failed", "err", err)
	}
	if _, err := a.SendAlerts(ctx); err != nil {
		a.Logger.WarnContext(ctx, "alerts not sent", "err", err)
	}
	if _, err := a.SendReport(ctx); err != nil {
		a.Logger.WarnContext(ctx, "monthly report not sent", "err", err)
	}
	msg := fmt.Sprintf("%d new and %d updated transactions", created, updated)
	if failed > 0 {
		msg += fmt.Sprintf("; %d %s failed", failed, plural(failed, "bank", "banks"))
	}
	return msg, nil
}

// Status is the last sync started from the app.
func (s *syncState) status() SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := SyncStatus{Running: s.running, OK: s.err == "", Message: s.summary}
	if s.err != "" {
		st.Message = s.err
	}
	if !s.started.IsZero() {
		st.Started = ptr(s.started.UTC().Format(time.RFC3339))
	}
	if !s.finished.IsZero() {
		st.Finished = ptr(s.finished.UTC().Format(time.RFC3339))
	}
	return st
}

func ptr[T any](v T) *T { return &v }

// POST /api/v1/sync starts a sync (202), 409 while one runs, 503 without
// Pluggy credentials; GET /api/v1/sync says how the last one went.
func (a *App) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		httpx.JSON(w, http.StatusOK, a.sync.status())
		return
	}
	switch err := a.SyncNow(); {
	case errors.Is(err, ErrSyncRunning):
		httpx.WriteError(w, r, a.Logger, httpx.Conflict("a sync is already running"))
	case err != nil:
		httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{"code": "unavailable", "message": "Pluggy is not set up on this server: " + err.Error()})
	default:
		httpx.JSON(w, http.StatusAccepted, a.sync.status())
	}
}
