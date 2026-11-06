package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"altalune.id/oagent/internal/kernel/runtime"
	rthttp "altalune.id/oagent/internal/kernel/runtime/http"
)

const token = "rt-token"

var cases = []string{
	"plain_reply", "tool_call", "mcp_server_down", "resume", "rebuild_from_history",
	"stored_result", "limit_reached", "cancel", "model_error",
}

func golden(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "turn", name))
	require.NoError(t, err)
	return raw
}

func request(t *testing.T, c string) runtime.TurnRequest {
	t.Helper()
	req, err := runtime.DecodeRequest(golden(t, c+".request.json"))
	require.NoError(t, err)
	return *req
}

func newClient(t *testing.T, h nethttp.HandlerFunc, logs *bytes.Buffer) runtime.Runtime {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	rt, err := rthttp.New(runtime.Config{Driver: "http", URL: srv.URL, Token: token})
	require.NoError(t, err)
	if logs != nil {
		rt = rt.WithLogger(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}
	return rt
}

func drain(t *testing.T, s runtime.Stream) ([]runtime.Event, error) {
	t.Helper()
	defer s.Close()
	var out []runtime.Event
	for {
		e, err := s.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, e)
	}
}

func replay(t *testing.T, c string) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		got, err := runtime.DecodeRequest(body)
		if assert.NoError(t, err, "the body is a valid turn request") {
			want, _ := json.Marshal(request(t, c))
			have, _ := json.Marshal(got)
			assert.JSONEq(t, string(want), string(have))
		}
		assert.Equal(t, nethttp.MethodPost, r.Method)
		assert.Equal(t, "/turn", r.URL.Path)
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write(golden(t, c+".stream.ndjson"))
	}
}

func TestHTTP_ReplaysEveryGolden(t *testing.T) {
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			rt := newClient(t, replay(t, c), nil)
			s, err := rt.Turn(t.Context(), request(t, c))
			require.NoError(t, err)
			events, err := drain(t, s)
			require.NoError(t, err)
			lines := strings.Split(strings.TrimSpace(string(golden(t, c+".stream.ndjson"))), "\n")
			require.Len(t, events, len(lines))
			for i, line := range lines {
				want, err := runtime.DecodeEvent([]byte(line))
				require.NoError(t, err)
				assert.Equal(t, want, events[i], "%s line %d", c, i+1)
			}
			_, ok := events[len(events)-1].(*runtime.Terminal)
			assert.True(t, ok, "the last event is the terminal")
		})
	}
}

func TestHTTP_StatusMapping(t *testing.T) {
	for _, c := range []struct {
		status int
		body   string
		is     func(error) bool
	}{
		{400, `{"error":"invalid_request","fields":["agent.model.id"]}`, runtime.IsContractError},
		{401, ``, runtime.IsContractError},
		{413, ``, runtime.IsContractError},
		{422, `{"error":"snapshot_unloadable"}`, runtime.IsSnapshotUnloadableError},
		{422, `{"error":"other"}`, runtime.IsContractError},
		{503, ``, runtime.IsBusyError},
		{500, `boom`, runtime.IsStatusError},
	} {
		t.Run(nethttp.StatusText(c.status), func(t *testing.T) {
			rt := newClient(t, func(w nethttp.ResponseWriter, _ *nethttp.Request) {
				w.WriteHeader(c.status)
				_, _ = io.WriteString(w, c.body)
			}, nil)
			s, err := rt.Turn(t.Context(), request(t, "plain_reply"))
			assert.Nil(t, s)
			assert.True(t, c.is(err), "%d %s: %v", c.status, c.body, err)
		})
	}
}

func TestHTTP_NoTerminalIsCrash(t *testing.T) {
	stream := string(golden(t, "tool_call.stream.ndjson"))
	cut := stream[:strings.LastIndex(strings.TrimSpace(stream), "\n")+1]
	rt := newClient(t, func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		_, _ = io.WriteString(w, cut)
	}, nil)
	s, err := rt.Turn(t.Context(), request(t, "tool_call"))
	require.NoError(t, err)
	events, err := drain(t, s)
	assert.True(t, runtime.IsNoTerminalError(err), "%v", err)
	assert.NotEmpty(t, events, "the events before the cut still arrive")

	rt = newClient(t, func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		_, _ = io.WriteString(w, `{"type":"OAGENT_HEARTBEAT"}`+"\n"+`{"type":"OAGENT_NOPE"}`+"\n")
	}, nil)
	s, err = rt.Turn(t.Context(), request(t, "tool_call"))
	require.NoError(t, err)
	_, err = drain(t, s)
	assert.True(t, runtime.IsContractError(err), "an unknown private event breaks the contract: %v", err)
}

func TestHTTP_BearerSent(t *testing.T) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator()) })
	traceID, _ := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	spanID, _ := trace.SpanIDFromHex("0102030405060708")
	ctx := trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled,
	}))
	var auth, parent, ctype string
	rt := newClient(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		auth, parent, ctype = r.Header.Get("Authorization"), r.Header.Get("traceparent"), r.Header.Get("Content-Type")
		replay(t, "plain_reply")(w, r)
	}, nil)
	s, err := rt.Turn(ctx, request(t, "plain_reply"))
	require.NoError(t, err)
	_, err = drain(t, s)
	require.NoError(t, err)
	assert.Equal(t, "Bearer "+token, auth)
	assert.Equal(t, "application/json", ctype)
	assert.Contains(t, parent, "0102030405060708090a0b0c0d0e0f10", "the trace context is propagated")
}

