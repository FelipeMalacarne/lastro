package ingest

// SyncCompleted is published after every sync, whatever its outcome.
type SyncCompleted struct {
	ItemIDs []string // the items read
	Failed  []string // the items whose read failed
}

func (SyncCompleted) EventName() string { return "ingest.sync_completed" }
