package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/queue"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/worker"
)

// Client is the queue on NATS JetStream.
type Client struct {
	nc         *natsgo.Conn
	js         jetstream.JetStream
	log        *slog.Logger
	tracer     trace.Tracer
	unexpected apperror.UnexpectedFunc
	metrics    metrics
	declared   atomic.Pointer[queue.Declared]
	budget     time.Duration
	onAttempt  func()
}

type publication struct {
	span          string
	stream        string
	subject       string
	kind          string
	name          string
	version       int
	nameHeader    string
	versionHeader string
	budget        time.Duration
	ok            metric.Int64Counter
	okOpts        []metric.AddOption
	failed        metric.Int64Counter
}

// Open ensures the streams on nc and returns a Client; the caller owns nc and closes it.
func Open(ctx context.Context, nc *natsgo.Conn, o Options) (*Client, error) {
	if o.Unexpected == nil {
		return nil, errors.New("queue: Options.Unexpected is required")
	}
	if nc == nil {
		return nil, errors.New("queue: no NATS connection")
	}
	c, err := newClient(o.Log, o.Tracer, o.Meter)
	if err != nil {
		return nil, err
	}
	c.unexpected = o.Unexpected
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("queue: jetstream: %w", err)
	}
	if err := ensureStreams(ctx, js); err != nil {
		return nil, err
	}
	c.nc, c.js = nc, js
	return c, nil
}

func newClient(log *slog.Logger, tracer trace.Tracer, meter metric.Meter) (*Client, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if tracer == nil {
		tracer = tracenoop.NewTracerProvider().Tracer("")
	}
	if meter == nil {
		meter = metricnoop.NewMeterProvider().Meter("")
	}
	m, err := newMetrics(meter)
	if err != nil {
		return nil, err
	}
	c := &Client{log: log, tracer: tracer, metrics: m, budget: publishBudget}
	c.declared.Store(&queue.Declared{})
	return c, nil
}

// Declare implements queue.Queue.
func (c *Client) Declare(jobs []queue.Job, broadcasts []queue.Broadcast) error {
	d, err := queue.NewDeclared(jobs, broadcasts)
	if err != nil {
		return err
	}
	c.declared.Store(d)
	return nil
}

// Submit implements queue.Queue: it waits for the WORK stream's ack.
func (c *Client) Submit(ctx context.Context, j queue.Job, data any) error {
	if !c.declared.Load().HasJob(j) {
		return &queue.UndeclaredJobError{Job: j}
	}
	return c.send(ctx, c.jobPublication(j), data)
}

// Emit implements queue.Queue: it waits for the BROADCAST stream's ack.
func (c *Client) Emit(ctx context.Context, b queue.Broadcast, data any) error {
	if !c.declared.Load().HasBroadcast(b) {
		return &queue.UndeclaredBroadcastError{Broadcast: b}
	}
	return c.send(ctx, c.broadcastPublication(b), data)
}

// Consumer implements queue.Queue with one durable consumer per handler.
func (c *Client) Consumer(ctx context.Context, hs []queue.Handler) (worker.Worker, error) {
	cons, err := NewConsumer(ctx, c, hs)
	if err != nil {
		return nil, err
	}
	return cons, nil
}

// Listener implements queue.Queue: it delivers broadcasts from the BROADCAST stream's next sequence on.
func (c *Client) Listener(ctx context.Context, ls []queue.Listener) (worker.Worker, error) {
	seq, err := c.BroadcastStartSeq(ctx)
	if err != nil {
		return nil, err
	}
	l, err := NewListener(c, ls, seq)
	if err != nil {
		return nil, err
	}
	return l, nil
}

