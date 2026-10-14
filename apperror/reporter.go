package apperror

import (
	"context"
	"errors"
	"log/slog"
	"maps"

	"go.opentelemetry.io/otel/trace"
)

// UnexpectedFunc is the function-type dependency domain services accept.
type UnexpectedFunc = func(ctx context.Context, message string, cause error, attrs ...any) *AppError

// ContextMetaFunc derives envelope metadata (e.g. request_id) from ctx.
type ContextMetaFunc func(ctx context.Context) map[string]string

// ReportSink receives an incident for fan-out.
type ReportSink interface {
	Report(ctx context.Context, inc Incident)
}

// IncidentCounter records one tick per unexpected incident, keyed by code.
type IncidentCounter interface {
	Inc(ctx context.Context, code string)
}

// Incident is one reported unexpected error.
type Incident struct {
	Message string
	Code    Code
	Cause   error
	AppErr  *AppError
	Attrs   []slog.Attr
	Meta    map[string]string
}

// Reporter turns unexpected errors into AppError envelopes and fans out incidents.
type Reporter struct {
	log          *slog.Logger
	isProduction bool
	contextMeta  ContextMetaFunc
	counter      IncidentCounter
	sinks        []ReportSink
}

// ReporterOption configures a Reporter at construction.
type ReporterOption func(*Reporter)

// WithContextMeta installs a function that adds ctx-derived key/values into the envelope.
func WithContextMeta(fn ContextMetaFunc) ReporterOption {
	return func(r *Reporter) { r.contextMeta = fn }
}

// WithSinks appends ReportSinks to the fan-out.
func WithSinks(s ...ReportSink) ReporterOption {
	return func(r *Reporter) { r.sinks = append(r.sinks, s...) }
}

// WithIncidentCounter attaches a counter ticked once per Unexpected call.
func WithIncidentCounter(c IncidentCounter) ReporterOption {
	return func(r *Reporter) { r.counter = c }
}

// NewReporter builds a Reporter.
func NewReporter(log *slog.Logger, isProduction bool, opts ...ReporterOption) *Reporter {
	r := &Reporter{log: log, isProduction: isProduction}
	for _, o := range opts {
		o(r)
	}
	return r
}

var _ UnexpectedFunc = (*Reporter)(nil).Unexpected

// Unexpected logs cause, fans out to sinks, and returns an AppError; a request the client cancelled logs at debug only.
func (r *Reporter) Unexpected(ctx context.Context, message string, cause error, attrs ...any) *AppError {
	cancelled := errors.Is(cause, context.Canceled) && errors.Is(ctx.Err(), context.Canceled)
	logAttrs := append([]any{slog.Any("error", cause)}, attrs...)
	if r.log != nil {
		if cancelled {
			r.log.DebugContext(ctx, "request cancelled by the client", logAttrs...)
		} else {
			r.log.ErrorContext(ctx, message, logAttrs...)
		}
	}

	meta := r.collectMeta(ctx)
	wireMsg := "an unexpected error occurred"
	if !r.isProduction && cause != nil {
		wireMsg += ": " + cause.Error()
		meta["underlying_error"] = cause.Error()
	}
	out := New(CodeUnexpected, wireMsg).WithCause(cause).WithMeta(meta)
	if cancelled {
		return out
	}

	if r.counter != nil {
		r.counter.Inc(ctx, string(out.Code()))
	}
	r.fanOut(ctx, message, cause, out, attrs)
	return out
}

func (r *Reporter) collectMeta(ctx context.Context) map[string]string {
	m := map[string]string{}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		m["trace_id"] = sc.TraceID().String()
	}
	if r.contextMeta != nil {
		maps.Copy(m, r.contextMeta(ctx))
	}
	return m
}

func (r *Reporter) fanOut(ctx context.Context, message string, cause error, out *AppError, attrs []any) {
	if len(r.sinks) == 0 {
		return
	}
	inc := Incident{
		Message: message,
		Code:    out.Code(),
		Cause:   cause,
		AppErr:  out,
		Attrs:   toSlogAttrs(attrs),
		Meta:    out.Meta(),
	}
	for _, s := range r.sinks {
		s.Report(ctx, inc)
	}
}

func toSlogAttrs(a []any) []slog.Attr {
	out := make([]slog.Attr, 0, len(a)/2)
	for i := 0; i+1 < len(a); i += 2 {
		key, _ := a[i].(string)
		if key == "" {
			continue
		}
		out = append(out, slog.Any(key, a[i+1]))
	}
	return out
}
