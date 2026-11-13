package run_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

const secretPassage = "SECRET-PASSAGE refunds take 7 days"

type fakeKnowledge struct {
	mu        sync.Mutex
	sources   []run.KnowledgeSource
	queries   []run.KnowledgeQuery
	deadlines []time.Duration
	hits      []run.KnowledgeHit
	usage     run.KnowledgeUsage
	err       error
	price     *int64
	priceErr  error
	priceRead int
	onSearch  func()
}

func (k *fakeKnowledge) BoundSources(context.Context, uuid.UUID) ([]run.KnowledgeSource, error) {
	return k.sources, nil
}

func (k *fakeKnowledge) EmbeddingPrice(context.Context, uuid.UUID) (*int64, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.priceRead++
	return k.price, k.priceErr
}

func (k *fakeKnowledge) Search(ctx context.Context, q run.KnowledgeQuery) ([]run.KnowledgeHit, run.KnowledgeUsage, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.queries = append(k.queries, q)
	if k.onSearch != nil {
		k.onSearch()
	}
	if d, ok := ctx.Deadline(); ok {
		k.deadlines = append(k.deadlines, time.Until(d))
	}
	return k.hits, k.usage, k.err
}

func (k *fakeKnowledge) texts() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]string, len(k.queries))
	for i, q := range k.queries {
		out[i] = q.Text
	}
	return out
}

func (f *svcFixture) withKnowledge() *fakeKnowledge {
	src := run.KnowledgeSource{ID: uuid.New(), PublicID: "ks_refunds"}
	k := &fakeKnowledge{
		sources: []run.KnowledgeSource{src},
		hits:    []run.KnowledgeHit{{Text: secretPassage, Title: "Refund policy", Heading: "Timing", Page: 2, Score: 0.9, Source: src.ID}},
		usage:   run.KnowledgeUsage{EmbeddingTokens: 7, RetrieveCalls: 1},
		price:   new(int64(20_000)),
	}
	f.svc.WithKnowledge(k, k, k)
	f.version.Knowledge = run.KnowledgeTuning{MaxBytes: 16000}
	return k
}

func TestRun_KnowledgeSearchedOncePerTurn(t *testing.T) {
	f := newSvc(t)
	k := f.withKnowledge()
	f.say(t, "record lunch 25000")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "tool_call")})
	assert.Equal(t, run.StatusCompleted, r.Status)

	require.Len(t, k.queries, 1, "one search for the turn, however many model calls the runtime makes")
	q := k.queries[0]
	assert.Equal(t, f.project, q.ProjectID)
	assert.Equal(t, []uuid.UUID{k.sources[0].ID}, q.Sources)
	assert.Equal(t, "record lunch 25000", q.Text)
	require.Len(t, k.deadlines, 1)
	assert.LessOrEqual(t, k.deadlines[0], 10*time.Second, "the search has a 10 s budget")

	req := f.rt.LastRequest()
	assert.Equal(t, []runtime.Passage{{Text: secretPassage, Title: "Refund policy", Heading: "Timing", Page: new(int32(2))}}, req.Knowledge)
}

func TestRun_KnowledgeQuerySendsTheTurnAndTheConversation(t *testing.T) {
	f := newSvc(t)
	k := f.withKnowledge()
	f.say(t, "my order is 42")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	f.say(t, "when does it ship")
	f.say(t, "and to Bandung?")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})

	require.Len(t, k.queries, 2)
	assert.Equal(t, "my order is 42", k.queries[0].Text)
	assert.Empty(t, k.queries[0].Context, "no previous message, no conversation input")
	assert.Equal(t, "when does it ship\nand to Bandung?", k.queries[1].Text, "this turn's text alone keeps its score")
	assert.Equal(t, "my order is 42\nwhen does it ship\nand to Bandung?", k.queries[1].Context,
		"the thread's previous user message, then this turn's text, carries a follow-up")

	f.say(t, "OLD-HEAD "+strings.Repeat("é", 3000))
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	for name, got := range map[string]string{"text": k.queries[2].Text, "context": k.queries[2].Context} {
		assert.Equal(t, 2000, utf8.RuneCountInString(got), "%s is cut at 2,000 characters", name)
		assert.True(t, utf8.ValidString(got), name)
		assert.Equal(t, strings.Repeat("é", 2000), got, "%s: the oldest text is cut first, the newest kept", name)
	}
}

