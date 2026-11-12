package fakes

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"altalune.id/oagent/internal/kernel/runtime"
)

// Runtime is a runtime.Runtime that answers each turn from Replies in order: an error, or NDJSON lines to stream.
type Runtime struct {
	// BeforeTurn runs as Turn is called, before the request is recorded, to observe state at that moment.
	BeforeTurn func()
	mu         sync.Mutex
	Replies    []RuntimeReply
	Requests   []runtime.TurnRequest
}

// RuntimeReply is one turn's answer; Block makes the stream wait for the turn's context after its lines.
type RuntimeReply struct {
	Err    error
	Stream []byte
	Block  bool
	// Delay holds the first event back, for a turn that takes a while.
	Delay time.Duration
}

var _ runtime.Runtime = (*Runtime)(nil)

// Turn implements runtime.Runtime.
func (f *Runtime) Turn(ctx context.Context, req runtime.TurnRequest) (runtime.Stream, error) {
	if f.BeforeTurn != nil {
		f.BeforeTurn()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Requests = append(f.Requests, req)
	if len(f.Replies) == 0 {
		return nil, &runtime.BusyError{}
	}
	reply := f.Replies[0]
	f.Replies = f.Replies[1:]
	if reply.Err != nil {
		return nil, reply.Err
	}
	lines := bufio.NewScanner(bytes.NewReader(reply.Stream))
	lines.Buffer(make([]byte, 0, 64<<10), 16<<20)
	return &fakeStream{ctx: ctx, lines: lines, block: reply.Block, delay: reply.Delay}, nil
}

// LastRequest returns the latest turn request.
func (f *Runtime) LastRequest() runtime.TurnRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Requests[len(f.Requests)-1]
}

type fakeStream struct {
	ctx   context.Context
	lines *bufio.Scanner
	block bool
	done  bool
	delay time.Duration
}

func (s *fakeStream) Next() (runtime.Event, error) {
	if s.done {
		return nil, io.EOF
	}
	if s.delay > 0 {
		wait := time.NewTimer(s.delay)
		defer wait.Stop()
		s.delay = 0
		select {
		case <-wait.C:
		case <-s.ctx.Done():
			return nil, s.ctx.Err()
		}
	}
	for s.lines.Scan() {
		if len(bytes.TrimSpace(s.lines.Bytes())) == 0 {
			continue
		}
		e, err := runtime.DecodeEvent(s.lines.Bytes())
		if err != nil {
			return nil, err
		}
		if _, ok := e.(*runtime.Terminal); ok {
			s.done = true
		}
		return e, nil
	}
	if s.block {
		<-s.ctx.Done()
		return nil, s.ctx.Err()
	}
	return nil, &runtime.NoTerminalError{}
}

func (s *fakeStream) Close() error { return nil }

// Calls returns how many turns were requested.
func (f *Runtime) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Requests)
}
