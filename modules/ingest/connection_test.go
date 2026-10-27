package ingest

import (
	"testing"
	"time"
)

func TestConnectionHealth(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	fresh := ItemState{ItemID: "i", Institution: "Nubank", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		DataUpdatedAt: now.Add(-2 * time.Hour), ConsentExpiresAt: now.AddDate(0, 6, 0)}
	tests := []struct {
		name   string
		edit   func(*ItemState)
		health Health
		detail string
		short  string
	}{
		{"fresh", func(*ItemState) {}, HealthOK, "Synced 2 h ago", "2 h"},
		{"minutes", func(s *ItemState) { s.DataUpdatedAt = now.Add(-5 * time.Minute) }, HealthOK, "Synced 5 min ago", "5 min"},
		{"47 hours is still fine", func(s *ItemState) { s.DataUpdatedAt = now.Add(-47 * time.Hour) }, HealthOK, "Synced 47 h ago", "47 h"},
		{"three days", func(s *ItemState) { s.DataUpdatedAt = now.Add(-73 * time.Hour) }, HealthStale, "Last synced 3 days ago", "3 days"},
		{"never", func(s *ItemState) { s.DataUpdatedAt = time.Time{} }, HealthStale, "Not synced yet", "never"},
		{"login error", func(s *ItemState) { s.Status = "LOGIN_ERROR" }, HealthReauth, "Needs re-authentication in Meu Pluggy", "log in"},
		{"revoked", func(s *ItemState) { s.ExecutionStatus = "USER_AUTHORIZATION_REVOKED" }, HealthReauth, "Needs re-authentication in Meu Pluggy", "log in"},
		{"authorization ended", func(s *ItemState) { s.ConsentExpiresAt = now.Add(-time.Hour) }, HealthReauth, "Authorization ended. Renew it in the Pluggy Dashboard", "renew"},
		{"syncing", func(s *ItemState) { s.SyncStartedAt, s.SyncFinishedAt = now.Add(-time.Minute), now.Add(-time.Hour) }, HealthSyncing, "Syncing accounts and transactions", "now"},
		{"a dead sync is not syncing", func(s *ItemState) { s.SyncStartedAt = now.Add(-time.Hour) }, HealthOK, "Synced 2 h ago", "2 h"},
		{"a failed read of fresh data is still fresh", func(s *ItemState) { s.SyncError = "pluggy: HTTP 500" }, HealthOK, "Synced 2 h ago", "2 h"},
	}
	for _, tt := range tests {
		s := fresh
		tt.edit(&s)
		c := s.Connection(now)
		if c.Health != tt.health || c.Detail != tt.detail || c.Short != tt.short {
			t.Errorf("%s: got %s %q %q", tt.name, c.Health, c.Detail, c.Short)
		}
	}

	soon := fresh
	soon.ConsentExpiresAt = now.AddDate(0, 0, 9)
	if c := soon.Connection(now); !c.ExpiresSoon || c.Health != HealthOK {
		t.Errorf("an authorization ending in 9 days is a heads-up, not a broken connection: %+v", c)
	}
	if c := fresh.Connection(now); c.ExpiresSoon || c.ReadFailing {
		t.Error("six months away is not soon, and nothing failed")
	}
	failing := fresh
	failing.SyncedAt, failing.SyncFinishedAt, failing.SyncError = now.Add(-30*time.Hour), now.Add(-time.Hour), "pluggy: HTTP 401"
	if c := failing.Connection(now); !c.ReadFailing {
		t.Error("a last read that failed after the last success is reported")
	}
}
