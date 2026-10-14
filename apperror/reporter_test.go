package apperror

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

type mappedErr struct{}

func (mappedErr) Error() string         { return "mapped" }
func (mappedErr) ToAppError() *AppError { return New(CodeValidation, "mapped") }

func TestAsAppError_ToAppError(t *testing.T) {
	ae, ok := AsAppError(mappedErr{})
	require.True(t, ok)
	assert.Equal(t, CodeValidation, ae.Code())

	ae, ok = AsAppError(fmt.Errorf("wrap: %w", mappedErr{}))
	require.True(t, ok)
	assert.Equal(t, CodeValidation, ae.Code())
}

type countingCounter struct{ codes []string }

func (c *countingCounter) Inc(_ context.Context, code string) { c.codes = append(c.codes, code) }

func TestReporter_IncidentCounter(t *testing.T) {
	c := &countingCounter{}
	r := NewReporter(nil, false, WithIncidentCounter(c))

	r.Unexpected(context.Background(), "x", errors.New("boom"))
	assert.Equal(t, []string{string(CodeUnexpected)}, c.codes)
}

func TestTruncateCause(t *testing.T) {
	assert.Equal(t, "abc", TruncateCause("abcdef", 3))
	assert.Equal(t, "abc", TruncateCause("abc", 10))
	assert.Equal(t, "", TruncateCause("é", 1)) // split multibyte rune is dropped
	assert.Equal(t, "", TruncateCause("x", 0))
	assert.Equal(t, "", TruncateCause("x", -1))
}

func TestAppError(t *testing.T) {
	base := errors.New("boom")
	e := New(CodeUnexpected, "oops").WithCause(base)

	assert.Equal(t, CodeUnexpected, e.Code())
	assert.Equal(t, "oops", e.Error())
	assert.Equal(t, "oops", e.Message())
	assert.ErrorIs(t, e, base)

	got, ok := AsAppError(e)
	assert.True(t, ok)
	assert.Same(t, e, got)

	_, ok = AsAppError(base)
	assert.False(t, ok)
}

func TestAppError_Meta(t *testing.T) {
	e := New(CodeUnexpected, "x").WithMeta(map[string]string{"a": "1"})
	assert.Equal(t, "1", e.Meta()["a"])

	got := e.Meta()
	got["a"] = "2"
	assert.Equal(t, "1", e.Meta()["a"]) // defensive copy

	assert.Same(t, e, e.WithCause(nil))
	assert.Same(t, e, e.WithMeta(nil))
}

type recordingSink struct{ inc *Incident }

func (s *recordingSink) Report(_ context.Context, inc Incident) { s.inc = &inc }

func TestReporter_Unexpected(t *testing.T) {
	sink := &recordingSink{}
	r := NewReporter(slog.New(slog.NewTextHandler(discard{}, nil)), false, WithSinks(sink))

	cause := errors.New("db down")
	out := r.Unexpected(context.Background(), "org: create", cause, "org_id", "o1")

	assert.Equal(t, CodeUnexpected, out.Code())
	assert.Contains(t, out.Error(), "db down") // not production → cause surfaced
	assert.Equal(t, "db down", out.Meta()["underlying_error"])

	require.NotNil(t, sink.inc)
	assert.Equal(t, "org: create", sink.inc.Message)
	assert.Len(t, sink.inc.Attrs, 1) // caller attrs only (org_id)
	assert.Equal(t, "db down", sink.inc.Meta["underlying_error"])
}

func TestReporter_ContextMeta(t *testing.T) {
	r := NewReporter(nil, false, WithContextMeta(func(context.Context) map[string]string {
		return map[string]string{"request_id": "r1"}
	}))

	out := r.Unexpected(context.Background(), "x", errors.New("boom"))
	assert.Equal(t, "r1", out.Meta()["request_id"])
}

func TestReporter_TraceMeta(t *testing.T) {
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{1},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	out := NewReporter(nil, false).Unexpected(ctx, "x", errors.New("boom"))
	assert.Equal(t, sc.TraceID().String(), out.Meta()["trace_id"])
}

func TestReporter_ProductionHidesCause(t *testing.T) {
	r := NewReporter(nil, true) // nil log is tolerated

	out := r.Unexpected(context.Background(), "x", errors.New("secret"))
	assert.NotContains(t, out.Error(), "secret")
	assert.Empty(t, out.Meta()["underlying_error"])
}

func TestToSlogAttrs(t *testing.T) {
	attrs := toSlogAttrs([]any{"a", 1, "", 2, "b", 3, "dangling"})
	assert.Len(t, attrs, 2)
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func TestReporter_Cancellation(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithTimeout(context.Background(), 0)
	defer stop()
	<-expired.Done()

	tests := []struct {
		name      string
		ctx       context.Context
		cause     error
		wantLevel slog.Level
		wantMsg   string
		incident  bool
	}{
		{"client went away", cancelled, fmt.Errorf("org: list: %w", context.Canceled), slog.LevelDebug, "request cancelled by the client", false},
		{"deadline is a fault", expired, fmt.Errorf("org: list: %w", context.DeadlineExceeded), slog.LevelError, "org: list", true},
		{"canceled cause on a live context is a fault", context.Background(), context.Canceled, slog.LevelError, "org: list", true},
		{"other error on a cancelled context is a fault", cancelled, errors.New("db down"), slog.LevelError, "org: list", true},
		{"canceled cause on an expired context is a fault", expired, fmt.Errorf("org: list: %w", context.Canceled), slog.LevelError, "org: list", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := &recordingHandler{}
			sink := &recordingSink{}
			counter := &countingCounter{}
			r := NewReporter(slog.New(logs), false, WithSinks(sink), WithIncidentCounter(counter))

			out := r.Unexpected(tt.ctx, "org: list", tt.cause, "org_id", "o1")

			assert.Equal(t, CodeUnexpected, out.Code())
			assert.ErrorIs(t, out, tt.cause)
			require.Len(t, logs.records, 1)
			rec := logs.records[0]
			assert.Equal(t, tt.wantLevel, rec.Level)
			assert.Equal(t, tt.wantMsg, rec.Message)
			attrs := map[string]string{}
			rec.Attrs(func(a slog.Attr) bool {
				attrs[a.Key] = a.Value.String()
				return true
			})
			assert.Equal(t, "o1", attrs["org_id"])
			assert.Equal(t, tt.cause.Error(), attrs["error"])
			assert.Equal(t, tt.incident, sink.inc != nil)
			assert.Equal(t, tt.incident, len(counter.codes) == 1)
		})
	}
}

type recordingHandler struct{ records []slog.Record }

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *recordingHandler) WithGroup(string) slog.Handler { return h }
