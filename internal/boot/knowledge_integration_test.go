//go:build integration

package boot

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/testutil/s3test"
)

type oneVersion struct {
	agent.Store
	v *agent.Version
}

func (o oneVersion) VersionByID(_ context.Context, id uuid.UUID) (*agent.Version, error) {
	if id != o.v.ID {
		return nil, &agent.NotFoundError{}
	}
	return o.v, nil
}

func (g grantScope) sourceIn(t *testing.T, f *apiFixture, orgID, projectID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := f.db.ExecContext(t.Context(), `INSERT INTO knowledge_sources (id, public_id, org_id, project_id, kind, backend, name, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'upload', 'pgvector', $2, 'idle', now(), now())`, id, "s"+id.String()[:20], orgID, projectID)
	require.NoError(t, err)
	return id
}

// SECURITY: a run searches only live sources of its own project, whatever a version's config names.
func TestRunKnowledgeBindings_OnlyLiveSourcesOfTheProject(t *testing.T) {
	f := newAPIFixture(t)
	g := newGrantScope(t, f)
	other := newGrantScope(t, f)
	sibling, err := f.svcs.Projects.Create(g.ctx, project.CreateRequest{OrgID: g.orgID, Slug: "sib", Name: "Sib"})
	require.NoError(t, err)

	mine := g.sourceIn(t, f, g.orgID, g.projectID)
	gone := g.sourceIn(t, f, g.orgID, g.projectID)
	_, err = f.db.ExecContext(t.Context(), `DELETE FROM knowledge_sources WHERE id = $1`, gone)
	require.NoError(t, err)
	foreignOrg := g.sourceIn(t, f, other.orgID, other.projectID)
	siblingPrj := g.sourceIn(t, f, g.orgID, sibling.ID)

	v := &agent.Version{ID: uuid.New(), OrgID: g.orgID, ProjectID: g.projectID, Config: agent.Config{
		KnowledgeSourceIDs: []uuid.UUID{mine, gone, foreignOrg, siblingPrj, uuid.New()},
	}}
	b := runKnowledge{agents: oneVersion{v: v}, knowledge: f.svcs.Knowledge}
	got, err := b.BoundSources(g.ctx, v.ID)
	require.NoError(t, err)
	mineSrc, err := f.svcs.Knowledge.GetSource(g.ctx, mine)
	require.NoError(t, err)
	assert.Equal(t, []run.KnowledgeSource{{ID: mine, PublicID: mineSrc.PublicID}}, got)

	none, err := b.BoundSources(g.ctx, uuid.New())
	assert.Error(t, err, "an unknown version is an error, not an empty list")
	assert.Empty(t, none)
}