// BroadcastStartSeq returns the BROADCAST stream's next sequence.
func (c *Client) BroadcastStartSeq(ctx context.Context) (uint64, error) {
	s, err := c.js.Stream(ctx, streamBroadcast)
	if err != nil {
		return 0, fmt.Errorf("queue: stream %s: %w", streamBroadcast, err)
	}
	info, err := s.Info(ctx)
	if err != nil {
		return 0, fmt.Errorf("queue: stream %s info: %w", streamBroadcast, err)
	}
	return info.State.LastSeq + 1, nil
}

func (c *Client) jobPublication(j queue.Job) publication {
	return publication{
		span:          "queue.publish",
		stream:        streamWork,
		subject:       j.Subject(),
		kind:          "job",
		name:          j.Name,
		version:       j.Version,
		nameHeader:    headerJob,
		versionHeader: headerJobVersion,
		budget:        c.budget,
		ok:            c.metrics.published,
		failed:        c.metrics.publishFailed,
	}
}

func (c *Client) broadcastPublication(b queue.Broadcast) publication {
	return publication{
		span:          "queue.emit",
		stream:        streamBroadcast,
		subject:       b.Subject(),
		kind:          "broadcast",
		name:          b.Name,
		version:       b.Version,
		nameHeader:    headerBroadcast,
		versionHeader: headerBroadcastVersion,
		budget:        c.budget,
		ok:            c.metrics.emitted,
		failed:        c.metrics.emitFailed,
	}
}

func (c *Client) send(ctx context.Context, p publication, data any) error {
	id, err := uuid.NewV7()
	if err != nil {
		return &queue.PublishError{Subject: p.subject, Cause: err}
	}
	return c.publish(ctx, p, data, id)
}

func (c *Client) publish(ctx context.Context, p publication, data any, id uuid.UUID) error {
	body, err := json.Marshal(data)
	if err != nil {
		p.failed.Add(ctx, 1)
		return &queue.PublishError{Subject: p.subject, Cause: err}
	}
	msg := natsgo.NewMsg(p.subject)
	msg.Data = body
	msg.Header.Set(p.nameHeader, p.name)
	msg.Header.Set(p.versionHeader, strconv.Itoa(p.version))
	msg.Header.Set(headerCreatedAt, time.Now().UTC().Format(time.RFC3339Nano))
	if tc, err := tenant.From(ctx); err == nil {
		setTenant(msg.Header, tc)
	}
	return c.deliver(ctx, p, msg, id.String())
}

func (c *Client) deliver(ctx context.Context, p publication, msg *natsgo.Msg, id string) error {
	ctx, span := c.tracer.Start(ctx, p.span, trace.WithSpanKind(trace.SpanKindProducer), trace.WithAttributes(spanAttributes(ctx, p)...))
	defer span.End()
	injectTrace(ctx, msg.Header)

	pubCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.budget)
	defer cancel()
	if err := c.publishWithRetry(pubCtx, msg, id); err != nil {
		span.RecordError(err)
		p.failed.Add(ctx, 1)
		return &queue.PublishError{Subject: p.subject, Cause: err}
	}
	p.ok.Add(ctx, 1, p.okOpts...)
	return nil
}

func (c *Client) publishWithRetry(ctx context.Context, msg *natsgo.Msg, id string) error {
	for {
		err := c.publishOnce(ctx, msg, id)
		if err == nil || !retryable(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(publishRetryGap):
		}
	}
}

func (c *Client) publishOnce(ctx context.Context, msg *natsgo.Msg, id string) error {
	if c.onAttempt != nil {
		c.onAttempt()
	}
	attemptCtx, cancel := context.WithTimeout(ctx, publishAttempt)
	defer cancel()
	_, err := c.js.PublishMsg(attemptCtx, msg, jetstream.WithMsgID(id), jetstream.WithRetryAttempts(0))
	return err
}

func retryable(err error) bool {
	for _, target := range []error{
		context.DeadlineExceeded,
		natsgo.ErrTimeout,
		jetstream.ErrNoStreamResponse,
		natsgo.ErrNoResponders,
		natsgo.ErrConnectionClosed,
		natsgo.ErrDisconnected,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
