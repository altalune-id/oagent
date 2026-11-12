//go:build integration

package pgvector_test

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/knowledge/embed"
	"altalune.id/oagent/internal/knowledge/knowledgetest"
	"altalune.id/oagent/internal/knowledge/pgvector"
	"altalune.id/oagent/internal/testutil/pgtest"
)

var provider = uuid.MustParse("0193a2b4-c5d6-7e8f-9a0b-1c2d3e4f5a6b")

type embedders struct {
	dims int
	emb  knowledge.Embedder
}

func (e embedders) settings(project uuid.UUID) *knowledge.Settings {
	return &knowledge.Settings{ProjectID: project, ProviderID: provider, Model: "text-embedding-3-small", Dims: e.dims}
}

func (e embedders) Settings(_ context.Context, project uuid.UUID) (*knowledge.Settings, error) {
	return e.settings(project), nil
}

func (e embedders) EmbedderFor(context.Context, *knowledge.Settings) (knowledge.Embedder, error) {
	return e.emb, nil
}

type fixed struct {
	dims    int
	vectors map[string][]float32
}

func (f fixed) Probe(context.Context) (int, error) { return f.dims, nil }

func (f fixed) Embed(_ context.Context, texts []string) (embed.Result, error) {
	res := embed.Result{Dims: f.dims, Tokens: int64(len(texts))}
	for _, t := range texts {
		res.Vectors = append(res.Vectors, f.vectors[t])
	}
	return res, nil
}

type backend struct {
	pool   db.Pool
	admin  *sql.DB
	schema string
	emb    embedders
	vector func(text string) []float32
}

func newBackend(t *testing.T, dims int, emb knowledge.Embedder, vector func(string) []float32) *backend {
	t.Helper()
	pool, admin, schemaName := pgtest.MigratedRLS(t)
	return &backend{pool: pool, admin: admin, schema: schemaName, emb: embedders{dims: dims, emb: emb}, vector: vector}
}

func (b *backend) Retriever() knowledge.Retriever {
	return pgvector.New(b.pool, b.schema, "", b.emb)
}

func (b *backend) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	_, err := b.admin.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)
}

func (b *backend) Project(t *testing.T) (context.Context, uuid.UUID) {
	t.Helper()
	org := uuid.New()
	b.exec(t, `INSERT INTO orgs (id, public_id, slug, name, created_at, updated_at) VALUES ($1, $2, $2, 'Org', now(), now())`, org, "o"+org.String()[:12])
	ctx := tenant.Into(context.Background(), tenant.Context{OrgID: org})
	return ctx, b.ProjectInOrg(t, ctx)
}

func (b *backend) ProjectInOrg(t *testing.T, ctx context.Context) uuid.UUID {
	t.Helper()
	tc, err := tenant.From(ctx)
	require.NoError(t, err)
	prj := uuid.New()
	b.exec(t, `INSERT INTO projects (id, public_id, org_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, $2, 'P', now(), now())`,
		prj, "p"+prj.String()[:12], tc.OrgID)
	return prj
}

func (b *backend) Source(t *testing.T, ctx context.Context, project uuid.UUID) uuid.UUID {
	t.Helper()
	tc, err := tenant.From(ctx)
	require.NoError(t, err)
	src := uuid.New()
	b.exec(t, `INSERT INTO knowledge_sources (id, public_id, org_id, project_id, kind, backend, name, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'upload', 'pgvector', $2, 'idle', now(), now())`, src, "ks"+src.String()[:12], tc.OrgID, project)
	return src
}

func (b *backend) document(t *testing.T, org, project, source uuid.UUID, title string) uuid.UUID {
	t.Helper()
	doc := uuid.New()
	b.exec(t, `INSERT INTO knowledge_documents (id, public_id, org_id, project_id, source_id, title, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'ready', now(), now())`, doc, "kd"+doc.String()[:12], org, project, source, title)
	return doc
}

