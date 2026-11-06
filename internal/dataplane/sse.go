package dataplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"altalune.id/oagent/internal/kernel/runtime"
)

const (
	heartbeatEvery = 15 * time.Second
	writeTimeout   = 30 * time.Second
)

// IDMap rewrites the ids an event carries toward the client: the runtime's thread key becomes the client's threadId.
type IDMap struct {
	ThreadID string
}

// Filter sits between the runtime stream and the client; it may hold, rewrite or drop events (output guardrails buffer here).
type Filter interface {
	Filter(e runtime.Event) []runtime.Event
}

type passThrough struct{}

func (passThrough) Filter(e runtime.Event) []runtime.Event { return []runtime.Event{e} }

// SSEWriter writes AG-UI events as server-sent events, extending the write deadline and flushing on every write.
type SSEWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
	n  int
}

// NewSSEWriter wraps w; the caller has already sent the SSE headers.
func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	return &SSEWriter{w: w, rc: http.NewResponseController(w)}
}

// Event writes e as the next numbered event of the run.
func (s *SSEWriter) Event(e runtime.Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("dataplane: encode event: %w", err)
	}
	s.n++
	return s.write([]byte("id: " + strconv.Itoa(s.n) + "\ndata: " + string(data) + "\n\n"))
}

// Ping writes a heartbeat comment that keeps proxies from closing a quiet stream.
func (s *SSEWriter) Ping() error { return s.write([]byte(": ping\n\n")) }

func (s *SSEWriter) write(b []byte) error {
	if err := s.rc.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("dataplane: write deadline: %w", err)
	}
	if _, err := s.w.Write(b); err != nil {
		return fmt.Errorf("dataplane: write: %w", err)
	}
	if err := s.rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("dataplane: flush: %w", err)
	}
	return nil
}

// Relay writes the allow-listed AG-UI events of one run to w until events closes, pinging a quiet stream every 15 s.
func Relay(ctx context.Context, events <-chan runtime.Event, w *SSEWriter, ids IDMap, f Filter) error {
	if f == nil {
		f = passThrough{}
	}
	tick := time.NewTicker(heartbeatEvery)
	defer tick.Stop()
	ended := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			if err := w.Ping(); err != nil {
				return err
			}
		case e, ok := <-events:
			if !ok {
				if ended {
					return nil
				}
				return w.Event(&runtime.RunError{Type: runtime.EventRunError, Code: detachedCode, Message: detachedMessage})
			}
			if err := relayOne(e, w, ids, f); err != nil {
				return err
			}
			ended = ended || terminal(e)
			tick.Reset(heartbeatEvery)
		}
	}
}

func relayOne(e runtime.Event, w *SSEWriter, ids IDMap, f Filter) error {
	if _, ok := e.(*runtime.Heartbeat); ok {
		return w.Ping()
	}
	for _, out := range f.Filter(e) {
		out, ok := toClient(out, ids)
		if !ok {
			continue
		}
		if err := w.Event(out); err != nil {
			return err
		}
	}
	return nil
}

func toClient(e runtime.Event, ids IDMap) (runtime.Event, bool) {
	switch ev := e.(type) {
	case *runtime.RunStarted:
		cp := *ev
		cp.ThreadID = ids.ThreadID
		return &cp, true
	case *runtime.RunFinished:
		cp := *ev
		cp.ThreadID = ids.ThreadID
		return &cp, true
	case *runtime.TextMessageStart, *runtime.TextMessageContent, *runtime.TextMessageEnd,
		*runtime.ToolCallStart, *runtime.ToolCallArgs, *runtime.ToolCallEnd, *runtime.ToolCallResult, *runtime.RunError:
		return e, true
	default:
		return nil, false
	}
}

const (
	detachedCode    = "detached"
	detachedMessage = "The stream ended before the run did; GET the run to see how it ended"
)

func terminal(e runtime.Event) bool {
	switch e.(type) {
	case *runtime.RunFinished, *runtime.RunError:
		return true
	}
	return false
}