func TestTurn_NoSecretInLogs(t *testing.T) {
	var logs bytes.Buffer
	for _, h := range []nethttp.HandlerFunc{
		replay(t, "tool_call"),
		func(w nethttp.ResponseWriter, _ *nethttp.Request) { w.WriteHeader(400) },
		func(w nethttp.ResponseWriter, _ *nethttp.Request) {
			_, _ = io.WriteString(w, `{"type":"OAGENT_HEARTBEAT"}`+"\n")
		},
	} {
		rt := newClient(t, h, &logs)
		req := request(t, "tool_call")
		s, err := rt.Turn(t.Context(), req)
		if err == nil {
			_, err = drain(t, s)
		}
		if err != nil {
			assert.NotContains(t, err.Error(), "sk-test")
			assert.NotContains(t, err.Error(), "tok-test")
		}
	}
	assert.NotEmpty(t, logs.String(), "the driver logs its refusals")
	assert.NotContains(t, logs.String(), "sk-test", "the provider key never reaches the logs")
	assert.NotContains(t, logs.String(), "tok-test", "nor the MCP credential")
}

func TestHTTP_CancelClosesRequest(t *testing.T) {
	closed := make(chan struct{})
	rt := newClient(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		_, _ = io.WriteString(w, `{"type":"OAGENT_HEARTBEAT"}`+"\n")
		w.(nethttp.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}, nil)
	ctx, cancel := context.WithCancel(t.Context())
	s, err := rt.Turn(ctx, request(t, "plain_reply"))
	require.NoError(t, err)
	e, err := s.Next()
	require.NoError(t, err)
	assert.IsType(t, &runtime.Heartbeat{}, e)
	cancel()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the runtime never saw the request close")
	}
	_, err = s.Next()
	assert.Error(t, err)
	assert.NoError(t, s.Close())
}

func TestNew_RefusesABadConfig(t *testing.T) {
	_, err := rthttp.New(runtime.Config{Driver: "http"})
	assert.Error(t, err, "a URL is needed")
	_, err = rthttp.New(runtime.Config{Driver: "http", URL: "::nope"})
	assert.Error(t, err)
}

func TestHTTP_RedirectIsNeverFollowed(t *testing.T) {
	var replayed atomic.Bool
	mux := nethttp.NewServeMux()
	mux.HandleFunc("/turn", func(w nethttp.ResponseWriter, r *nethttp.Request) {
		nethttp.Redirect(w, r, "/elsewhere", nethttp.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/elsewhere", func(nethttp.ResponseWriter, *nethttp.Request) { replayed.Store(true) })
	rt := newClient(t, mux.ServeHTTP, nil)
	_, err := rt.Turn(t.Context(), request(t, "plain_reply"))
	assert.True(t, runtime.IsStatusError(err), "%v", err)
	assert.False(t, replayed.Load(), "a 307 never replays the secret body elsewhere")
}

func TestHTTP_LineOverTheCapBreaksTheContract(t *testing.T) {
	huge := `{"type":"OAGENT_HEARTBEAT","pad":"` + strings.Repeat("x", 8<<20) + `"}` + "\n"
	rt := newClient(t, func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		_, _ = io.WriteString(w, huge)
	}, nil)
	s, err := rt.Turn(t.Context(), request(t, "plain_reply"))
	require.NoError(t, err)
	_, err = drain(t, s)
	assert.True(t, runtime.IsContractError(err), "a line over 8 MiB is refused before it is decoded: %v", err)
	assert.ErrorIs(t, err, rthttp.ErrLineTooLong)
}

func TestHTTP_UnknownStandardTypeIsSkipped(t *testing.T) {
	lines := strings.SplitAfter(string(golden(t, "plain_reply.stream.ndjson")), "\n")
	withRaw := lines[0] + `{"type":"RAW","event":{"vendor":"strands"},"source":"strands"}` + "\n" +
		`{"type":"STATE_DELTA","delta":[]}` + "\n" + strings.Join(lines[1:], "")
	var logs bytes.Buffer
	rt := newClient(t, func(w nethttp.ResponseWriter, _ *nethttp.Request) { _, _ = io.WriteString(w, withRaw) }, &logs)
	s, err := rt.Turn(t.Context(), request(t, "plain_reply"))
	require.NoError(t, err)
	events, err := drain(t, s)
	require.NoError(t, err, "a standard AG-UI type Go does not relay is skipped, and the turn completes")
	_, ok := events[len(events)-1].(*runtime.Terminal)
	assert.True(t, ok)
	for _, e := range events {
		assert.NotEqual(t, runtime.EventType("RAW"), e.EventType())
	}
	assert.Contains(t, logs.String(), "RAW", "the skip is logged at Debug")
}

func TestHTTP_UnknownPrivateTypeFailsTheTurn(t *testing.T) {
	rt := newClient(t, func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		_, _ = io.WriteString(w, `{"type":"OAGENT_HEARTBEAT"}`+"\n"+`{"type":"OAGENT_FOO","x":1}`+"\n")
	}, nil)
	s, err := rt.Turn(t.Context(), request(t, "plain_reply"))
	require.NoError(t, err)
	_, err = drain(t, s)
	assert.True(t, runtime.IsContractError(err), "an unknown private event could carry a checkpoint, so it is never dropped: %v", err)
}

func TestHTTP_UnknownFieldOnAKnownTypeStaysStrict(t *testing.T) {
	rt := newClient(t, func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		_, _ = io.WriteString(w, `{"type":"OAGENT_HEARTBEAT","extra":true}`+"\n")
	}, nil)
	s, err := rt.Turn(t.Context(), request(t, "plain_reply"))
	require.NoError(t, err)
	_, err = drain(t, s)
	assert.True(t, runtime.IsContractError(err), "%v", err)
}