func (b *backend) Add(t *testing.T, ctx context.Context, p knowledgetest.Passage) {
	t.Helper()
	tc, err := tenant.From(ctx)
	require.NoError(t, err)
	doc := b.document(t, tc.OrgID, p.ProjectID, p.SourceID, p.Title)
	dims, vector, key := b.emb.dims, b.vector(p.Text), b.emb.settings(p.ProjectID).ModelKey()
	if p.Stale {
		key = "stale/" + key
	}
	if p.OtherSize {
		dims = 1024
		if b.emb.dims == 1024 {
			dims = 768
		}
		vector = knowledgetest.Vector(dims, p.Text)
	}
	b.exec(t, `INSERT INTO knowledge_chunks (id, org_id, project_id, source_id, document_id, ordinal, heading_path, page, url, text, embedding, dims, model_key, created_at)
		VALUES ($1, $2, $3, $4, $5, 0, $6, NULLIF($7, 0), NULLIF($8, ''), $9, $10::public.vector, $11, $12, now())`,
		uuid.New(), tc.OrgID, p.ProjectID, p.SourceID, doc, p.Heading, p.Page, p.URL, p.Text,
		knowledge.VectorText(vector), dims, key)
}

func TestRetriever_Contract(t *testing.T) {
	for _, dims := range []int{768, 3072} {
		t.Run(strconv.Itoa(dims), func(t *testing.T) {
			knowledgetest.Run(t, func(t *testing.T) knowledgetest.Backend {
				return newBackend(t, dims, &knowledgetest.Embedder{Dims: dims}, func(text string) []float32 { return knowledgetest.Vector(dims, text) })
			})
		})
	}
}

func unit(dims int, weights map[int]float64) []float32 {
	v := make([]float32, dims)
	for i, w := range weights {
		v[i] = float32(w)
	}
	return v
}

func cosineTo(dims int, c float64) []float32 {
	return unit(dims, map[int]float64{0: c, 1: math.Sqrt(1 - c*c)})
}

func keyed(dims int, texts map[string][]float32) (fixed, func(string) []float32) {
	f := fixed{dims: dims, vectors: texts}
	return f, func(text string) []float32 { return texts[text] }
}

func TestRetriever_CrossProjectChunkNeverReturned(t *testing.T) {
	emb, vec := keyed(768, map[string][]float32{"q": cosineTo(768, 1), "same": cosineTo(768, 1)})
	b := newBackend(t, 768, emb, vec)
	ctx, prj := b.Project(t)
	src := b.Source(t, ctx, prj)
	sibling := b.ProjectInOrg(t, ctx)
	siblingSrc := b.Source(t, ctx, sibling)
	otherCtx, other := b.Project(t)
	otherSrc := b.Source(t, otherCtx, other)
	b.Add(t, ctx, knowledgetest.Passage{ProjectID: sibling, SourceID: siblingSrc, Title: "Sibling", Text: "same"})
	b.Add(t, otherCtx, knowledgetest.Passage{ProjectID: other, SourceID: otherSrc, Title: "Other org", Text: "same"})

	all := []uuid.UUID{src, siblingSrc, otherSrc}
	hits, _, err := b.Retriever().Search(ctx, knowledge.Query{ProjectID: prj, Sources: all, Text: "q"})
	require.NoError(t, err)
	assert.Empty(t, hits, "the project filter keeps a sibling project's chunk out")

	hits, _, err = b.Retriever().Search(ctx, knowledge.Query{ProjectID: other, Sources: all, Text: "q"})
	require.NoError(t, err)
	assert.Empty(t, hits, "row-level security keeps another org's chunk out even when its project is asked for")

	hits, _, err = b.Retriever().Search(otherCtx, knowledge.Query{ProjectID: other, Sources: all, Text: "q"})
	require.NoError(t, err)
	assert.Len(t, hits, 1, "its own org finds it")
}

func TestRetriever_UnboundSourceNeverSearched(t *testing.T) {
	emb, vec := keyed(1024, map[string][]float32{"q": cosineTo(1024, 1), "bound": cosineTo(1024, 0.9), "unbound": cosineTo(1024, 1)})
	b := newBackend(t, 1024, emb, vec)
	ctx, prj := b.Project(t)
	bound, unbound := b.Source(t, ctx, prj), b.Source(t, ctx, prj)
	b.Add(t, ctx, knowledgetest.Passage{ProjectID: prj, SourceID: bound, Title: "B", Text: "bound"})
	b.Add(t, ctx, knowledgetest.Passage{ProjectID: prj, SourceID: unbound, Title: "U", Text: "unbound"})

	hits, usage, err := b.Retriever().Search(ctx, knowledge.Query{ProjectID: prj, Sources: []uuid.UUID{bound}, Text: "q"})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, bound, hits[0].Source)
	assert.Equal(t, knowledge.Usage{EmbeddingTokens: 1, RetrieveCalls: 1}, usage)
}

