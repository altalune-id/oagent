package dataplane_test

import (
	"bufio"
	"bytes"
	"context"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/dataplane"
	"altalune.id/oagent/internal/kernel/runtime"
)

func goldenEvents(t *testing.T, c string) []runtime.Event {
	t.Helper()
	var out []runtime.Event
	sc := bufio.NewScanner(bytes.NewReader(readGolden(t, c+".ndjson")))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		if strings.Contains(sc.Text(), `"type":"RAW"`) {
			continue
		}
		e, err := runtime.DecodeEvent(sc.Bytes())
		require.NoError(t, err)
		out = append(out, e)
	}
	switch c {
	case "cancelled":
		out = append(out, &runtime.RunError{Type: runtime.EventRunError, Message: "The run was cancelled.", Code: "cancelled"})
	case "interrupted":
		out = append(out, &runtime.RunError{
			Type: runtime.EventRunError, Code: "interrupted",
			Message: "The run was interrupted and resumes shortly; reload the thread for its result.",
		})
	}
	return out
}

func relayAll(t *testing.T, events []runtime.Event) []byte {
	t.Helper()
	ch := make(chan runtime.Event, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	rec := httptest.NewRecorder()
	require.NoError(t, dataplane.Relay(t.Context(), ch, dataplane.NewSSEWriter(rec), dataplane.IDMap{ThreadID: clientThread}, nil))
	return rec.Body.Bytes()
}

func TestRelay_MatchesEveryGolden(t *testing.T) {
	for _, c := range sseCases {
		if c == "replay_settled" {
			continue
		}
		t.Run(c, func(t *testing.T) {
			assert.Equal(t, string(readGolden(t, c+".sse")), string(relayAll(t, goldenEvents(t, c))))
		})
	}
}

func TestRelay_DropsUnknownTypes(t *testing.T) {
	out := relayAll(t, []runtime.Event{
		&runtime.RunStarted{Type: runtime.EventRunStarted, ThreadID: "t0193a2b4c5d67e8f9a0b1c2d3e4f5a6b", RunID: runPublicID},
		&runtime.Checkpoint{Type: runtime.EventCheckpoint, ToolUseID: "call_1"},
		&runtime.Terminal{Type: runtime.EventTerminal, Status: "completed"},
		&runtime.RunFinished{Type: runtime.EventRunFinished, ThreadID: "t0193a2b4c5d67e8f9a0b1c2d3e4f5a6b", RunID: runPublicID},
	})
	assert.Equal(t, 2, strings.Count(string(out), "data: "), "only the allow-listed events")
	assert.NotContains(t, string(out), "OAGENT_")
	assert.Contains(t, string(out), "id: 2\n", "ids count relayed events only")
}

func TestRelay_NoUUIDOrSessionIDLeaves(t *testing.T) {
	out := string(relayAll(t, goldenEvents(t, "tool_call")))
	assert.NotRegexp(t, regexp.MustCompile(`t[0-9a-f]{32}`), out)
	assert.NotRegexp(t, regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-`), out)
	assert.Contains(t, out, `"threadId":"`+clientThread+`"`)
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	mu        sync.Mutex
	deadlines []time.Time
	writes    []time.Time
}

func (d *deadlineRecorder) SetWriteDeadline(t time.Time) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deadlines = append(d.deadlines, t)
	return nil
}

func (d *deadlineRecorder) Write(b []byte) (int, error) {
	d.mu.Lock()
	d.writes = append(d.writes, time.Now())
	d.mu.Unlock()
	return d.ResponseRecorder.Write(b)
}

func TestSSEWriter_HeartbeatAndDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
		ch := make(chan runtime.Event)
		done := make(chan error, 1)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			done <- dataplane.Relay(ctx, ch, dataplane.NewSSEWriter(rec), dataplane.IDMap{ThreadID: clientThread}, nil)
		}()
		time.Sleep(31 * time.Second)
		synctest.Wait()
		rec.mu.Lock()
		pings := strings.Count(rec.Body.String(), ": ping\n\n")
		rec.mu.Unlock()
		assert.Equal(t, 2, pings, "a quiet stream gets a ping every 15 s")
		ch <- &runtime.RunFinished{Type: runtime.EventRunFinished, ThreadID: "x", RunID: runPublicID}
		close(ch)
		require.NoError(t, <-done)
		rec.mu.Lock()
		defer rec.mu.Unlock()
		require.Len(t, rec.deadlines, 3, "every write extends the deadline")
		for i, d := range rec.deadlines {
			assert.Equal(t, 30*time.Second, d.Sub(rec.writes[i]), "the deadline is now + 30 s before write %d", i)
		}
		assert.True(t, rec.Flushed)
	})
}

func TestRelay_StopsWhenTheClientIsGone(t *testing.T) {
	ch := make(chan runtime.Event)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := dataplane.Relay(ctx, ch, dataplane.NewSSEWriter(httptest.NewRecorder()), dataplane.IDMap{ThreadID: clientThread}, nil)
	assert.ErrorIs(t, err, context.Canceled)
}

type upper struct{}

func (upper) Filter(e runtime.Event) []runtime.Event {
	if c, ok := e.(*runtime.TextMessageContent); ok {
		cp := *c
		cp.Delta = strings.ToUpper(cp.Delta)
		return []runtime.Event{&cp}
	}
	return []runtime.Event{e}
}

func TestRelay_FilterSeesEveryRelayedEvent(t *testing.T) {
	ch := make(chan runtime.Event, 1)
	ch <- &runtime.TextMessageContent{Type: runtime.EventTextMessageContent, MessageID: "m", Delta: "hi"}
	close(ch)
	rec := httptest.NewRecorder()
	require.NoError(t, dataplane.Relay(t.Context(), ch, dataplane.NewSSEWriter(rec), dataplane.IDMap{ThreadID: clientThread}, upper{}))
	assert.Contains(t, rec.Body.String(), `"delta":"HI"`)
}

func TestRelay_AStreamWithNoTerminalEndsDetached(t *testing.T) {
	ch := make(chan runtime.Event, 2)
	ch <- &runtime.RunStarted{Type: runtime.EventRunStarted, ThreadID: "t0193a2b4c5d67e8f9a0b1c2d3e4f5a6b", RunID: runPublicID}
	ch <- &runtime.TextMessageContent{Type: runtime.EventTextMessageContent, MessageID: "m", Delta: "hi"}
	close(ch)
	rec := httptest.NewRecorder()
	require.NoError(t, dataplane.Relay(t.Context(), ch, dataplane.NewSSEWriter(rec), dataplane.IDMap{ThreadID: clientThread}, nil))
	out := rec.Body.String()
	last := out[strings.LastIndex(strings.TrimSpace(out), "data: "):]
	assert.Contains(t, last, `"type":"RUN_ERROR"`)
	assert.Contains(t, last, `"code":"detached"`, "the client learns the stream ended before the run did")
	assert.Contains(t, last, "GET", "and how to find out how it ended")
	assert.Equal(t, 1, strings.Count(out, "RUN_ERROR"))

	done := httptest.NewRecorder()
	finished := make(chan runtime.Event, 1)
	finished <- &runtime.RunFinished{Type: runtime.EventRunFinished, ThreadID: "t0193a2b4c5d67e8f9a0b1c2d3e4f5a6b", RunID: runPublicID}
	close(finished)
	require.NoError(t, dataplane.Relay(t.Context(), finished, dataplane.NewSSEWriter(done), dataplane.IDMap{ThreadID: clientThread}, nil))
	assert.NotContains(t, done.Body.String(), "detached", "a stream that ended is left as it is")
}
