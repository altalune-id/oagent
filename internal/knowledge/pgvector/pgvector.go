// Package pgvector is the knowledge Retriever over the project's chunks in Postgres, one HNSW index per vector size.
package pgvector

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge"
)

const halfvecDims = 3072

// Embedders gives a project's embedding settings and an embedder for them.
type Embedders interface {
	Settings(ctx context.Context, projectID uuid.UUID) (*knowledge.Settings, error)
	EmbedderFor(ctx context.Context, st *knowledge.Settings) (knowledge.Embedder, error)
}

// Retriever searches knowledge chunks of the project's current model key.
type Retriever struct {
	pool       db.Pool
	embedders  Embedders
	statements map[int]string
}

var _ knowledge.Retriever = (*Retriever)(nil)

// New returns a Retriever on pool for the knowledge tables in schema with tablePrefix.
func New(pool db.Pool, schema, tablePrefix string, e Embedders) *Retriever {
	if schema == "" {
		schema = "public"
	}
	statements := make(map[int]string, len(knowledge.SupportedDims))
	for _, dims := range knowledge.SupportedDims {
		statements[dims] = statement(schema+"."+tablePrefix, dims)
	}
	return &Retriever{pool: pool, embedders: e, statements: statements}
}

// NOTE: hand-built: jet has no per-size vector cast or OPERATOR(public.<=>), and a literal dims must match a partial HNSW index.
func statement(prefix string, dims int) string {
	typ := "public.vector"
	if dims == halfvecDims {
		typ = "public.halfvec"
	}
	cast := typ + "(" + strconv.Itoa(dims) + ")"
	distance := "c.embedding::" + cast + " OPERATOR(public.<=>) $1::" + cast
	nearest := "SELECT c.id, c.org_id, c.document_id, c.text, c.heading_path, c.page, c.url, c.source_id, 1 - (" + distance + ") AS score" +
		" FROM " + prefix + "knowledge_chunks c" +
		" WHERE c.dims = " + strconv.Itoa(dims) + " AND c.project_id = $2 AND c.source_id = ANY($3::uuid[]) AND c.model_key = $4" +
		" ORDER BY " + distance + " LIMIT $5"
	return "SELECT n.id, n.text, d.title, n.heading_path, COALESCE(n.page, 0), COALESCE(n.url, d.url, ''), n.source_id, n.score" +
		" FROM (" + nearest + ") n JOIN " + prefix + "knowledge_documents d ON d.org_id = n.org_id AND d.id = n.document_id"
}

// Search embeds q.Text with the project's model and returns the closest chunks of q's sources.
func (r *Retriever) Search(ctx context.Context, q knowledge.Query) ([]knowledge.Hit, knowledge.Usage, error) {
	// NOTE: it refuses to run inside a unit of work, where its transaction-local settings would leak.
	// NOTE: a read replica may return nothing for a moment after a model switch, until it has the new model's chunks; accepted.
	tc := tenant.Peek(ctx)
	if len(q.Sources) == 0 || strings.TrimSpace(q.Text) == "" || tc.OrgID == uuid.Nil {
		return nil, knowledge.Usage{}, nil
	}
	if tc.ProjectID != uuid.Nil && tc.ProjectID != q.ProjectID {
		return nil, knowledge.Usage{}, &ProjectMismatchError{}
	}
	st, err := r.embedders.Settings(ctx, q.ProjectID)
	if err != nil {
		return nil, knowledge.Usage{}, err
	}
	stmt, ok := r.statements[st.Dims]
	if !ok {
		return nil, knowledge.Usage{}, &knowledge.EmbeddingModelInvalidError{Reason: knowledge.EmbeddingSize, Dims: st.Dims}
	}
	e, err := r.embedders.EmbedderFor(ctx, st)
	if err != nil {
		return nil, knowledge.Usage{}, err
	}
	inputs := q.Inputs()
	res, err := e.Embed(ctx, inputs)
	if err != nil {
		return nil, knowledge.Usage{EmbeddingTokens: res.Tokens}, knowledge.EmbedError(err)
	}
	usage := knowledge.Usage{EmbeddingTokens: res.Tokens}
	if len(res.Vectors) != len(inputs) {
		return nil, usage, &knowledge.EmbeddingFailedError{Reason: knowledge.EmbeddingWrongDims, Cause: errors.New("one query vector per input")}
	}
	vectors := make([]string, len(res.Vectors))
	for i, v := range res.Vectors {
		if len(v) != st.Dims {
			cause := errors.New("a query vector is not the project's size")
			return nil, usage, &knowledge.EmbeddingFailedError{Reason: knowledge.EmbeddingWrongDims, Cause: cause}
		}
		if !slices.ContainsFunc(v, func(x float32) bool { return x != 0 }) {
			return nil, usage, &knowledge.EmbeddingFailedError{Reason: knowledge.EmbeddingRejected, Cause: errors.New("a query vector is all zero")}
		}
		vectors[i] = knowledge.VectorText(v)
	}
	usage.RetrieveCalls = 1
	hits, err := r.query(ctx, stmt, vectors, q, st.ModelKey())
	if err != nil {
		return nil, usage, err
	}
	return keep(hits, knowledge.MinScoreOf(q), knowledge.SearchK(q.K)), usage, nil
}