func TestRetriever_ScoreThreshold(t *testing.T) {
	emb, vec := keyed(1536, map[string][]float32{
		"q": cosineTo(1536, 1), "same": cosineTo(1536, 1), "c30": cosineTo(1536, 0.3), "c10": cosineTo(1536, 0.1), "orthogonal": cosineTo(1536, 0),
	})
	b := newBackend(t, 1536, emb, vec)
	ctx, prj := b.Project(t)
	src := b.Source(t, ctx, prj)
	for _, text := range []string{"c10", "orthogonal", "c30", "same"} {
		b.Add(t, ctx, knowledgetest.Passage{ProjectID: prj, SourceID: src, Title: text, Text: text})
	}

	hits, _, err := b.Retriever().Search(ctx, knowledge.Query{ProjectID: prj, Sources: []uuid.UUID{src}, Text: "q", K: 10})
	require.NoError(t, err)
	require.Len(t, hits, 2)
	assert.Equal(t, []string{"same", "c30"}, []string{hits[0].Text, hits[1].Text})
	assert.InDelta(t, 1, hits[0].Score, 1e-4)
	assert.InDelta(t, 0.3, hits[1].Score, 1e-4)
}

func (b *backend) bulk(t *testing.T, ctx context.Context, project, source uuid.UUID, n, dims int, cosine float64) {
	t.Helper()
	tc, err := tenant.From(ctx)
	require.NoError(t, err)
	doc := b.document(t, tc.OrgID, project, source, "Bulk")
	b.exec(t, `INSERT INTO knowledge_chunks (id, org_id, project_id, source_id, document_id, ordinal, text, embedding, dims, model_key, created_at)
		SELECT gen_random_uuid(), $1, $2, $3, $4, g, 'chunk '||g,
			('['||array_to_string(ARRAY(SELECT CASE WHEN i = 1 THEN $5::float8 WHEN i = 2 THEN sqrt(1 - $5::float8 ^ 2)
				ELSE (random() - 0.5) * 0.004 + g * 0 END FROM generate_series(1, $6::int) i), ',')||']')::public.vector,
			$6::smallint, $7, now()
		FROM generate_series(0, $8::int - 1) g`,
		tc.OrgID, project, source, doc, cosine, dims, b.emb.settings(project).ModelKey(), n)
	b.exec(t, `ANALYZE knowledge_chunks`)
}

