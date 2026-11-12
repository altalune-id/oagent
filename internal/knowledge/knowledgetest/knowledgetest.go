// Package knowledgetest is the contract every knowledge.Retriever passes, with a deterministic embedder and an in-memory fake.
package knowledgetest

import (
	"cmp"
	"context"
	"hash/fnv"
	"math"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"unicode"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/knowledge/embed"
)

// Embedder embeds a text as its hashed bag of words, so texts sharing words are close; Calls counts Embed calls.
type Embedder struct {
	Dims  int
	Calls atomic.Int32
}

// Probe returns Dims.
func (e *Embedder) Probe(context.Context) (int, error) { return e.Dims, nil }

// Embed returns one vector per text and counts one token per word.
func (e *Embedder) Embed(_ context.Context, texts []string) (embed.Result, error) {
	e.Calls.Add(1)
	res := embed.Result{Dims: e.Dims}
	for _, t := range texts {
		res.Vectors = append(res.Vectors, Vector(e.Dims, t))
		res.Tokens += int64(len(words(t)))
	}
	return res, nil
}

func words(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// Vector is the unit bag-of-words vector of text in dims dimensions.
func Vector(dims int, text string) []float32 {
	v := make([]float32, dims)
	for _, w := range words(text) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(w))
		v[int(h.Sum32())%dims]++
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
		v[i] = float32(float64(v[i]) / math.Sqrt(norm))
	}
	return v
}

// Passage is one stored chunk under test.
type Passage struct {
	ProjectID uuid.UUID
	SourceID  uuid.UUID
	Title     string
	Heading   string
	Page      int
	URL       string
	Text      string
	// Stale stores the passage under an older model key; OtherSize at another vector size. Neither may ever be returned.
	Stale     bool
	OtherSize bool
}

// Backend is the store behind the Retriever under test.
type Backend interface {
	Retriever() knowledge.Retriever
	Project(t *testing.T) (context.Context, uuid.UUID)
	ProjectInOrg(t *testing.T, ctx context.Context) uuid.UUID
	Source(t *testing.T, ctx context.Context, project uuid.UUID) uuid.UUID
	Add(t *testing.T, ctx context.Context, p Passage)
}

// Run checks the Retriever contract; newBackend returns a fresh, empty backend for each case.
func Run(t *testing.T, newBackend func(t *testing.T) Backend) {
	t.Helper()
	t.Run("ClosestFirstWithTheirSource", func(t *testing.T) { closestFirst(t, newBackend(t)) })
	t.Run("OnlyTheListedSources", func(t *testing.T) { onlyListedSources(t, newBackend(t)) })
	t.Run("NeverAnotherProject", func(t *testing.T) { neverAnotherProject(t, newBackend(t)) })
	t.Run("NothingToSearch", func(t *testing.T) { nothingToSearch(t, newBackend(t)) })
	t.Run("KIsClamped", func(t *testing.T) { kIsClamped(t, newBackend(t)) })
	t.Run("OnlyTheCurrentModelAndSize", func(t *testing.T) { onlyCurrentModel(t, newBackend(t)) })
	t.Run("NoTenantFindsNothing", func(t *testing.T) { noTenant(t, newBackend(t)) })
	t.Run("AnotherProjectOnTheContextRefused", func(t *testing.T) { contextProject(t, newBackend(t)) })
}

func onlyCurrentModel(t *testing.T, b Backend) {
	ctx, prj := b.Project(t)
	src := b.Source(t, ctx, prj)
	b.Add(t, ctx, Passage{ProjectID: prj, SourceID: src, Title: "Current", Text: "Refunds take seven days."})
	b.Add(t, ctx, Passage{ProjectID: prj, SourceID: src, Title: "Stale", Text: "Refunds take seven days.", Stale: true})
	b.Add(t, ctx, Passage{ProjectID: prj, SourceID: src, Title: "Other size", Text: "Refunds take seven days.", OtherSize: true})

	hits, _ := search(ctx, t, b, knowledge.Query{ProjectID: prj, Sources: []uuid.UUID{src}, Text: "refunds take seven days", K: 10})
	require.Len(t, hits, 1, "a chunk of an older model key or another vector size is never returned")
	assert.Equal(t, "Current", hits[0].Title)
}

func noTenant(t *testing.T, b Backend) {
	ctx, prj := b.Project(t)
	src := b.Source(t, ctx, prj)
	b.Add(t, ctx, Passage{ProjectID: prj, SourceID: src, Title: "T", Text: "Refunds take seven days."})
	hits, _, _ := b.Retriever().Search(context.Background(), knowledge.Query{ProjectID: prj, Sources: []uuid.UUID{src}, Text: "refunds"})
	assert.Empty(t, hits, "a context with no tenant finds nothing")
}

func contextProject(t *testing.T, b Backend) {
	ctx, prj := b.Project(t)
	src := b.Source(t, ctx, prj)
	b.Add(t, ctx, Passage{ProjectID: prj, SourceID: src, Title: "T", Text: "Refunds take seven days."})
	tc, err := tenant.From(ctx)
	require.NoError(t, err)
	other := tenant.Into(context.Background(), tenant.Context{OrgID: tc.OrgID, ProjectID: uuid.New()})
	hits, _, err := b.Retriever().Search(other, knowledge.Query{ProjectID: prj, Sources: []uuid.UUID{src}, Text: "refunds"})
	require.Error(t, err, "a query for another project than the context's is refused")
	assert.Empty(t, hits)
}