func embeddingsServer(t *testing.T, dims int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  []map[string]any{{"index": 0, "embedding": make([]float32, dims)}},
			"usage": map[string]any{"prompt_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func knowledgeServices(t *testing.T, pool db.Pool, schemaName string, env config.Env, allow bool) *Services {
	t.Helper()
	svcs, _ := knowledgeStack(t, pool, schemaName, env, allow)
	return svcs
}

func knowledgeStack(t *testing.T, pool db.Pool, schemaName string, env config.Env, allow bool) (*Services, knowledgeParts) {
	t.Helper()
	cfg := devLocalConfig()
	cfg.Env = env
	cfg.Postgres.Schema = schemaName
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Knowledge.AllowPrivateHosts = allow
	cfg.Knowledge.Pgvector.Enabled = true
	k := kernel.Kernel{Pool: pool, Reporter: apperror.NewReporter(nil, false)}
	svcs := buildServices(k, cfg)
	parts, err := buildKnowledge(t.Context(), &k, cfg, svcs, nil)
	require.NoError(t, err)
	return svcs, parts
}

// SECURITY: a local embeddings server is reachable outside production, and in production only when the operator allows it.
func TestKnowledge_PrivateEmbeddingsHostByEnv(t *testing.T) {
	srv := embeddingsServer(t, 768)
	pool, _, schemaName := pgtest.MigratedRLS(t)
	dev := knowledgeServices(t, pool, schemaName, config.EnvDevelopment, false)
	for name, tc := range map[string]struct {
		env   config.Env
		allow bool
		ok    bool
	}{
		"development":                {config.EnvDevelopment, false, true},
		"production":                 {config.EnvProduction, false, false},
		"production, operator allow": {config.EnvProduction, true, true},
	} {
		t.Run(name, func(t *testing.T) {
			g := newGrantScope(t, &apiFixture{svcs: dev})
			p, err := dev.ModelProviders.Create(g.ctx, g.projectID, modelprovider.Input{
				Name: "local", Kind: modelprovider.KindOpenAICompatible, BaseURL: srv.URL + "/v1",
				Models: []modelprovider.Model{{ID: "e", Embedding: true, Prices: &modelprovider.Prices{InputMicros: 1}}},
			}, "sk-test")
			require.NoError(t, err)

			svcs := knowledgeServices(t, pool, schemaName, tc.env, tc.allow)
			st, err := svcs.Knowledge.SetEmbeddingModel(g.ctx, g.projectID, p.ID, "e")
			if tc.ok {
				require.NoError(t, err)
				assert.Equal(t, 768, st.Dims)
				return
			}
			assert.Equal(t, apperror.CodeKnowledgeEmbeddingFailed, codeOfErr(err), "the guard refuses 127.0.0.1: %v", err)
		})
	}
}

func TestGrantKnowledge_UnknownOrGoneSourceIsInvalidGrant(t *testing.T) {
	f := newAPIFixture(t)
	g := newGrantScope(t, f)
	other := newGrantScope(t, f)
	mine := g.sourceIn(t, f, g.orgID, g.projectID)
	theirs := g.sourceIn(t, f, other.orgID, other.projectID)
	grants := grantKnowledge{knowledge: f.svcs.Knowledge}

	require.NoError(t, grants.SourceExists(g.ctx, g.projectID, mine))
	for name, id := range map[string]uuid.UUID{"unknown": uuid.New(), "another org's": theirs} {
		var invalid *agent.InvalidGrantError
		require.ErrorAs(t, grants.SourceExists(g.ctx, g.projectID, id), &invalid, name)
		assert.Equal(t, agent.GrantKnowledgeSource, invalid.Reason, name)
	}

	require.NoError(t, db.NewTransactor(f.pool).RunInTx(g.ctx, func(ctx context.Context) error {
		return grants.LockSourcesShared(ctx, []uuid.UUID{mine})
	}))
	err := db.NewTransactor(f.pool).RunInTx(g.ctx, func(ctx context.Context) error {
		return grants.LockSourcesShared(ctx, []uuid.UUID{mine, uuid.New()})
	})
	var invalid *agent.InvalidGrantError
	require.ErrorAs(t, err, &invalid, "a source gone before the lock fails the publish")
	assert.Equal(t, agent.GrantKnowledgeSource, invalid.Reason)
}

func TestPublicIDs_KnowledgeSource(t *testing.T) {
	f := newAPIFixture(t)
	g := newGrantScope(t, f)
	id := g.sourceIn(t, f, g.orgID, g.projectID)
	refs := newPublicIDs(f.svcs)

	pub, ok, err := refs.KnowledgeSourcePublicID(g.ctx, id)
	require.NoError(t, err)
	require.True(t, ok)
	gotID, gotPrj, ok, err := refs.KnowledgeSourceID(g.ctx, pub)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, id, gotID)
	assert.Equal(t, g.projectID, gotPrj)

	_, ok, err = refs.KnowledgeSourcePublicID(g.ctx, uuid.New())
	require.NoError(t, err, "a gone source is absent, not an error")
	assert.False(t, ok)
	_, _, ok, err = refs.KnowledgeSourceID(g.ctx, "unknownsource00000001")
	require.NoError(t, err)
	assert.False(t, ok)

	off := publicIDs{}
	_, ok, err = off.KnowledgeSourcePublicID(g.ctx, id)
	assert.NoError(t, err)
	assert.False(t, ok, "without knowledge nothing resolves")
	_, _, ok, err = off.KnowledgeSourceID(g.ctx, pub)
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestBuildStorageDriver_ByConfig(t *testing.T) {
	cfg := devLocalConfig()
	k := &kernel.Kernel{}

	d, h, err := buildStorageDriver(t.Context(), k, cfg, nil)
	require.NoError(t, err)
	assert.Nil(t, d, "no driver: uploads are off")
	assert.Nil(t, h)

	cfg.Storage.Driver = "ftp"
	_, _, err = buildStorageDriver(t.Context(), k, cfg, nil)
	require.Error(t, err, "an unknown driver fails boot instead of turning uploads off")

	cfg.Storage.Driver = config.StorageDriverMemory
	d, h, err = buildStorageDriver(t.Context(), k, cfg, nil)
	require.NoError(t, err)
	assert.NotNil(t, d)
	assert.Nil(t, h, "memory has no health check")

	cfg.Storage.Driver = config.StorageDriverS3
	cfg.S3 = config.S3Config{Endpoint: "ftp://s3.example.com", Region: "auto", Bucket: "b", AccessKeyID: "k", SecretAccessKey: "s"}
	_, _, err = buildStorageDriver(t.Context(), k, cfg, nil)
	require.Error(t, err, "a bad endpoint fails boot")

	s3 := s3test.New(t)
	cfg.S3 = config.S3Config{
		Endpoint: s3.Endpoint, Region: s3.Region, Bucket: s3.Bucket, AccessKeyID: s3.AccessKeyID, SecretAccessKey: s3.SecretAccessKey,
		UsePathStyle: true, Timeout: 5 * time.Second,
	}
	k = &kernel.Kernel{}
	d, h, err = buildStorageDriver(t.Context(), k, cfg, nil)
	require.NoError(t, err)
	assert.NotNil(t, d)
	assert.NotNil(t, h, "s3 gets a health check")
	assert.NoError(t, k.Close(), "the client is closed with the kernel")
}

// SECURITY: knowledge.allowPrivateHosts opens only the embeddings client; a production crawler never reaches a private host.
func TestKnowledge_CrawlerPrivateHostsOnlyOutsideProduction(t *testing.T) {
	pool, _, schemaName := pgtest.MigratedRLS(t)
	for name, tc := range map[string]struct {
		env   config.Env
		allow bool
		dials bool
	}{
		"development":                {config.EnvDevelopment, false, true},
		"production":                 {config.EnvProduction, false, false},
		"production, operator allow": {config.EnvProduction, true, false},
	} {
		t.Run(name, func(t *testing.T) {
			var conns atomic.Int32
			site := httptest.NewUnstartedServer(http.NotFoundHandler())
			site.Config.ConnState = func(_ net.Conn, s http.ConnState) {
				if s == http.StateNew {
					conns.Add(1)
				}
			}
			site.StartTLS()
			t.Cleanup(site.Close)

			svcs, parts := knowledgeStack(t, pool, schemaName, tc.env, tc.allow)
			g := newGrantScope(t, &apiFixture{svcs: svcs})
			_, err := svcs.Knowledge.CreateSource(g.ctx, g.projectID, knowledge.SourceInput{
				Kind: knowledge.KindURL, Backend: knowledge.BackendPgvector, Name: "Local", SeedURL: site.URL + "/",
			})
			require.NoError(t, err)
			parts.crawl.Round(tenant.WithOrg(context.Background(), g.orgID))
			parts.crawl.Wait()
			if tc.dials {
				assert.Positive(t, conns.Load(), "outside production the crawler reaches the local site")
				return
			}
			assert.Zero(t, conns.Load(), "the guard refuses the dial before any connection")
		})
	}
}

func TestBuildKnowledge_WarnsWhenProductionOpensPrivateEmbeddingsHosts(t *testing.T) {
	pool, _, schemaName := pgtest.MigratedRLS(t)
	for name, tc := range map[string]struct {
		env   config.Env
		allow bool
		warn  bool
	}{
		"production, operator allow": {config.EnvProduction, true, true},
		"production":                 {config.EnvProduction, false, false},
		"development, flag on":       {config.EnvDevelopment, true, false},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := devLocalConfig()
			cfg.Env = tc.env
			cfg.Postgres.Schema = schemaName
			cfg.Secrets.EncryptionKey = testKeyHex
			cfg.Knowledge.AllowPrivateHosts = tc.allow
			k := kernel.Kernel{Pool: pool, Reporter: apperror.NewReporter(nil, false)}
			log, buf := bufferLogger()
			_, err := buildKnowledge(t.Context(), &k, cfg, buildServices(k, cfg), log)
			require.NoError(t, err)
			warned := strings.Contains(buf.String(), "level=WARN") && strings.Contains(buf.String(), "knowledge.allowPrivateHosts")
			assert.Equal(t, tc.warn, warned, buf.String())
		})
	}
}