func imageOnly() run.Message {
	body, _ := json.Marshal(map[string]any{"id": uuid.NewString(), "role": "user", "content": []map[string]any{
		{"type": "image", "id": "wamid.1", "source": map[string]string{"type": "data", "value": "aGk=", "mimeType": "image/jpeg"}},
	}})
	return run.Message{Role: run.RoleUser, Body: body}
}

func TestRun_NoUserTextNoSearch(t *testing.T) {
	f := newSvc(t)
	k := f.withKnowledge()
	_, err := f.store.AppendMessage(f.ctx, f.thread, imageOnly())
	require.NoError(t, err)
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Empty(t, k.queries, "a photo with no caption has no text to search")
	assert.Nil(t, f.rt.LastRequest().Knowledge)

	g := newSvc(t)
	none := g.withKnowledge()
	none.sources = nil
	g.say(t, "hello")
	g.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Empty(t, none.queries, "an agent with no bound source never searches")
	for _, r := range g.store.Runs {
		assert.Zero(t, r.KnowledgeRetrieveCalls)
	}
}

func TestRun_ResumeLegRepeatsSearch(t *testing.T) {
	f := newSvc(t)
	k := f.withKnowledge()
	f.say(t, "record lunch")
	first := f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "tool_call"))})
	require.Equal(t, run.StatusInterrupted, first.Status)
	again := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "tool_call")})
	assert.Equal(t, runtime.InputContinue, f.rt.LastRequest().Input.Kind)
	assert.Equal(t, []string{"record lunch", "record lunch"}, k.texts(), "a continue leg searches again with the run's own messages")
	assert.NotEmpty(t, f.rt.LastRequest().Knowledge)
	assert.Equal(t, int32(2), again.KnowledgeRetrieveCalls)
	assert.Equal(t, int32(2), again.KnowledgePassages, "one passage sent on each leg")
	assert.Equal(t, int64(14), again.Usage.EmbeddingTokens, "both legs' query tokens")

	g := newSvc(t)
	gk := g.withKnowledge()
	a := g.withApprovals()
	parked := g.await(t, a, "call_1")
	a.Decide(parked.ID, "approved", "")
	g.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, runtime.InputResume, g.rt.LastRequest().Input.Kind)
	assert.Equal(t, []string{"record lunch 25000", "record lunch 25000"}, gk.texts(), "an approval resume searches again")
	assert.NotEmpty(t, g.rt.LastRequest().Knowledge)
}

func TestRun_RetrieveFailureFailOpen(t *testing.T) {
	f := newSvc(t)
	var logs bytes.Buffer
	f.svc.WithLogger(slog.New(slog.NewJSONHandler(&logs, nil)))
	reader := sdkmetric.NewManualReader()
	f.svc.WithMeter(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test"))
	k := f.withKnowledge()
	k.err = errors.New("embeddings provider down")
	k.usage = run.KnowledgeUsage{EmbeddingTokens: 5, RetrieveCalls: 1}
	f.say(t, "how long do refunds take")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})

	assert.Equal(t, run.StatusCompleted, r.Status, "the turn runs without knowledge")
	assert.Nil(t, f.rt.LastRequest().Knowledge)
	assert.Contains(t, logs.String(), "ks_refunds", "the log names the sources by public id")
	assert.NotContains(t, logs.String(), "how long do refunds take", "never the query")
	assert.NotContains(t, logs.String(), "SECRET-PASSAGE")
	assert.Equal(t, int64(1), counter(t, reader, "knowledge.retrieve_errors"))
	assert.Equal(t, int64(5), r.Usage.EmbeddingTokens, "tokens the provider billed before the failure still count")
	assert.Equal(t, uint64(1), histogramCount(t, reader, "knowledge.retrieve_ms"))
}

func collect(t *testing.T, reader *sdkmetric.ManualReader, name string) *metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == name {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}

func counter(t *testing.T, reader *sdkmetric.ManualReader, name string) int64 {
	t.Helper()
	m := collect(t, reader, name)
	if m == nil {
		return 0
	}
	var n int64
	for _, dp := range m.Data.(metricdata.Sum[int64]).DataPoints {
		n += dp.Value
	}
	return n
}

