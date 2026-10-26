// Package events is an in-memory, synchronous bus for domain events.
//
// Rule of use: publish only after the commit. An event describes a fact that
// already happened (TransactionsImported), never an intention.
package events

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

// Event is any domain event. Events are structs passed by value, and
// EventName must work on the zero value of the type.
type Event interface {
	EventName() string
}

type handler func(ctx context.Context, e Event) error

// Bus delivers events to subscribed handlers.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]handler
	logger   *slog.Logger
}

func NewBus(logger *slog.Logger) *Bus {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Bus{handlers: map[string][]handler{}, logger: logger}
}

// Subscribe registers a typed handler for events of type T.
func Subscribe[T Event](b *Bus, h func(ctx context.Context, e T) error) {
	var zero T
	name := zero.EventName()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[name] = append(b.handlers[name], func(ctx context.Context, e Event) error {
		te, ok := e.(T)
		if !ok {
			return fmt.Errorf("events: %s: unexpected type %T", name, e)
		}
		return h(ctx, te)
	})
}

// Publish delivers each event to every handler, in order and synchronously.
// A failing handler does not stop the others; errors are returned joined.
func (b *Bus) Publish(ctx context.Context, evs ...Event) error {
	var errs []error
	for _, e := range evs {
		b.mu.RLock()
		hs := append([]handler(nil), b.handlers[e.EventName()]...)
		b.mu.RUnlock()
		for _, h := range hs {
			if err := h(ctx, e); err != nil {
				b.logger.ErrorContext(ctx, "event handler failed", "event", e.EventName(), "err", err)
				errs = append(errs, fmt.Errorf("%s: %w", e.EventName(), err))
			}
		}
	}
	return errors.Join(errs...)
}