type ranked struct {
	id  uuid.UUID
	hit knowledge.Hit
}

func keep(hits []ranked, minScore float64, k int) []knowledge.Hit {
	hits = slices.DeleteFunc(hits, func(r ranked) bool { return !(r.hit.Score >= minScore) })
	slices.SortFunc(hits, func(a, b ranked) int {
		return cmp.Or(cmp.Compare(b.hit.Score, a.hit.Score), bytes.Compare(a.id[:], b.id[:]))
	})
	out := make([]knowledge.Hit, 0, min(len(hits), k))
	for _, r := range hits[:min(len(hits), k)] {
		out = append(out, r.hit)
	}
	return out
}

// NOTE: an iterative scan stops after max_scan_tuples index tuples, so a project with fewer matches can still come back short.
//
//nolint:gochecknoglobals // a fixed list.
var tune = []db.Setting{{Name: "hnsw.iterative_scan", Value: "relaxed_order"}, {Name: "hnsw.max_scan_tuples", Value: "100000"}}

func (r *Retriever) query(ctx context.Context, stmt string, vectors []string, q knowledge.Query, modelKey string) ([]ranked, error) {
	sources := make([]string, len(q.Sources))
	for i, s := range q.Sources {
		sources[i] = s.String()
	}
	list, k := "{"+strings.Join(sources, ",")+"}", knowledge.SearchK(q.K)
	best := map[uuid.UUID]knowledge.Hit{}
	err := db.ReadTuned(ctx, r.pool, tune, func(tx qrm.DB) error {
		for _, vector := range vectors {
			if err := scanNearest(ctx, tx, stmt, best, vector, q.ProjectID, list, modelKey, k); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("pgvector: search: %w", err)
	}
	hits := make([]ranked, 0, len(best))
	for id, h := range best {
		hits = append(hits, ranked{id: id, hit: h})
	}
	return hits, nil
}

func scanNearest(ctx context.Context, tx qrm.DB, stmt string, best map[uuid.UUID]knowledge.Hit, args ...any) error {
	rows, err := tx.QueryContext(ctx, stmt, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			id uuid.UUID
			h  knowledge.Hit
		)
		if err := rows.Scan(&id, &h.Text, &h.Title, &h.Heading, &h.Page, &h.URL, &h.Source, &h.Score); err != nil {
			return err
		}
		if prev, ok := best[id]; !ok || h.Score > prev.Score {
			best[id] = h
		}
	}
	return rows.Err()
}

// CheckVersion fails when the database's pgvector extension is missing or older than 0.8, which iterative index scans need.
func CheckVersion(ctx context.Context, conn *sql.DB) error {
	var v string
	if err := conn.QueryRowContext(ctx, "SELECT extversion FROM pg_extension WHERE extname = 'vector'").Scan(&v); err != nil {
		return fmt.Errorf("pgvector: read the vector extension version: %w", err)
	}
	if !versionSupported(v) {
		return fmt.Errorf("pgvector: the vector extension is %s; knowledge needs 0.8 or newer", v)
	}
	return nil
}

func versionSupported(v string) bool {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return major > 0 || minor >= 8
}

// ProjectMismatchError refuses a query whose project is not the one on its context.
type ProjectMismatchError struct{}

func (*ProjectMismatchError) Error() string {
	return "pgvector: the query's project is not the context's project"
}

// IsProjectMismatchError reports whether err's tree contains a *ProjectMismatchError.
func IsProjectMismatchError(err error) bool {
	_, ok := errors.AsType[*ProjectMismatchError](err)
	return ok
}
