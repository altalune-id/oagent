package queue

import (
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"altalune.id/oagent/apperror"
)

// Options configures Connect.
type Options struct {
	URL, Token     string
	User, Password string
	ConnectTimeout time.Duration
	Log            *slog.Logger
	Tracer         trace.Tracer
	Meter          metric.Meter
	Unexpected     apperror.UnexpectedFunc
}
