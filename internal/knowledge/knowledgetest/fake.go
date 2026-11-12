package knowledgetest

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge"
)

const currentModelKey = "current"

type stored struct {
	Passage
	org      uuid.UUID
	modelKey string
	dims     int
	vector   []float32
}

// Fake is an in-memory Retriever and its Backend, scoped by the tenant org on ctx like the real store.
type Fake struct {
	Embedder *Embedder
	mu       sync.Mutex
	orgs     map[uuid.UUID]uuid.UUID
	passages []stored
}

// NewFake returns an empty Fake that embeds with a dims-sized Embedder.
func NewFake(dims int) *Fake {
	return &Fake{Embedder: &Embedder{Dims: dims}, orgs: map[uuid.UUID]uuid.UUID{}}
}

// Retriever returns f.
func (f *Fake) Retriever() knowledge.Retriever { return f }

// Project makes a project in a new org.
func (f *Fake) Project(*testing.T) (context.Context, uuid.UUID) {
	org, prj := uuid.New(), uuid.New()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.orgs[prj] = org
	return tenant.Into(context.Background(), tenant.Context{OrgID: org}), prj
}

// ProjectInOrg makes another project in ctx's org.
func (f *Fake) ProjectInOrg(t *testing.T, ctx context.Context) uuid.UUID {
	tc, err := tenant.From(ctx)
	require.NoError(t, err)
	prj := uuid.New()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.orgs[prj] = tc.OrgID
	return prj
}

// Source returns a new source id.
func (f *Fake) Source(*testing.T, context.Context, uuid.UUID) uuid.UUID { return uuid.New() }

// Add stores p, embedded, under ctx's org.
func (f *Fake) Add(t *testing.T, ctx context.Context, p Passage) {
	tc, err := tenant.From(ctx)
	require.NoError(t, err)
	f.mu.Lock()
	defer f.mu.Unlock()
	sp := stored{Passage: p, org: tc.OrgID, modelKey: currentModelKey, dims: f.Embedder.Dims, vector: Vector(f.Embedder.Dims, p.Text)}
	if p.Stale {
		sp.modelKey = "stale"
	}
	if p.OtherSize {
		sp.dims++
	}
	f.passages = append(f.passages, sp)
}

// Search keeps the Retriever contract over the stored passages.
func (f *Fake) Search(ctx context.Context, q knowledge.Query) ([]knowledge.Hit, knowledge.Usage, error) {
	if len(q.Sources) == 0 || strings.TrimSpace(q.Text) == "" {
		return nil, knowledge.Usage{}, nil
	}
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, knowledge.Usage{}, nil
	}
	if tc.ProjectID != uuid.Nil && tc.ProjectID != q.ProjectID {
		return nil, knowledge.Usage{}, errors.New("knowledgetest: the query's project is not the context's project")
	}
	res, err := f.Embedder.Embed(ctx, []string{q.Text})
	if err != nil {
		return nil, knowledge.Usage{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var hits []knowledge.Hit
	for _, p := range f.passages {
		if p.org != tc.OrgID || p.ProjectID != q.ProjectID || !slices.Contains(q.Sources, p.SourceID) ||
			p.modelKey != currentModelKey || p.dims != f.Embedder.Dims {
			continue
		}
		if score := cosine(res.Vectors[0], p.vector); score >= knowledge.MinSearchScore {
			hits = append(hits, knowledge.Hit{
				Text: p.Text, Title: p.Title, Heading: p.Heading, Page: p.Page, URL: p.URL, Score: score, Source: p.SourceID,
			})
		}
	}
	slices.SortStableFunc(hits, func(a, b knowledge.Hit) int { return cmp.Compare(b.Score, a.Score) })
	return hits[:min(len(hits), knowledge.SearchK(q.K))], knowledge.Usage{EmbeddingTokens: res.Tokens, RetrieveCalls: 1}, nil
}

func cosine(a, b []float32) float64 {
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot
}
