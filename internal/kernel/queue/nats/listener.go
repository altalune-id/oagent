package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"altalune.id/oagent/internal/kernel/queue"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/worker"
)

const listenerTimeout = 5 * time.Second

const (
	listenerSubject      = "broadcast"
	reportListenerFailed = "queue: broadcast listener failed"
)

// Listen delivers every broadcast to its listener through one ordered consumer per instance.
type Listen struct {
	client    *Client
	listeners map[string]queue.Listener
	subjects  []string
	startSeq  uint64
}

var _ worker.Worker = (*Listen)(nil)

// NewListener builds the worker that delivers broadcasts from startSeq on.
func NewListener(c *Client, ls []queue.Listener, startSeq uint64) (*Listen, error) {
	if err := queue.CheckListeners(ls); err != nil {
		return nil, err
	}
	listeners := make(map[string]queue.Listener, len(ls))
	for _, l := range ls {
		listeners[l.Broadcast.Subject()] = l
	}
	return &Listen{client: c, listeners: listeners, subjects: slices.Sorted(maps.Keys(listeners)), startSeq: startSeq}, nil
}

// Name implements worker.Worker.
func (l *Listen) Name() string { return queue.ListenerName }

// Run listens until ctx is cancelled; *ConsumerClosedError if the loop stops first.
func (l *Listen) Run(ctx context.Context) error {
	if len(l.subjects) == 0 {
		return nil
	}
	oc, err := l.client.js.OrderedConsumer(ctx, streamBroadcast, jetstream.OrderedConsumerConfig{
		FilterSubjects: l.subjects,
		DeliverPolicy:  jetstream.DeliverByStartSequencePolicy,
		OptStartSeq:    l.startSeq,
	})
	if err != nil {
		return startFailed(ctx, "ordered consumer", err)
	}
	cc, err := oc.Consume(l.handle(ctx), l.consumeErrHandler())
	if err != nil {
		return startFailed(ctx, "consume", err)
	}
	select {
	case <-cc.Closed():
		if !cancelled(ctx) {
			return &queue.ConsumerClosedError{Subject: listenerSubject}
		}
		return nil
	case <-ctx.Done():
	}
	cc.Stop()
	select {
	case <-cc.Closed():
	case <-time.After(drainWait):
		l.client.log.Warn("queue: listener stop timed out", slog.String("subject", listenerSubject))
	}
	return nil
}

func startFailed(ctx context.Context, op string, err error) error {
	switch {
	case cancelled(ctx):
		return nil
	case errors.Is(err, natsgo.ErrConnectionClosed):
		return &queue.ConsumerClosedError{Subject: listenerSubject}
	}
	return fmt.Errorf("queue: listener: %s: %w", op, err)
}

func cancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func (l *Listen) consumeErrHandler() jetstream.ConsumeErrHandler {
	return func(_ jetstream.ConsumeContext, err error) {
		l.client.log.Warn("queue: listen", slog.String("err", err.Error()))
	}
}

func (l *Listen) handle(runCtx context.Context) jetstream.MessageHandler {
	return func(msg jetstream.Msg) {
		subject := msg.Subject()
		ctx := extractTrace(context.WithoutCancel(runCtx), msg.Headers())
		ctx, span := l.client.tracer.Start(ctx, "queue.listen", trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(
				attribute.String("messaging.system", "nats"),
				attribute.String("subject", subject),
				attribute.String("stream", streamBroadcast),
			))
		defer span.End()
		if err := l.dispatch(ctx, span, msg); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			l.client.log.ErrorContext(ctx, reportListenerFailed, slog.String("subject", subject), slog.String("err", err.Error()))
			l.client.metrics.broadcastFailed.Add(ctx, 1, metric.WithAttributes(attribute.String("subject", subject)))
			_ = l.client.unexpected(ctx, reportListenerFailed, err, "subject", subject, "msg_id", msg.Headers().Get(headerMsgID))
		}
	}
}

func (l *Listen) dispatch(ctx context.Context, span trace.Span, msg jetstream.Msg) error {
	subject := msg.Subject()
	ls, ok := l.listeners[subject]
	if !ok {
		return errors.New("queue: no listener for " + subject)
	}
	span.SetAttributes(attribute.String("broadcast", ls.Broadcast.Name))
	m, tc, scoped, err := decodeMessage(l.client.broadcastPublication(ls.Broadcast), msg, 1)
	if err != nil {
		return err
	}
	if scoped {
		ctx = tenant.Into(ctx, tc)
		span.SetAttributes(tenantAttributes(tc)...)
	}
	return l.client.safely(ctx, subject, listenerTimeout, func(ctx context.Context) error { return ls.Handle(ctx, m) })
}
