//go:build integration

package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"lastro/modules/ingest/internal/fixtures"
	"lastro/modules/ingest/internal/pluggy"
	"lastro/modules/ledger"
	"lastro/platform/db/dbtest"
	"lastro/platform/events"
)

// fakePluggy answers from the fixtures. drop hides transactions, as if
// Pluggy had stopped returning them.
type fakePluggy struct {
	set   fixtures.Set
	drop  map[string]bool
	reads []time.Time
}

func (f *fakePluggy) Item(_ context.Context, id string) (pluggy.Item, error) {
	for _, it := range f.set.Items {
		if it.ID == id {
			return it, nil
		}
	}
	return pluggy.Item{}, &pluggy.Error{Status: 404, Message: "item not found"}
}

func (f *fakePluggy) Accounts(_ context.Context, itemID string) ([]pluggy.Account, error) {
	return f.set.Accounts[itemID], nil
}

func (f *fakePluggy) Transactions(_ context.Context, accountID string, from time.Time) ([]pluggy.Transaction, error) {
	f.reads = append(f.reads, from)
	var out []pluggy.Transaction
	for _, t := range f.set.Transactions[accountID] {
		d, _ := parseDate(t.Date)
		if f.drop[t.ID] || (!from.IsZero() && d.Before(from.Truncate(24*time.Hour))) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func (f *fakePluggy) Investments(_ context.Context, itemID string) ([]pluggy.Investment, error) {
	return f.set.Investments[itemID], nil
}

func newSync(t *testing.T) (*Service, *fakePluggy, *ledger.Service) {
	t.Helper()
	pool := dbtest.New(t)
	set, err := fixtures.Load("testdata/pluggy")
	if err != nil {
		t.Fatal(err)
	}
	led := ledger.New(ledger.NewRepository(pool), nil)
	fake := &fakePluggy{set: set, drop: map[string]bool{}}
	s := &Service{
		repo:    NewRepository(pool),
		pluggy:  fake,
		ledger:  led,
		itemIDs: []string{"a0000000-0000-4000-8000-000000000001", "a0000000-0000-4000-8000-000000000002"},
		bus:     nil,
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		now:     func() time.Time { return time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC) },
	}
	s.bus = events.NewBus(nil)
	return s, fake, led
}

func TestSyncEndToEnd(t *testing.T) {
	ctx := context.Background()
	s, fake, _ := newSync(t)

	rep, err := s.Sync(ctx, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Items) != 2 || rep.Items[0].Institution != "Nubank" || rep.Items[0].Accounts != 2 || rep.Items[0].Created != 14 || rep.Items[1].Created != 4 {
		t.Fatalf("first sync: %+v", rep.Items)
	}
	if rep.Items[0].Investments != 1 {
		t.Errorf("investments are stored: %+v", rep.Items[0])
	}
	if fake.reads[0] != (time.Time{}) {
		t.Errorf("the first sync reads everything, read from %v", fake.reads[0])
	}
	if rep.Paired != 2 {
		t.Errorf("the Pix from Nubank to Itaú and the card bill paid from checking pair: %d", rep.Paired)
	}

	// The next sync reads a window and changes nothing.
	fake.reads = nil
	rep, err = s.Sync(ctx, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Items[0].Unchanged != 14 || rep.Items[0].Created+rep.Items[0].Updated+rep.Items[0].Removed != 0 {
		t.Fatalf("second sync must change nothing: %+v", rep.Items[0])
	}
	if want := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC); !fake.reads[0].Equal(want) {
		t.Errorf("incremental reads start 90 days back: %v, want %v", fake.reads[0], want)
	}

	// The pending Uber ride vanishes from Pluggy.
	fake.drop["c0000000-0000-4000-8000-000000000010"] = true
	rep, err = s.Sync(ctx, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Items[0].Removed != 1 {
		t.Fatalf("a vanished pending purchase is removed: %+v", rep.Items[0])
	}

	// Reprocessing from the raw records reproduces the same ledger.
	rep, err = s.Sync(ctx, SyncOptions{Reprocess: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := rep.Items[0]; got.Created+got.Updated+got.Removed != 0 || got.Unchanged != 13 {
		t.Fatalf("reprocess must change nothing: %+v", got)
	}

	conns, err := s.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 2 {
		t.Fatalf("want 2 connections, got %+v", conns)
	}
	byInst := map[string]ConnectionView{}
	for _, c := range conns {
		byInst[c.Institution] = c
	}
	if c := byInst["Nubank"]; c.Health != HealthOK || len(c.AccountIDs) != 2 {
		t.Errorf("Nubank: %+v", c)
	}
	if c := byInst["Itaú"]; c.Health != HealthStale || c.Detail != "Last synced 3 days ago" || len(c.AccountIDs) != 1 {
		t.Errorf("Itaú's data is from 23 Oct: %+v", c)
	}
}

func TestSyncRecordsFailures(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newSync(t)
	s.itemIDs = append(s.itemIDs, "a0000000-0000-4000-8000-00000000dead")
	rep, err := s.Sync(ctx, SyncOptions{})
	if err == nil || len(rep.Items) != 3 || rep.Items[2].Err == nil || rep.Items[0].Err != nil {
		t.Fatalf("one bad item fails the sync but not the others: %v %+v", err, rep.Items)
	}
	st, err := s.repo.item(ctx, "a0000000-0000-4000-8000-00000000dead")
	if err != nil || st.SyncError == "" || !st.SyncedAt.IsZero() {
		t.Fatalf("the failure is recorded: %+v %v", st, err)
	}
}

func TestSyncRefusesAnExpiredAuthorization(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newSync(t)
	s.now = func() time.Time { return time.Date(2027, 10, 1, 0, 0, 0, 0, time.UTC) }
	rep, err := s.Sync(ctx, SyncOptions{})
	if err == nil || rep.Items[0].Accounts != 0 {
		t.Fatalf("an expired authorization reads nothing: %v %+v", err, rep.Items)
	}
	conns, _ := s.Connections(ctx)
	if conns[0].Health != HealthReauth {
		t.Errorf("and the connection says so: %+v", conns[0])
	}
}

func TestOnlyOneSyncAtATime(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newSync(t)
	unlock, ok, err := s.repo.lock(ctx)
	if err != nil || !ok {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := s.Sync(ctx, SyncOptions{}); !errors.Is(err, ErrSyncRunning) {
		t.Fatalf("want ErrSyncRunning, got %v", err)
	}
}
