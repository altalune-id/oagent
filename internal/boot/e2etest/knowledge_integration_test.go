//go:build integration

package e2etest

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"hash/fnv"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/boot"
	"altalune.id/oagent/internal/dataplane"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/storage/memory"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/knowledge/crawl"
	"altalune.id/oagent/internal/knowledge/extract"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/testutil/runtimetest"
	"altalune.id/oagent/scope"
)

const (
	extractChildEnv = "OAGENT_E2E_EXTRACT_CHILD"
	embedDims       = 768
	embedModel      = "embed"
	otherSecret     = "SECRETB"
)

func TestMain(m *testing.M) {
	if os.Getenv(extractChildEnv) != "" {
		if err := extract.Serve(context.Background(), extract.Kind(os.Args[len(os.Args)-1]), os.Stdin, os.Stdout, extract.PDF{}); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func bagOfWords(text string) []float32 {
	v := make([]float32, embedDims)
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(w))
		v[h.Sum32()%embedDims]++
	}
	var norm float64
	for _, x := range v {
		norm += float64(x * x)
	}
	if norm == 0 {
		v[0] = 1
		return v
	}
	for i := range v {
		v[i] /= float32(math.Sqrt(norm))
	}
	return v
}

func embeddingsServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data := make([]map[string]any, len(req.Input))
		tokens := 0
		for i, in := range req.Input {
			data[i] = map[string]any{"index": i, "embedding": bagOfWords(in)}
			tokens += len(strings.Fields(in))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "usage": map[string]any{"prompt_tokens": tokens}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func helpSite(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><h1>Opening hours</h1><p>The help desk opens at 9 every weekday morning.</p></body></html>`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

type kStack struct {
	url, key, agentPub string
	agent              *agent.Agent
	chat               uuid.UUID
	admin              *sql.DB
	svcs               *boot.Services
	parts              boot.KnowledgeParts
	inA, inB           context.Context
	prjA, prjB         uuid.UUID
	embedURL           string
}

func newKnowledgeStack(t *testing.T, rt *runtimetest.Runtime, site *httptest.Server) *kStack {
	t.Helper()
	cfg := devLocalConfig()
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Runtime = config.RuntimeConfig{Driver: config.RuntimeDriverHTTP, URL: rt.URL(), Token: rt.Token()}
	cfg.Run = config.RunConfig{
		TurnTimeout: 2 * time.Minute, LeaseTTL: 30 * time.Second, Heartbeat: time.Second, DrainTimeout: 5 * time.Second,
		SweepEvery: time.Hour, MaxAttempts: 3, MaxConcurrentPerInstance: 4, MaxConcurrentPerProject: 4, MaxPerMinutePerProject: 100,
	}
	cfg.RateLimit.DataPlane = config.DataPlaneLimitConfig{PerKeyPerMinute: 600, PerIPPerMinute: 600}
	cfg.Knowledge = config.KnowledgeConfig{Pgvector: config.KnowledgePgvectorConfig{Enabled: true}, AllowPrivateHosts: true}
	cfg.Storage = config.StorageConfig{
		Driver: config.StorageDriverMemory, App: "oagent",
		Upload: config.StorageUploadConfig{URLTTL: 15 * time.Minute, PendingTTL: time.Hour},
	}
	pool, admin, schema := pgtest.MigratedRLS(t)
	cfg.Postgres.Schema = schema
	k := kernel.Kernel{Pool: pool, Reporter: apperror.NewReporter(nil, false)}
	svcs := boot.BuildServices(k, cfg)
	parts, err := boot.BuildKnowledge(t.Context(), &k, cfg, svcs, testLogger())
	require.NoError(t, err)
	svcs.Knowledge.WithExtractor(knowledge.ChildExtractor{Child: extract.Child{
		Path: os.Args[0], Args: []string{"-test.run=^$"}, Env: []string{extractChildEnv + "=1", "GORACE=atexit_sleep_ms=0"},
		Timeout: 30 * time.Second, MaxOutput: extract.MaxChildOutput, MemoryLimit: 256 << 20,
	}})
	// NOTE: the guarded client cannot trust the test site's certificate; the crawl keeps its rules and private-host flag.
	svcs.Knowledge.WithCrawler(func() knowledge.Crawler {
		return &crawl.Crawler{Client: site.Client(), Config: crawl.Config{AllowPrivateHosts: true, PerHostInterval: time.Millisecond}}
	})
	lims, err := boot.BuildLimiters(cfg, nil, testLogger())
	require.NoError(t, err)
	runs, err := boot.BuildRun(k, cfg, svcs, lims.Shared(), testLogger())
	require.NoError(t, err)
	svcs.RunStore, svcs.Runs, svcs.RunTurns, svcs.RunKicker, svcs.RuntimeReady = runs.Store, runs.Svc, runs.Runner, runs.Kicker, runs.Ready
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runs.Runner.Run(ctx)
	}()
	require.Eventually(t, runs.Runner.Running, 10*time.Second, time.Millisecond, "the runner starts")

	o, err := org.NewOrg("org-"+uuid.NewString()[:8], "Org", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, svcs.OrgStore.Save(tenant.WithOrg(t.Context(), o.ID), o))
	prjA, err := svcs.Projects.Create(t.Context(), project.CreateRequest{OrgID: o.ID, Slug: "main", Name: "Main"})
	require.NoError(t, err)
	prjB, err := svcs.Projects.Create(t.Context(), project.CreateRequest{OrgID: o.ID, Slug: "other", Name: "Other"})
	require.NoError(t, err)
	s := &kStack{
		admin: admin, svcs: svcs, parts: parts, prjA: prjA.ID, prjB: prjB.ID, embedURL: embeddingsServer(t).URL + "/v1",
		inA: tenant.Into(t.Context(), tenant.Context{OrgID: o.ID, ProjectID: prjA.ID}),
		inB: tenant.Into(t.Context(), tenant.Context{OrgID: o.ID, ProjectID: prjB.ID}),
	}
	s.embeddingModel(t, s.inA, prjA.ID)
	s.embeddingModel(t, s.inB, prjB.ID)

	chat, err := svcs.ModelProviders.Create(s.inA, prjA.ID, modelprovider.Input{
		Name: "fake", Kind: modelprovider.KindOpenAICompatible, BaseURL: rt.ModelBaseURL(),
		Models: []modelprovider.Model{{ID: runtimetest.ModelID}},
	}, "sk-fake")
	require.NoError(t, err)
	exp := time.Now().Add(time.Hour)
	_, s.key, err = svcs.APIKeys.Create(t.Context(), apikey.CreateRequest{
		OrgID: o.ID, Kind: apikey.KindOrg, AllProjects: true, Name: "integrator", ExpiresAt: &exp,
		Scopes: []string{scope.RunCreate, scope.RunRead},
	})
	require.NoError(t, err)
	srv := httptest.NewServer(boot.BuildDataPlane(cfg, svcs, lims, k.Reporter.Unexpected, testLogger()))
	t.Cleanup(srv.Close)
	// NOTE: registered last so it runs first: the runner drains before the server closes.
	t.Cleanup(func() {
		stop()
		<-done
		parts.Ingest.Wait()
		parts.Crawl.Wait()
	})
	s.url, s.chat = srv.URL, chat.ID
	s.agent = s.newAgent(t, o.ID)
	s.agentPub = s.agent.PublicID
	return s
}

func (s *kStack) embeddingModel(t *testing.T, in context.Context, prj uuid.UUID) {
	t.Helper()
	p, err := s.svcs.ModelProviders.Create(in, prj, modelprovider.Input{
		Name: "embeddings", Kind: modelprovider.KindOpenAICompatible, BaseURL: s.embedURL,
		Models: []modelprovider.Model{{ID: embedModel, Embedding: true, Prices: &modelprovider.Prices{InputMicros: 20_000}}},
	}, "sk-embed")
	require.NoError(t, err)
	st, err := s.svcs.Knowledge.SetEmbeddingModel(in, prj, p.ID, embedModel)
	require.NoError(t, err)
	require.Equal(t, embedDims, st.Dims)
}

func (s *kStack) upload(t *testing.T, in context.Context, prj uuid.UUID, name, filename, contentType string, body []byte) uuid.UUID {
	t.Helper()
	src, err := s.svcs.Knowledge.CreateSource(in, prj, knowledge.SourceInput{Kind: knowledge.KindUpload, Backend: knowledge.BackendPgvector, Name: name})
	require.NoError(t, err)
	doc, _, err := s.svcs.Knowledge.BeginUpload(in, src.ID, knowledge.UploadInput{Filename: filename, ContentType: contentType, Size: int64(len(body))})
	require.NoError(t, err)
	var key string
	require.NoError(t, s.admin.QueryRowContext(t.Context(), `SELECT object_key FROM files WHERE id = $1`, *doc.FileID).Scan(&key))
	driver, ok := s.parts.Driver.(*memory.Driver)
	require.True(t, ok, "the stack runs on the memory driver")
	driver.Put(key, contentType, body)
	_, err = s.svcs.Knowledge.CompleteUpload(in, doc.ID)
	require.NoError(t, err)
	return src.ID
}

func (s *kStack) newAgent(t *testing.T, orgID uuid.UUID) *agent.Agent {
	t.Helper()
	a, err := agent.New(orgID, s.prjA, "bot", "Bot", "", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, s.svcs.AgentStore.Save(s.inA, a))
	return a
}

func (s *kStack) bind(t *testing.T, a *agent.Agent, providerID uuid.UUID, sources ...uuid.UUID) {
	t.Helper()
	cfg := agent.Config{Instructions: "Answer from the knowledge passages.", Model: runtimetest.ModelID, ProviderID: providerID, KnowledgeSourceIDs: sources}
	require.NoError(t, a.SetDraft(cfg, time.Now()))
	v, _, err := a.Publish(nil, uuid.Nil, time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, errors.Join(s.svcs.AgentStore.InsertVersion(s.inA, v), s.svcs.AgentStore.MarkPublished(s.inA, a), s.svcs.AgentStore.SaveDraft(s.inA, a)))
}

func (s *kStack) ask(t *testing.T, thread, question string) (runID, answer string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.url+"/api/v1/agents/"+s.agentPub+"/runs", strings.NewReader(
		`{"threadId":"`+thread+`","runId":"r","messages":[{"id":"`+uuid.NewString()+`","role":"user","content":"`+question+`"}]}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(dataplane.UserHeader, "alice")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	events := readSSE(t, resp.Body)
	require.NotEmpty(t, events)
	var text strings.Builder
	for _, e := range events {
		text.WriteString(e.Delta)
	}
	runID = events[0].RunID
	require.Eventually(t, func() bool {
		var status string
		err := s.admin.QueryRowContext(t.Context(), `SELECT status FROM runs WHERE public_id = $1`, runID).Scan(&status)
		return err == nil && status == "completed"
	}, 30*time.Second, 50*time.Millisecond)
	return runID, text.String()
}

func (s *kStack) usage(t *testing.T, runID string) (embeddingTokens int64, retrieveCalls int32) {
	t.Helper()
	require.NoError(t, s.admin.QueryRowContext(t.Context(),
		`SELECT embedding_tokens, knowledge_retrieve_calls FROM runs WHERE public_id = $1`, runID).Scan(&embeddingTokens, &retrieveCalls))
	return embeddingTokens, retrieveCalls
}

func modelText(t *testing.T, body []byte) string {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal(body, &v))
	var b strings.Builder
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			b.WriteString(x)
			b.WriteByte('\n')
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(v)
	return b.String()
}

