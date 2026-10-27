package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSyncWithoutPluggy(t *testing.T) {
	a := &App{}
	w := httptest.NewRecorder()
	a.handleSync(w, httptest.NewRequest(http.MethodPost, "/api/v1/sync", nil))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "LASTRO_PLUGGY") {
		t.Fatalf("without credentials the server says what is missing: %d %s", w.Code, w.Body)
	}
	w = httptest.NewRecorder()
	a.handleSync(w, httptest.NewRequest(http.MethodGet, "/api/v1/sync", nil))
	var st SyncStatus
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil || st.Running || st.Started != nil {
		t.Fatalf("nothing ran: %+v %v", st, err)
	}
}

func TestSyncOneAtATime(t *testing.T) {
	a := &App{}
	a.Config.Pluggy.ClientID, a.Config.Pluggy.ClientSecret, a.Config.Pluggy.ItemIDs = "id", "secret", []string{"item"}
	a.sync.running = true
	if err := a.SyncNow(); !errors.Is(err, ErrSyncRunning) {
		t.Fatalf("a second sync waits for the first: %v", err)
	}
}
