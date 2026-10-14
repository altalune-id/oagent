package queue

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
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/tenant"
)

// Client publishes jobs and broadcasts; a disabled Client accepts every call and publishes nothing.
type Client struct {
	nc         *nats.Conn
	js         jetstream.JetStream
	log        *slog.Logger
	tracer     trace.Tracer
	unexpected apperror.UnexpectedFunc
	metrics    metrics
	declared   atomic.Pointer[declaredSet]
	budget     time.Duration
	onAttempt  func()
}

type declaredSet struct {
	jobs       map[Job]struct{}
	broadcasts map[Broadcast]struct{}
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
func Open(ctx context.Context, nc *nats.Conn, o Options) (*Client, error) {
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

// Disabled returns a Client with no connection, whose Submit and Emit do nothing.
func Disabled(log *slog.Logger) *Client {
	c, _ := newClient(log, nil, nil)
	c.log.Info("queue: disabled — Submit is a no-op")
	return c
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
	c.declared.Store(&declaredSet{})
	return c, nil
}

// Enabled reports whether c holds a NATS connection.
func (c *Client) Enabled() bool { return c.nc != nil }

// Declare records the jobs Submit accepts and the broadcasts Emit accepts, refusing an invalid or duplicated one.
func (c *Client) Declare(jobs []Job, broadcasts []Broadcast) error {
	jobSet, jobDup, err := collect(jobs)
	if err != nil {
		return err
	}
	broadcastSet, broadcastDup, err := collect(broadcasts)
	if err != nil {
		return err
	}
	if err := wiringError(append(jobDup, broadcastDup...)); err != nil {
		return err
	}
	c.declared.Store(&declaredSet{jobs: jobSet, broadcasts: broadcastSet})
	return nil
}

// Submit publishes data as one run of job j, waiting for the stream's ack.
func (c *Client) Submit(ctx context.Context, j Job, data any) error {
	if !c.Enabled() {
		return nil
	}
	if _, ok := c.declared.Load().jobs[j]; !ok {
		return &UndeclaredJobError{Job: j}
	}
	return c.send(ctx, c.jobPublication(j), data)
}

// Emit publishes data to every running instance, waiting for the stream's ack.
func (c *Client) Emit(ctx context.Context, b Broadcast, data any) error {
	if !c.Enabled() {
		return nil
	}
	if _, ok := c.declared.Load().broadcasts[b]; !ok {
		return &UndeclaredBroadcastError{Broadcast: b}
	}
	return c.send(ctx, c.broadcastPublication(b), data)
}

// BroadcastStartSeq returns the BROADCAST stream's next sequence.
func (c *Client) BroadcastStartSeq(ctx context.Context) (uint64, error) {
	if !c.Enabled() {
		return 0, nil
	}
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

func (c *Client) jobPublication(j Job) publication {
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

func (c *Client) broadcastPublication(b Broadcast) publication {
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
		return &PublishError{Subject: p.subject, Cause: err}
	}
	return c.publish(ctx, p, data, id)
}

func (c *Client) publish(ctx context.Context, p publication, data any, id uuid.UUID) error {
	body, err := json.Marshal(data)
	if err != nil {
		p.failed.Add(ctx, 1)
		return &PublishError{Subject: p.subject, Cause: err}
	}
	msg := nats.NewMsg(p.subject)
	msg.Data = body
	msg.Header.Set(p.nameHeader, p.name)
	msg.Header.Set(p.versionHeader, strconv.Itoa(p.version))
	msg.Header.Set(headerCreatedAt, time.Now().UTC().Format(time.RFC3339Nano))
	if tc, err := tenant.From(ctx); err == nil {
		setTenant(msg.Header, tc)
	}
	return c.deliver(ctx, p, msg, id.String())
}

func (c *Client) deliver(ctx context.Context, p publication, msg *nats.Msg, id string) error {
	ctx, span := c.tracer.Start(ctx, p.span, trace.WithSpanKind(trace.SpanKindProducer), trace.WithAttributes(spanAttributes(ctx, p)...))
	defer span.End()
	injectTrace(ctx, msg.Header)

	pubCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.budget)
	defer cancel()
	if err := c.publishWithRetry(pubCtx, msg, id); err != nil {
		span.RecordError(err)
		p.failed.Add(ctx, 1)
		return &PublishError{Subject: p.subject, Cause: err}
	}
	p.ok.Add(ctx, 1, p.okOpts...)
	return nil
}

func (c *Client) publishWithRetry(ctx context.Context, msg *nats.Msg, id string) error {
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

func (c *Client) publishOnce(ctx context.Context, msg *nats.Msg, id string) error {
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
		nats.ErrTimeout,
		jetstream.ErrNoStreamResponse,
		nats.ErrNoResponders,
		nats.ErrConnectionClosed,
		nats.ErrDisconnected,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
