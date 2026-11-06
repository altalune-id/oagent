package run

import (
	"sync"

	"altalune.id/oagent/internal/kernel/runtime"
)

// SinkCapacity is how many events a held stream buffers before a slow reader is detached.
const SinkCapacity = 256

// Observer sees every event of one turn as it streams, then the event that ends it.
type Observer func(runtime.Event)

// Sink carries one run's events to a streaming client without ever blocking the turn; a full sink detaches.
type Sink struct {
	ch       chan runtime.Event
	mu       sync.Mutex
	done     bool
	detached bool
}

// NewSink returns a sink that buffers capacity events.
func NewSink(capacity int) *Sink {
	return &Sink{ch: make(chan runtime.Event, max(capacity, 1))}
}

// Events is read by the client writer; it closes when the run settles or the sink detaches.
func (s *Sink) Events() <-chan runtime.Event { return s.ch }

// Detached reports whether the reader fell behind and the stream was cut while the run went on.
func (s *Sink) Detached() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.detached
}

// Observe hands e to the reader, or detaches the sink when its buffer is full; it never blocks.
func (s *Sink) Observe(e runtime.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return
	}
	select {
	case s.ch <- e:
	default:
		s.detached, s.done = true, true
		close(s.ch)
	}
}

// Close ends the stream; it is safe to call more than once.
func (s *Sink) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.done {
		s.done = true
		close(s.ch)
	}
}
