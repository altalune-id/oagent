package fakes

import (
	"context"
	"sync"

	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
)

// Published is one event a Publisher saw, with the tenant and whether a unit of work was open.
type Published struct {
	Type   events.Type
	Data   any
	Tenant tenant.Context
	InTx   bool
}

// Publisher records every event it is given; Err fails every call.
type Publisher struct {
	Err    error
	mu     sync.Mutex
	events []Published
}

// Enqueue records the event.
func (p *Publisher) Enqueue(ctx context.Context, t events.Type, data any) error {
	if p.Err != nil {
		return p.Err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, Published{Type: t, Data: data, Tenant: tenant.Peek(ctx), InTx: InTx(ctx)})
	return nil
}

// Events returns a copy of what was published.
func (p *Publisher) Events() []Published {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Published(nil), p.events...)
}