func histogramCount(t *testing.T, reader *sdkmetric.ManualReader, name string) uint64 {
	t.Helper()
	m := collect(t, reader, name)
	if m == nil {
		return 0
	}
	var n uint64
	for _, dp := range m.Data.(metricdata.Histogram[int64]).DataPoints {
		n += dp.Count
	}
	return n
}

func TestRun_KnowledgeUsageRecorded(t *testing.T) {
	costAt := func(price int64) (*run.Run, *runtime.Budget) {
		f := newSvc(t)
		f.version.MaxCostCents = 100
		k := f.withKnowledge()
		k.price = new(price)
		k.usage = run.KnowledgeUsage{EmbeddingTokens: 1_000_000, RetrieveCalls: 1}
		f.say(t, "how long do refunds take")
		r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
		return r, f.rt.LastRequest().Limits.Budget
	}
	cheap, cheapBudget := costAt(20_000)
	dear, dearBudget := costAt(130_000)

	require.NotNil(t, cheap.Prices)
	assert.Equal(t, int64(20_000), *cheap.Prices.Embedding, "the embedding price is pinned at run open")
	assert.Equal(t, int64(1_000_000), cheap.Usage.EmbeddingTokens)
	assert.Equal(t, int32(1), cheap.KnowledgeRetrieveCalls)
	assert.GreaterOrEqual(t, cheap.KnowledgeRetrieveMs, int64(0))
	cheapCost, ok := run.Cost(cheap.Usage, cheap.Prices)
	require.True(t, ok)
	dearCost, ok := run.Cost(dear.Usage, dear.Prices)
	require.True(t, ok)
	assert.Equal(t, int64(110_000), dearCost-cheapCost, "embedding tokens are priced at the embedding price, not the main input price")
	assert.Equal(t, int64(100*10_000-20_000), cheapBudget.RemainingMicros, "the query's embedding cost is spent before the turn is sent")
	assert.Equal(t, int64(100*10_000-130_000), dearBudget.RemainingMicros)
}

func TestRun_NoPassageTextStored(t *testing.T) {
	f := newSvc(t)
	var logs bytes.Buffer
	f.svc.WithLogger(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	f.withKnowledge()
	f.say(t, "how long do refunds take")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.Contains(t, f.rt.LastRequest().Knowledge[0].Text, "SECRET-PASSAGE", "the runtime gets it")

	for _, m := range f.store.Messages {
		assert.NotContains(t, string(m.Body), "SECRET-PASSAGE")
	}
	stored, err := json.Marshal(f.store.Runs[r.ID])
	require.NoError(t, err)
	assert.NotContains(t, string(stored), "SECRET-PASSAGE")
	assert.NotContains(t, logs.String(), "SECRET-PASSAGE")
}

func TestRun_UnpricedEmbeddingNeverSearchedOnAPricedRun(t *testing.T) {
	for name, set := range map[string]func(k *fakeKnowledge){
		"no embedding price":      func(k *fakeKnowledge) { k.price = nil },
		"the price lookup failed": func(k *fakeKnowledge) { k.priceErr = errors.New("db down") },
	} {
		t.Run(name, func(t *testing.T) {
			f := newSvc(t)
			var logs bytes.Buffer
			f.svc.WithLogger(slog.New(slog.NewJSONHandler(&logs, nil)))
			f.version.MaxCostCents = 100
			k := f.withKnowledge()
			set(k)
			f.say(t, "record lunch")
			first := f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "tool_call"))})
			require.Equal(t, run.StatusInterrupted, first.Status)
			spent, ok := run.Cost(first.Usage, first.Prices)
			require.True(t, ok)
			require.Positive(t, spent)

			f.execute(t, fakes.RuntimeReply{Stream: stream(t, "tool_call")})
			assert.Empty(t, k.queries, "a priced run never takes on tokens it cannot price")
			budget := f.rt.LastRequest().Limits.Budget
			require.NotNil(t, budget)
			assert.Equal(t, int64(100*10_000)-spent, budget.RemainingMicros, "the second leg's budget subtracts the first leg's spend")
			assert.NotContains(t, logs.String(), f.project.String(), "never an internal id in the log")
		})
	}
}

