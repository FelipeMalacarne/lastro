package ingest

import (
	"fmt"
	"time"
)

// Health is how fresh a connection's data is, in the four states the
// SyncStatus component shows.
type Health string

const (
	HealthOK      Health = "ok"
	HealthSyncing Health = "syncing"
	HealthStale   Health = "stale"
	HealthReauth  Health = "reauth"
)

const (
	// staleAfter is how old data may get before screens warn about it.
	// Meu Pluggy refreshes every 24 hours, so two days means a missed day.
	staleAfter = 48 * time.Hour
	// syncTimeout is how long a sync may look running before it is assumed dead.
	syncTimeout = 15 * time.Minute
	// expiryNotice is how early to warn that the Meu Pluggy authorization ends.
	expiryNotice = 14 * 24 * time.Hour
)

// Pluggy statuses and execution statuses that only I can fix, by logging in
// again in Meu Pluggy or renewing the authorization in the Pluggy Dashboard.
var reauthStates = map[string]bool{
	"LOGIN_ERROR":                    true,
	"WAITING_USER_INPUT":             true,
	"INVALID_CREDENTIALS":            true,
	"INVALID_CREDENTIALS_MFA":        true,
	"ACCOUNT_LOCKED":                 true,
	"ACCOUNT_NEEDS_ACTION":           true,
	"ACCOUNT_CREDENTIALS_RESET":      true,
	"USER_AUTHORIZATION_PENDING":     true,
	"USER_AUTHORIZATION_NOT_GRANTED": true,
	"USER_AUTHORIZATION_REVOKED":     true,
	"USER_AUTHORIZATION_EXPIRED":     true,
}

// ItemState is what ingest knows about one item: Pluggy's view of the
// connection and lastro's own reads of it.
type ItemState struct {
	ItemID           string
	Institution      string
	Status           string
	ExecutionStatus  string
	ErrorCode        string
	DataUpdatedAt    time.Time // when Pluggy last refreshed the bank's data
	ConsentExpiresAt time.Time
	SyncStartedAt    time.Time
	SyncFinishedAt   time.Time
	SyncedAt         time.Time // lastro's last read that succeeded
	SyncError        string
}

// Connection is the health of one item as the screens show it.
type Connection struct {
	ItemID      string
	Institution string
	Health      Health
	// Detail is the sentence for SyncStatus: "Synced 2 h ago",
	// "Last synced 3 days ago", "Needs re-authentication in Meu Pluggy".
	Detail string
	// Short is the compact detail for the sidebar: "2 h", "3 days".
	Short            string
	DataUpdatedAt    time.Time
	ConsentExpiresAt time.Time
	// ExpiresSoon is set when the Meu Pluggy authorization ends within two weeks.
	ExpiresSoon bool
	// ReadFailing is set when lastro's own last read of Pluggy failed: then
	// the fix is on lastro's side (credentials, network), not in Meu Pluggy.
	ReadFailing bool
}

// Health decides the connection's state at now. Freshness is about the
// bank's data (DataUpdatedAt), not about when lastro last read Pluggy: a read
// of data Meu Pluggy stopped refreshing is still stale.
func (s ItemState) Connection(now time.Time) Connection {
	c := Connection{
		ItemID:           s.ItemID,
		Institution:      s.Institution,
		DataUpdatedAt:    s.DataUpdatedAt,
		ConsentExpiresAt: s.ConsentExpiresAt,
	}
	if c.Institution == "" {
		c.Institution = "A connection"
	}
	expired := !s.ConsentExpiresAt.IsZero() && !now.Before(s.ConsentExpiresAt)
	c.ExpiresSoon = !expired && !s.ConsentExpiresAt.IsZero() && s.ConsentExpiresAt.Sub(now) < expiryNotice
	c.ReadFailing = s.SyncError != "" && !s.SyncFinishedAt.IsZero() && s.SyncedAt.Before(s.SyncFinishedAt)
	age := now.Sub(s.DataUpdatedAt)

	switch {
	case expired:
		c.Health, c.Short = HealthReauth, "renew"
		c.Detail = "Authorization ended. Renew it in the Pluggy Dashboard"
	case reauthStates[s.Status] || reauthStates[s.ExecutionStatus] || reauthStates[s.ErrorCode]:
		c.Health, c.Short = HealthReauth, "log in"
		c.Detail = "Needs re-authentication in Meu Pluggy"
	case s.running(now):
		c.Health, c.Short = HealthSyncing, "now"
		c.Detail = "Syncing accounts and transactions"
	case s.DataUpdatedAt.IsZero():
		c.Health, c.Short = HealthStale, "never"
		c.Detail = "Not synced yet"
	case age > staleAfter:
		c.Health, c.Short = HealthStale, ago(age)
		c.Detail = "Last synced " + ago(age) + " ago"
	default:
		c.Health, c.Short = HealthOK, ago(age)
		c.Detail = "Synced " + ago(age) + " ago"
	}
	return c
}

func (s ItemState) running(now time.Time) bool {
	return !s.SyncStartedAt.IsZero() && s.SyncFinishedAt.Before(s.SyncStartedAt) && now.Sub(s.SyncStartedAt) < syncTimeout
}

// ago formats a duration the way sync freshness is written: "5 min", "2 h", "3 days".
func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "1 min"
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	}
	return fmt.Sprintf("%d days", int(d.Hours()/24))
}
