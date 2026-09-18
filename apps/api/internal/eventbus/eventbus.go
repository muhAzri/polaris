// Package eventbus is Polaris's in-process pub/sub. New modules
// (notifications, completion, badges, ...) subscribe to events instead of
// every service calling into every other service directly. It's
// deliberately just a Go map of handlers plus an event_log table for later
// replay/debugging — no broker needed yet.
package eventbus

import (
	"context"
	"encoding/json"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Event struct {
	Name      string
	ContextID *string
	UserID    *string
	Data      map[string]any
}

type Handler func(ctx context.Context, e Event)

type Dispatcher struct {
	pool     *pgxpool.Pool
	handlers map[string][]Handler
}

func New(pool *pgxpool.Pool) *Dispatcher {
	return &Dispatcher{pool: pool, handlers: make(map[string][]Handler)}
}

func (d *Dispatcher) Subscribe(name string, h Handler) {
	d.handlers[name] = append(d.handlers[name], h)
}

// Dispatch persists the event to event_log and then invokes subscribers
// synchronously, in subscription order. A logging failure doesn't block
// delivery — the event still reaches handlers even if the audit trail
// write fails.
func (d *Dispatcher) Dispatch(ctx context.Context, e Event) {
	data, err := json.Marshal(e.Data)
	if err != nil {
		data = []byte("{}")
	}
	if _, err := d.pool.Exec(ctx, `
		INSERT INTO event_log (name, context_id, user_id, data)
		VALUES ($1, $2, $3, $4)
	`, e.Name, e.ContextID, e.UserID, data); err != nil {
		log.Printf("eventbus: failed to log event %s: %v", e.Name, err)
	}

	for _, h := range d.handlers[e.Name] {
		h(ctx, e)
	}
}