func TestRetriever_SmallProjectGetsHitsBesideLargeOne(t *testing.T) {
	emb, vec := keyed(768, map[string][]float32{"q": cosineTo(768, 1)})
	b := newBackend(t, 768, emb, vec)
	largeCtx, large := b.Project(t)
	b.bulk(t, largeCtx, large, b.Source(t, largeCtx, large), 4000, 768, 1)
	ctx, small := b.Project(t)
	src := b.Source(t, ctx, small)
	b.bulk(t, ctx, small, src, 1000, 768, 0.8)

	b.exec(t, `DROP INDEX knowledge_chunks_scope_idx`)
	b.pool.R.SetMaxOpenConns(1)
	conn, err := b.pool.R.Conn(t.Context())
	require.NoError(t, err)
	for _, stmt := range []string{"SET enable_seqscan = off", "SET enable_bitmapscan = off"} {
		_, err := conn.ExecContext(t.Context(), stmt)
		require.NoError(t, err)
	}
	r := pgvector.New(b.pool, b.schema, "", b.emb)
	tc, err := tenant.From(ctx)
	require.NoError(t, err)
	args := []any{knowledge.VectorText(vec("q")), small, "{" + src.String() + "}", b.emb.settings(small).ModelKey(), knowledge.DefaultSearchK}
	tx, err := conn.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
	require.NoError(t, err)
	_, err = tx.ExecContext(t.Context(), "SELECT set_config('app.current_org_id', $1, true)", tc.OrgID.String())
	require.NoError(t, err)
	require.Contains(t, explain(t, tx, pgvector.Statement(r, 768), args...), "knowledge_chunks_hnsw_768", "the plan a table too large for a scope scan gets")
	count := func(mode string) int {
		t.Helper()
		_, err := tx.ExecContext(t.Context(), "SET LOCAL hnsw.iterative_scan = "+mode)
		require.NoError(t, err)
		var n int
		require.NoError(t, tx.QueryRowContext(t.Context(), "SELECT count(*) FROM ("+pgvector.Statement(r, 768)+") x", args...).Scan(&n))
		return n
	}
	assert.Less(t, count("off"), knowledge.DefaultSearchK, "without an iterative scan the other project's closer chunks fill the candidates")
	assert.Equal(t, knowledge.DefaultSearchK, count("relaxed_order"))
	require.NoError(t, tx.Rollback())
	require.NoError(t, conn.Close())

	hits, _, err := r.Search(ctx, knowledge.Query{ProjectID: small, Sources: []uuid.UUID{src}, Text: "q"})
	require.NoError(t, err)
	require.Len(t, hits, knowledge.DefaultSearchK, "the driver turns the iterative scan on")
	for _, h := range hits {
		assert.Equal(t, src, h.Source)
		assert.InDelta(t, 0.8, h.Score, 0.01)
	}
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func explain(t *testing.T, q querier, stmt string, args ...any) string {
	t.Helper()
	rows, err := q.QueryContext(t.Context(), "EXPLAIN "+stmt, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan = append(plan, line)
	}
	require.NoError(t, rows.Err())
	return strings.Join(plan, "\n")
}

func TestRetriever_LiteralDimsUsesIndex(t *testing.T) {
	b := newBackend(t, 768, knowledgetest.NewFake(768).Embedder, nil)
	ctx, _ := b.Project(t)
	tc, err := tenant.From(ctx)
	require.NoError(t, err)
	r := pgvector.New(b.pool, b.schema, "", b.emb)
	conn, err := b.pool.R.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	tx, err := conn.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	for _, stmt := range []string{"SET LOCAL plan_cache_mode = force_generic_plan", "SET LOCAL enable_seqscan = off"} {
		_, err := tx.ExecContext(t.Context(), stmt)
		require.NoError(t, err)
	}
	_, err = tx.ExecContext(t.Context(), "SELECT set_config('app.current_org_id', $1, true)", tc.OrgID.String())
	require.NoError(t, err)
	for _, dims := range knowledge.SupportedDims {
		t.Run(strconv.Itoa(dims), func(t *testing.T) {
			name := fmt.Sprintf("search_%d_%d", dims, time.Now().UnixNano())
			_, err := tx.ExecContext(t.Context(), "PREPARE "+name+" AS "+pgvector.Statement(r, dims))
			require.NoError(t, err)
			zero := "[" + strings.TrimSuffix(strings.Repeat("0.1,", dims), ",") + "]"
			plan := strings.Join(rowsOf(t, tx, fmt.Sprintf("EXPLAIN EXECUTE %s('%s', '%s', '{%s}', 'mk', 5)", name, zero, uuid.New(), uuid.New())), "\n")
			assert.Contains(t, plan, fmt.Sprintf("knowledge_chunks_hnsw_%d", dims), "a generic plan uses the partial HNSW index")
			assert.Contains(t, plan, "current_org_id()", "under the service role's row-level security")
		})
	}
}

func rowsOf(t *testing.T, q querier, query string) []string {
	t.Helper()
	rows, err := q.QueryContext(t.Context(), query)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		out = append(out, line)
	}
	require.NoError(t, rows.Err())
	return out
}

func TestCheckVersion(t *testing.T) {
	b := newBackend(t, 768, knowledgetest.NewFake(768).Embedder, nil)
	require.NoError(t, pgvector.CheckVersion(t.Context(), b.admin), "the test image runs pgvector 0.8")
	require.NoError(t, pgvector.CheckVersion(t.Context(), b.pool.R), "the service role can read the version too")
}