func search(ctx context.Context, t *testing.T, b Backend, q knowledge.Query) ([]knowledge.Hit, knowledge.Usage) {
	t.Helper()
	hits, usage, err := b.Retriever().Search(ctx, q)
	require.NoError(t, err)
	return hits, usage
}

func closestFirst(t *testing.T, b Backend) {
	ctx, prj := b.Project(t)
	src := b.Source(t, ctx, prj)
	policy := Passage{
		ProjectID: prj, SourceID: src, Title: "Refund policy", Heading: "Timing", Page: 2, Text: "Refunds take seven days for card payments.",
	}
	faq := Passage{ProjectID: prj, SourceID: src, Title: "Refund FAQ", URL: "https://help.example.com/refunds", Text: "Refunds take days."}
	b.Add(t, ctx, policy)
	b.Add(t, ctx, faq)
	b.Add(t, ctx, Passage{ProjectID: prj, SourceID: src, Title: "Office", Text: "The office cat is called Miso."})

	q := knowledge.Query{ProjectID: prj, Sources: []uuid.UUID{src}, Text: "how many days do refunds take", K: 5}
	hits, usage := search(ctx, t, b, q)
	require.Len(t, hits, 2, "the unrelated passage scores under the threshold")
	assert.Equal(t, knowledge.Hit{Text: faq.Text, Title: faq.Title, URL: faq.URL, Score: hits[0].Score, Source: src}, hits[0])
	want := knowledge.Hit{Text: policy.Text, Title: policy.Title, Heading: policy.Heading, Page: 2, Score: hits[1].Score, Source: src}
	assert.Equal(t, want, hits[1])
	assert.Greater(t, hits[0].Score, hits[1].Score)
	assert.GreaterOrEqual(t, hits[1].Score, knowledge.MinSearchScore)
	assert.Equal(t, 1, usage.RetrieveCalls)
	assert.Equal(t, int64(6), usage.EmbeddingTokens, "the query's tokens")
}

func onlyListedSources(t *testing.T, b Backend) {
	ctx, prj := b.Project(t)
	bound, unbound := b.Source(t, ctx, prj), b.Source(t, ctx, prj)
	b.Add(t, ctx, Passage{ProjectID: prj, SourceID: bound, Title: "Bound", Text: "Refunds take seven days."})
	b.Add(t, ctx, Passage{ProjectID: prj, SourceID: unbound, Title: "Unbound", Text: "Refunds take seven days."})

	hits, _ := search(ctx, t, b, knowledge.Query{ProjectID: prj, Sources: []uuid.UUID{bound}, Text: "refunds take seven days"})
	require.Len(t, hits, 1)
	assert.Equal(t, bound, hits[0].Source)
}

func neverAnotherProject(t *testing.T, b Backend) {
	ctx, prj := b.Project(t)
	src := b.Source(t, ctx, prj)
	sibling := b.ProjectInOrg(t, ctx)
	siblingSrc := b.Source(t, ctx, sibling)
	otherCtx, otherPrj := b.Project(t)
	otherSrc := b.Source(t, otherCtx, otherPrj)
	b.Add(t, ctx, Passage{ProjectID: prj, SourceID: src, Title: "Ours", Text: "Refunds take seven days."})
	b.Add(t, ctx, Passage{ProjectID: sibling, SourceID: siblingSrc, Title: "Sibling", Text: "Refunds take seven days."})
	b.Add(t, otherCtx, Passage{ProjectID: otherPrj, SourceID: otherSrc, Title: "Other org", Text: "Refunds take seven days."})

	all := []uuid.UUID{src, siblingSrc, otherSrc}
	hits, _ := search(ctx, t, b, knowledge.Query{ProjectID: prj, Sources: all, Text: "refunds take seven days"})
	require.Len(t, hits, 1, "another project's source never matches, even when listed")
	assert.Equal(t, src, hits[0].Source)

	hits, _ = search(ctx, t, b, knowledge.Query{ProjectID: otherPrj, Sources: all, Text: "refunds take seven days"})
	assert.Empty(t, hits, "another org's project is not searchable from this org")
}

func nothingToSearch(t *testing.T, b Backend) {
	ctx, prj := b.Project(t)
	src := b.Source(t, ctx, prj)
	b.Add(t, ctx, Passage{ProjectID: prj, SourceID: src, Title: "T", Text: "Refunds take seven days."})
	for _, q := range []knowledge.Query{
		{ProjectID: prj, Text: "refunds"},
		{ProjectID: prj, Sources: []uuid.UUID{src}, Text: " \n"},
	} {
		hits, usage := search(ctx, t, b, q)
		assert.Empty(t, hits)
		assert.Equal(t, knowledge.Usage{}, usage, "no sources or no text: no embedding, no search")
	}
}

func kIsClamped(t *testing.T, b Backend) {
	ctx, prj := b.Project(t)
	src := b.Source(t, ctx, prj)
	for i := range knowledge.MaxSearchK + 3 {
		b.Add(t, ctx, Passage{ProjectID: prj, SourceID: src, Title: "T", Text: strings.Repeat("refund ", i+1) + "policy"})
	}
	q := knowledge.Query{ProjectID: prj, Sources: []uuid.UUID{src}, Text: "refund policy"}
	for k, want := range map[int]int{3: 3, 0: knowledge.DefaultSearchK, -1: knowledge.DefaultSearchK, 100: knowledge.MaxSearchK} {
		q.K = k
		hits, _ := search(ctx, t, b, q)
		assert.Len(t, hits, want, "k=%d", k)
		assert.True(t, slices.IsSortedFunc(hits, func(a, b knowledge.Hit) int { return cmp.Compare(b.Score, a.Score) }), "best first")
	}
}