func (s *kStack) ingest(t *testing.T) {
	t.Helper()
	s.parts.Ingest.Round(t.Context())
	s.parts.Ingest.Wait()
}

func (s *kStack) documentsReady(in context.Context, t *testing.T, src uuid.UUID) {
	t.Helper()
	docs, err := s.svcs.Knowledge.ListDocuments(in, src)
	require.NoError(t, err)
	require.NotEmpty(t, docs)
	for _, d := range docs {
		require.Equal(t, knowledge.DocumentReady, d.Status, "%s: %+v", d.Title, d.Failure)
	}
}

// SECURITY: a run sees only its own project's passages, whatever its version names, and the runtime gets each passage's source.
func TestKnowledge_AgainstRuntimeContainer(t *testing.T) {
	model := runtimetest.NewFakeModel(t,
		runtimetest.Reply{Chunks: []string{"Refunds are paid within 7 days ", "[1]."}, InputTokens: 200, OutputTokens: 12},
		runtimetest.Reply{Chunks: []string{"The help desk opens at 9 ", "[1]."}, InputTokens: 200, OutputTokens: 12},
	)
	site := helpSite(t)
	s := newKnowledgeStack(t, runtimetest.StartRuntime(t, model), site)

	pdf, err := os.ReadFile("../../knowledge/testdata/docs/text_3pages.pdf")
	require.NoError(t, err)
	policies := s.upload(t, s.inA, s.prjA, "Policies", "policy.pdf", knowledge.TypePDF, pdf)
	help, err := s.svcs.Knowledge.CreateSource(s.inA, s.prjA, knowledge.SourceInput{
		Kind: knowledge.KindURL, Backend: knowledge.BackendPgvector, Name: "Help", SeedURL: site.URL + "/",
	})
	require.NoError(t, err)
	foreign := s.upload(t, s.inB, s.prjB, "Theirs", "theirs.md", knowledge.TypeMarkdown,
		[]byte("# Refund policy\n\n"+otherSecret+" refunds are paid within 7 days. The help desk opens at 9 every weekday.\n"))

	s.parts.Crawl.Round(t.Context())
	s.parts.Crawl.Wait()
	s.ingest(t)
	s.documentsReady(s.inA, t, policies)
	s.documentsReady(s.inA, t, help.ID)
	s.documentsReady(s.inB, t, foreign)

	s.bind(t, s.agent, s.chat, policies, help.ID, foreign)

	t.Run("an uploaded PDF answers with its cited page", func(t *testing.T) {
		runID, answer := s.ask(t, "pdf", "When are refunds paid?")
		assert.Equal(t, "Refunds are paid within 7 days [1].", answer)
		reqs := model.Requests()
		require.Len(t, reqs, 1)
		sent := modelText(t, reqs[0])
		assert.Contains(t, sent, "Refund policy. Refunds are paid within 7 days.")
		assert.Contains(t, sent, `title="policy.pdf"`)
		assert.Contains(t, sent, `page="1"`)
		assert.NotContains(t, sent, otherSecret, "another project's passage never reaches the model")
		tokens, calls := s.usage(t, runID)
		assert.Positive(t, tokens, "the query embedding is billed on the run")
		assert.Equal(t, int32(1), calls)
	})

	t.Run("a crawled page answers with its cited URL", func(t *testing.T) {
		runID, answer := s.ask(t, "site", "When does the help desk open?")
		assert.Equal(t, "The help desk opens at 9 [1].", answer)
		reqs := model.Requests()
		require.Len(t, reqs, 2)
		sent := modelText(t, reqs[1])
		assert.Contains(t, sent, "The help desk opens at 9 every weekday morning.")
		assert.Contains(t, sent, `url="`+site.URL+`/"`)
		assert.NotContains(t, sent, otherSecret, "another project's passage never reaches the model")
		tokens, calls := s.usage(t, runID)
		assert.Positive(t, tokens)
		assert.Equal(t, int32(1), calls)
	})
}
