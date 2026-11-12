package pgvector_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/knowledge/embed"
	"altalune.id/oagent/internal/knowledge/knowledgetest"
	"altalune.id/oagent/internal/knowledge/pgvector"
)

type stubEmbedders struct {
	dims        int
	settingsErr error
	embedderErr error
	emb         knowledge.Embedder
}

func (s stubEmbedders) Settings(_ context.Context, project uuid.UUID) (*knowledge.Settings, error) {
	return &knowledge.Settings{ProjectID: project, Model: "m", Dims: s.dims}, s.settingsErr
}

func (s stubEmbedders) EmbedderFor(context.Context, *knowledge.Settings) (knowledge.Embedder, error) {
	return s.emb, s.embedderErr
}

type failing struct {
	res embed.Result
	err error
}

func (f failing) Probe(context.Context) (int, error) { return 0, nil }

func (f failing) Embed(context.Context, []string) (embed.Result, error) {
	if f.err != nil {
		return f.res, f.err
	}
	if f.res.Vectors == nil {
		return embed.Result{}, &embed.Error{Reason: embed.Unreachable}
	}
	return f.res, nil
}

func TestSearch_FailsBeforeTouchingTheDatabase(t *testing.T) {
	boom := errors.New("boom")
	q := knowledge.Query{ProjectID: uuid.New(), Sources: []uuid.UUID{uuid.New()}, Text: "refunds"}
	cases := map[string]struct {
		e     stubEmbedders
		check func(t *testing.T, err error, usage knowledge.Usage)
	}{
		"no settings": {stubEmbedders{dims: 768, settingsErr: boom}, func(t *testing.T, err error, _ knowledge.Usage) {
			require.ErrorIs(t, err, boom)
		}},
		"a size with no index": {stubEmbedders{dims: 999}, func(t *testing.T, err error, _ knowledge.Usage) {
			require.True(t, knowledge.IsEmbeddingModelInvalidError(err), "%v", err)
		}},
		"no embedder": {stubEmbedders{dims: 768, embedderErr: boom}, func(t *testing.T, err error, _ knowledge.Usage) {
			require.ErrorIs(t, err, boom)
		}},
		"the embeddings call fails": {stubEmbedders{dims: 768, emb: failing{}}, func(t *testing.T, err error, usage knowledge.Usage) {
			e, ok := errors.AsType[*knowledge.EmbeddingFailedError](err)
			require.True(t, ok, "%v", err)
			assert.Equal(t, knowledge.EmbeddingUnreachable, e.Reason, "mapped like ingest maps it")
			assert.Equal(t, knowledge.Usage{}, usage)
		}},
		"a vector of another size": {stubEmbedders{dims: 768, emb: failing{res: embed.Result{Vectors: [][]float32{{1, 2}}, Tokens: 3}}}, func(t *testing.T, err error, usage knowledge.Usage) {
			e, ok := errors.AsType[*knowledge.EmbeddingFailedError](err)
			require.True(t, ok, "%v", err)
			assert.Equal(t, knowledge.EmbeddingWrongDims, e.Reason)
			assert.Equal(t, knowledge.Usage{EmbeddingTokens: 3}, usage, "the spent tokens are still reported")
		}},
		"a failure after the provider billed": {stubEmbedders{dims: 768, emb: failing{res: embed.Result{Tokens: 5}, err: &embed.Error{Reason: embed.WrongDims}}}, func(t *testing.T, err error, usage knowledge.Usage) {
			require.Error(t, err)
			assert.Equal(t, knowledge.Usage{EmbeddingTokens: 5}, usage, "billed tokens come back with the error")
		}},
		"an all-zero query vector": {stubEmbedders{dims: 768, emb: failing{res: embed.Result{Vectors: [][]float32{make([]float32, 768)}, Tokens: 3}}}, func(t *testing.T, err error, _ knowledge.Usage) {
			e, ok := errors.AsType[*knowledge.EmbeddingFailedError](err)
			require.True(t, ok, "%v", err)
			assert.Equal(t, knowledge.EmbeddingRejected, e.Reason, "a zero vector has no direction: every cosine is NaN")
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := tenant.Into(t.Context(), tenant.Context{OrgID: uuid.New()})
			hits, usage, err := pgvector.New(db.Pool{}, "", "", c.e).Search(ctx, q)
			assert.Nil(t, hits)
			c.check(t, err, usage)
		})
	}
}

