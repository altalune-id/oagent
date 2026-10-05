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
