package nats

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/queue"
	"altalune.id/oagent/internal/kernel/tenant"
)

func testJob() queue.Job { return queue.Job{Name: "test.run", Version: 1} }

func testBroadcast() queue.Broadcast { return queue.Broadcast{Name: "test.ping", Version: 1} }

type testPayload struct {
	Title string `json:"title"`
}

func discardUnexpected(context.Context, string, error, ...any) *apperror.AppError { return nil }

type testSetup struct {
	Options
	url, user, password string
}

func testOptions(url string) testSetup {
	return testSetup{
		Options: Options{Log: slog.New(slog.DiscardHandler), Unexpected: discardUnexpected},
		url:     url,
	}
}

func dialTest(t *testing.T, o testSetup) *natsgo.Conn {
	t.Helper()
	var opts []natsgo.Option
	if o.user != "" {
		opts = append(opts, natsgo.UserInfo(o.user, o.password))
	}
	nc, err := natsgo.Connect(o.url, opts...)
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	return nc
}

func connectTest(t *testing.T, o testSetup) *Client {
	t.Helper()
	c, err := Open(t.Context(), dialTest(t, o), o.Options)
	require.NoError(t, err)
	return c
}

func streamInfo(t *testing.T, c *Client, name string) *jetstream.StreamInfo {
	t.Helper()
	s, err := c.js.Stream(t.Context(), name)
	require.NoError(t, err)
	info, err := s.Info(t.Context())
	require.NoError(t, err)
	return info
}

func lastMsg(t *testing.T, c *Client, stream, subject string) *jetstream.RawStreamMsg {
	t.Helper()
	s, err := c.js.Stream(t.Context(), stream)
	require.NoError(t, err)
	m, err := s.GetLastMsgForSubject(t.Context(), subject)
	require.NoError(t, err)
	return m
}

func TestOpen_CreatesStreams(t *testing.T) {
	c := connectTest(t, testOptions(startNATS(t)))

	tests := []struct {
		stream    string
		subjects  []string
		retention jetstream.RetentionPolicy
		discard   jetstream.DiscardPolicy
		maxAge    time.Duration
		maxBytes  int64
	}{
		{streamWork, []string{"jobs.>"}, jetstream.WorkQueuePolicy, jetstream.DiscardNew, 7 * 24 * time.Hour, 256 << 20},
		{streamDLQ, []string{"dlq.jobs.>"}, jetstream.LimitsPolicy, jetstream.DiscardOld, 30 * 24 * time.Hour, 256 << 20},
		{streamBroadcast, []string{"broadcast.>"}, jetstream.LimitsPolicy, jetstream.DiscardOld, time.Hour, 64 << 20},
	}
	for _, tt := range tests {
		t.Run(tt.stream, func(t *testing.T) {
			cfg := streamInfo(t, c, tt.stream).Config
			assert.Equal(t, tt.subjects, cfg.Subjects)
			assert.Equal(t, tt.retention, cfg.Retention)
			assert.Equal(t, tt.discard, cfg.Discard)
			assert.Equal(t, tt.maxAge, cfg.MaxAge)
			assert.Equal(t, tt.maxBytes, cfg.MaxBytes)
			assert.Equal(t, jetstream.FileStorage, cfg.Storage)
			assert.Equal(t, 1, cfg.Replicas)
			assert.Equal(t, 2*time.Minute, cfg.Duplicates)
		})
	}
}

func TestOpen_Idempotent(t *testing.T) {
	url := startNATS(t)
	connectTest(t, testOptions(url))
	c := connectTest(t, testOptions(url))

	assert.Equal(t, []string{"jobs.>"}, streamInfo(t, c, streamWork).Config.Subjects)
	assert.Equal(t, []string{"dlq.jobs.>"}, streamInfo(t, c, streamDLQ).Config.Subjects)
}

func TestOpen_RequiresUnexpected(t *testing.T) {
	o := testOptions(startNATS(t))
	o.Unexpected = nil
	_, err := Open(t.Context(), dialTest(t, o), o.Options)
	require.Error(t, err)
}

func TestOpen_RequiresAConnection(t *testing.T) {
	_, err := Open(t.Context(), nil, testOptions("").Options)
	require.ErrorContains(t, err, "no NATS connection")
}