func TestSearch_NothingToSearchSkipsEverything(t *testing.T) {
	e := stubEmbedders{dims: 768, settingsErr: errors.New("never asked"), emb: &knowledgetest.Embedder{Dims: 768}}
	r := pgvector.New(db.Pool{}, "", "", e)
	for _, q := range []knowledge.Query{{ProjectID: uuid.New(), Text: "x"}, {ProjectID: uuid.New(), Sources: []uuid.UUID{uuid.New()}, Text: "  "}} {
		hits, usage, err := r.Search(t.Context(), q)
		require.NoError(t, err)
		assert.Nil(t, hits)
		assert.Equal(t, knowledge.Usage{}, usage)
	}
}

func TestStatement_QualifiesTypesAndKeepsTheSizeLiteral(t *testing.T) {
	r := pgvector.New(db.Pool{}, "", "oa_", stubEmbedders{})
	for _, dims := range knowledge.SupportedDims {
		stmt := pgvector.Statement(r, dims)
		assert.Contains(t, stmt, "public.oa_knowledge_chunks c")
		assert.Contains(t, stmt, "WHERE c.dims = "+strconv.Itoa(dims)+" AND")
		assert.Contains(t, stmt, "OPERATOR(public.<=>)")
	}
	assert.Contains(t, pgvector.Statement(r, 3072), "public.halfvec(3072)")
	assert.Contains(t, pgvector.Statement(r, 1536), "public.vector(1536)")
}

var _ pgvector.Embedders = (*knowledge.Service)(nil)

func TestSearch_RefusesAnotherProjectThanTheContexts(t *testing.T) {
	emb := &knowledgetest.Embedder{Dims: 768}
	r := pgvector.New(db.Pool{}, "", "", stubEmbedders{dims: 768, emb: emb})
	ctx := tenant.Into(t.Context(), tenant.Context{OrgID: uuid.New(), ProjectID: uuid.New()})
	hits, _, err := r.Search(ctx, knowledge.Query{ProjectID: uuid.New(), Sources: []uuid.UUID{uuid.New()}, Text: "refunds"})
	require.Error(t, err)
	assert.True(t, pgvector.IsProjectMismatchError(fmt.Errorf("wrapped: %w", err)), "%v", err)
	assert.False(t, pgvector.IsProjectMismatchError(errors.New("other")))
	assert.Nil(t, hits)
	assert.Zero(t, emb.Calls.Load(), "refused before any embedding is paid for")
}

func TestKeep_DropsLowAndNaNScores(t *testing.T) {
	hits := []knowledge.Hit{{Text: "low", Score: 0.1}, {Text: "nan", Score: math.NaN()}, {Text: "b", Score: 0.5}, {Text: "a", Score: 0.9}, {Text: "edge", Score: knowledge.MinSearchScore}}
	got := pgvector.Keep(hits)
	texts := make([]string, len(got))
	for i, h := range got {
		texts[i] = h.Text
	}
	assert.Equal(t, []string{"a", "b", "edge"}, texts, "best first; NaN and low scores dropped")
}

func TestVersionAtLeast(t *testing.T) {
	for v, ok := range map[string]bool{"0.8.0": true, "0.8.7": true, "0.10.1": true, "1.0": true, "0.7.4": false, "0.5": false, "": false, "x.y": false} {
		assert.Equal(t, ok, pgvector.VersionSupported(v), "%q", v)
	}
}
