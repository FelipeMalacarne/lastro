package events

import (
	"context"
	"errors"
	"testing"
)

type imported struct{ Count int }

func (imported) EventName() string { return "test.imported" }

type other struct{}

func (other) EventName() string { return "test.other" }

func TestPublishDeliversToTypedHandlers(t *testing.T) {
	bus := NewBus(nil)
	var got []int
	Subscribe(bus, func(_ context.Context, e imported) error { got = append(got, e.Count); return nil })
	Subscribe(bus, func(_ context.Context, e imported) error { got = append(got, e.Count*10); return nil })

	if err := bus.Publish(context.Background(), imported{Count: 2}, other{}); err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 2 || got[0] != 2 || got[1] != 20 {
		t.Fatalf("handlers received %v", got)
	}
}

func TestPublishKeepsGoingAfterAnError(t *testing.T) {
	bus := NewBus(nil)
	boom := errors.New("boom")
	called := false
	Subscribe(bus, func(context.Context, imported) error { return boom })
	Subscribe(bus, func(context.Context, imported) error { called = true; return nil })

	err := bus.Publish(context.Background(), imported{})
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if !called {
		t.Fatal("the second handler should have run")
	}
}