func TestRun_PaidSearchesCappedPerRun(t *testing.T) {
	f := newSvc(t)
	var logs bytes.Buffer
	f.svc.WithLogger(slog.New(slog.NewJSONHandler(&logs, nil)))
	k := f.withKnowledge()
	k.usage = run.KnowledgeUsage{EmbeddingTokens: 1, RetrieveCalls: run.MaxRetrievesPerRun}
	f.say(t, "record lunch")
	first := f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "tool_call"))})
	require.Equal(t, int32(run.MaxRetrievesPerRun), first.KnowledgeRetrieveCalls)
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "tool_call")})
	assert.Len(t, k.queries, 1, "a run that already paid for its searches searches no more")
	assert.NotContains(t, logs.String(), "knowledge", "quietly")
}

func TestRun_CancelDuringSearchIsNoError(t *testing.T) {
	f := newSvc(t)
	var logs bytes.Buffer
	f.svc.WithLogger(slog.New(slog.NewJSONHandler(&logs, nil)))
	reader := sdkmetric.NewManualReader()
	f.svc.WithMeter(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test"))
	k := f.withKnowledge()
	ctx, cancel := context.WithCancel(f.ctx)
	k.onSearch = cancel
	k.err = context.Canceled
	f.say(t, "how long do refunds take")
	f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	token := f.claim(t)
	_, _ = f.svc.Execute(ctx, f.thread, token)
	assert.Zero(t, counter(t, reader, "knowledge.retrieve_errors"), "a cancel is not a failed search")
	assert.NotContains(t, logs.String(), "knowledge search failed")
}

func TestRun_EmbeddingPriceReadOnlyWithBoundSources(t *testing.T) {
	f := newSvc(t)
	k := f.withKnowledge()
	k.sources = nil
	f.say(t, "hello")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Zero(t, k.priceRead, "an agent with no knowledge never reads the embedding price")
	assert.Nil(t, r.Prices.Embedding)

	g := newSvc(t)
	gk := g.withKnowledge()
	g.say(t, "hello")
	r = g.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, 1, gk.priceRead)
	assert.Equal(t, int64(20_000), *r.Prices.Embedding)
}

func TestRun_KnowledgeUsesTheAgentsTuning(t *testing.T) {
	f := newSvc(t)
	k := f.withKnowledge()
	floor := 0.4
	f.version.Knowledge = run.KnowledgeTuning{K: 3, MinScore: &floor, MaxBytes: 16000}
	f.say(t, "refunds")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})

	require.Len(t, k.queries, 1)
	assert.Equal(t, 3, k.queries[0].K)
	require.NotNil(t, k.queries[0].MinScore)
	assert.InDelta(t, 0.4, *k.queries[0].MinScore, 1e-9)
}

func TestRun_KnowledgeUnsetTuningLeavesTheRecommendedValuesToTheRetriever(t *testing.T) {
	f := newSvc(t)
	k := f.withKnowledge()
	f.say(t, "refunds")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})

	require.Len(t, k.queries, 1)
	assert.Zero(t, k.queries[0].K)
	assert.Nil(t, k.queries[0].MinScore)
}

func TestRun_KnowledgeBudgetCutsThePassages(t *testing.T) {
	big := strings.Repeat("a", 6000)
	for budget, want := range map[int]int{7000: 1, 16000: 2, 32000: 3} {
		f := newSvc(t)
		k := f.withKnowledge()
		k.hits = []run.KnowledgeHit{{Text: big, Title: "1"}, {Text: big, Title: "2"}, {Text: big, Title: "3"}}
		f.version.Knowledge.MaxBytes = budget
		f.say(t, "refunds")
		r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
		assert.Len(t, f.rt.LastRequest().Knowledge, want, "a budget of %d bytes", budget)
		assert.Equal(t, int32(want), r.KnowledgePassages, "only the passages actually sent are counted")
	}
}

func TestRun_KnowledgePassagesCountOnlyWhatASentRequestCarried(t *testing.T) {
	f := newSvc(t)
	k := f.withKnowledge()
	delete(f.servers.Servers, f.server)
	f.say(t, "refunds")
	r := f.execute(t)
	require.Len(t, k.queries, 1, "the search ran")
	assert.Zero(t, r.KnowledgePassages, "the request failed to build, so no passage was sent")
}