func TestBroadcastStartSeq(t *testing.T) {
	c := connectTest(t, testOptions(startNATS(t)))
	require.NoError(t, c.Declare(nil, []queue.Broadcast{testBroadcast()}))

	seq, err := c.BroadcastStartSeq(t.Context())
	require.NoError(t, err)
	assert.Equal(t, uint64(1), seq)

	require.NoError(t, c.Emit(t.Context(), testBroadcast(), testPayload{Title: "a"}))
	require.NoError(t, c.Emit(t.Context(), testBroadcast(), testPayload{Title: "b"}))

	seq, err = c.BroadcastStartSeq(t.Context())
	require.NoError(t, err)
	assert.Equal(t, streamInfo(t, c, streamBroadcast).State.LastSeq+1, seq)
	assert.Equal(t, uint64(3), seq)
}

func TestSubmit_PublishesWithHeaders(t *testing.T) {
	c := connectTest(t, testOptions(startNATS(t)))
	require.NoError(t, c.Declare([]queue.Job{testJob()}, nil))

	tc := tenant.Context{OrgID: uuid.New(), ProjectID: uuid.New(), UserID: uuid.New()}
	ctx := tenant.Into(t.Context(), tc)
	require.NoError(t, c.Submit(ctx, testJob(), testPayload{Title: "hello"}))

	assert.Equal(t, uint64(1), streamInfo(t, c, streamWork).State.Msgs)
	m := lastMsg(t, c, streamWork, "jobs.test.run.v1")

	var got testPayload
	require.NoError(t, json.Unmarshal(m.Data, &got))
	assert.Equal(t, "hello", got.Title)

	_, err := uuid.Parse(m.Header.Get("Nats-Msg-Id"))
	require.NoError(t, err)
	assert.Equal(t, "test.run", m.Header.Get("Oagent-Job"))
	assert.Equal(t, "1", m.Header.Get("Oagent-Job-Version"))
	_, err = time.Parse(time.RFC3339, m.Header.Get("Oagent-Created-At"))
	require.NoError(t, err)
	assert.Equal(t, tc.OrgID.String(), m.Header.Get("Oagent-Org-Id"))
	assert.Equal(t, tc.ProjectID.String(), m.Header.Get("Oagent-Project-Id"))
	assert.Equal(t, tc.UserID.String(), m.Header.Get("Oagent-User-Id"))
}

func TestSubmit_Unscoped(t *testing.T) {
	c := connectTest(t, testOptions(startNATS(t)))
	require.NoError(t, c.Declare([]queue.Job{testJob()}, nil))

	require.NoError(t, c.Submit(t.Context(), testJob(), testPayload{}))

	m := lastMsg(t, c, streamWork, "jobs.test.run.v1")
	assert.Empty(t, m.Header.Values("Oagent-Org-Id"))
}

func TestSubmit_UndeclaredJob(t *testing.T) {
	c := connectTest(t, testOptions(startNATS(t)))
	require.NoError(t, c.Declare([]queue.Job{testJob()}, nil))

	err := c.Submit(t.Context(), queue.Job{Name: "test.other", Version: 1}, testPayload{})
	require.Error(t, err)
	assert.True(t, queue.IsUndeclaredJobError(err))
	assert.Equal(t, uint64(0), streamInfo(t, c, streamWork).State.Msgs)
}

func TestSubmit_BeforeDeclare(t *testing.T) {
	c := connectTest(t, testOptions(startNATS(t)))
	assert.True(t, queue.IsUndeclaredJobError(c.Submit(t.Context(), testJob(), testPayload{})))
}

func TestDeclare_FailureKeepsPreviousSet(t *testing.T) {
	c := connectTest(t, testOptions(startNATS(t)))
	require.NoError(t, c.Declare([]queue.Job{testJob()}, nil))
	require.Error(t, c.Declare([]queue.Job{{Name: "bad", Version: 1}}, nil))

	require.NoError(t, c.Submit(t.Context(), testJob(), testPayload{}))
}

func TestEmit(t *testing.T) {
	c := connectTest(t, testOptions(startNATS(t)))
	require.NoError(t, c.Declare(nil, []queue.Broadcast{testBroadcast()}))

	require.NoError(t, c.Emit(t.Context(), testBroadcast(), testPayload{Title: "ping"}))

	assert.Equal(t, uint64(1), streamInfo(t, c, streamBroadcast).State.Msgs)
	m := lastMsg(t, c, streamBroadcast, "broadcast.test.ping.v1")
	assert.Equal(t, "test.ping", m.Header.Get("Oagent-Broadcast"))
	assert.Equal(t, "1", m.Header.Get("Oagent-Broadcast-Version"))
	_, err := uuid.Parse(m.Header.Get("Nats-Msg-Id"))
	require.NoError(t, err)
	assert.Empty(t, m.Header.Values("Oagent-Job"))
}

func TestEmit_UndeclaredBroadcast(t *testing.T) {
	c := connectTest(t, testOptions(startNATS(t)))
	require.NoError(t, c.Declare(nil, []queue.Broadcast{testBroadcast()}))

	err := c.Emit(t.Context(), queue.Broadcast{Name: "test.other", Version: 1}, testPayload{})
	require.Error(t, err)
	assert.True(t, queue.IsUndeclaredBroadcastError(err))
	assert.Equal(t, uint64(0), streamInfo(t, c, streamBroadcast).State.Msgs)
}

func TestPublish_SameMsgIDStoresOnce(t *testing.T) {
	c := connectTest(t, testOptions(startNATS(t)))
	require.NoError(t, c.Declare([]queue.Job{testJob()}, nil))

	id := uuid.New()
	p := c.jobPublication(testJob())
	require.NoError(t, c.publish(t.Context(), p, testPayload{Title: "a"}, id))
	require.NoError(t, c.publish(t.Context(), p, testPayload{Title: "a"}, id))

	assert.Equal(t, uint64(1), streamInfo(t, c, streamWork).State.Msgs)
}

func TestSubmit_CancelledContextStillPublishes(t *testing.T) {
	c := connectTest(t, testOptions(startNATS(t)))
	require.NoError(t, c.Declare([]queue.Job{testJob()}, nil))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, c.Submit(ctx, testJob(), testPayload{}))

	assert.Equal(t, uint64(1), streamInfo(t, c, streamWork).State.Msgs)
}

func TestSubmit_ServerDownIsPublishErrorAfterBudget(t *testing.T) {
	ns := startNATSServer(t)
	c := connectTest(t, testOptions(ns.ClientURL()))
	require.NoError(t, c.Declare([]queue.Job{testJob()}, nil))
	c.budget = 1500 * time.Millisecond
	ns.Shutdown()

	start := time.Now()
	err := c.Submit(t.Context(), testJob(), testPayload{})
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.True(t, queue.IsPublishError(err))
	assert.GreaterOrEqual(t, elapsed, 1400*time.Millisecond)
	assert.Less(t, elapsed, 3*time.Second)
}

func TestSubmit_NonRetryableErrorReturnsAtOnce(t *testing.T) {
	c := connectTest(t, testOptions(startNATS(t)))
	require.NoError(t, c.Declare([]queue.Job{testJob()}, nil))

	var attempts atomic.Int64
	c.onAttempt = func() { attempts.Add(1) }

	oversized := strings.Repeat("a", int(c.nc.MaxPayload())+4096)
	err := c.Submit(t.Context(), testJob(), testPayload{Title: oversized})

	require.Error(t, err)
	assert.True(t, queue.IsPublishError(err))
	assert.ErrorIs(t, err, natsgo.ErrMaxPayload)
	assert.Equal(t, int64(1), attempts.Load())
}

func TestSubmit_UnmarshalablePayload(t *testing.T) {
	c := connectTest(t, testOptions(startNATS(t)))
	require.NoError(t, c.Declare([]queue.Job{testJob()}, nil))

	err := c.Submit(t.Context(), testJob(), make(chan int))
	assert.True(t, queue.IsPublishError(err))
}

func TestMetrics_CountPublishes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	o := testOptions(startNATS(t))
	o.Meter = mp.Meter("test")
	c := connectTest(t, o)
	require.NoError(t, c.Declare([]queue.Job{testJob()}, []queue.Broadcast{testBroadcast()}))

	require.NoError(t, c.Submit(t.Context(), testJob(), testPayload{}))
	require.NoError(t, c.Emit(t.Context(), testBroadcast(), testPayload{}))
	require.Error(t, c.Submit(t.Context(), testJob(), make(chan int)))

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))
	got := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
				for _, dp := range sum.DataPoints {
					got[m.Name] += dp.Value
				}
			}
		}
	}
	assert.Equal(t, int64(1), got["queue.published"])
	assert.Equal(t, int64(1), got["queue.emitted"])
	assert.Equal(t, int64(1), got["queue.publish_failed"])
}
