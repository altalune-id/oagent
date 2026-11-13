//go:build integration

package schema_test

import (
	"database/sql"
	"strconv"
	"strings"
	"testing"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/testutil/pgtest"
)

type knowledgeScope struct {
	twoOrgs
	sourceA, sourceB, documentA uuid.UUID
}

func seedKnowledge(t *testing.T, db *sql.DB) knowledgeScope {
	t.Helper()
	s := knowledgeScope{twoOrgs: seedTwoOrgs(t, db), sourceA: uuid.New(), sourceB: uuid.New(), documentA: uuid.New()}
	for src, scope := range map[uuid.UUID][2]uuid.UUID{s.sourceA: {s.orgA, s.prjA}, s.sourceB: {s.orgB, s.prjB}} {
		require.NoError(t, insertSource(t, db, src, scope[0], scope[1], "upload", nil))
	}
	_, err := db.ExecContext(t.Context(), `INSERT INTO knowledge_documents (id, public_id, org_id, project_id, source_id, title, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'Doc', 'pending', now(), now())`, s.documentA, "d"+s.documentA.String()[:12], s.orgA, s.prjA, s.sourceA)
	require.NoError(t, err)
	return s
}

func insertSource(t *testing.T, db *sql.DB, id, org, prj uuid.UUID, kind string, seed any) error {
	t.Helper()
	_, err := db.ExecContext(t.Context(), `INSERT INTO knowledge_sources (id, public_id, org_id, project_id, kind, backend, name, seed_url, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'pgvector', $2, $6, 'idle', now(), now())`, id, "s"+id.String()[:12], org, prj, kind, seed)
	return err
}

func vectorLiteral(dims int) string {
	return "[" + strings.TrimSuffix(strings.Repeat("0.1,", dims), ",") + "]"
}

func insertChunk(t *testing.T, db *sql.DB, org, prj, source, document uuid.UUID, ordinal, dims int) error {
	t.Helper()
	_, err := db.ExecContext(t.Context(), `INSERT INTO knowledge_chunks (id, org_id, project_id, source_id, document_id, ordinal, text, embedding, dims, model_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, 't', $7::public.vector, $8, 'm', now())`, uuid.New(), org, prj, source, document, ordinal, vectorLiteral(dims), dims)
	return err
}

func TestKnowledgeSources_KindColumnsTied(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedTwoOrgs(t, db)
	insert := func(kind, backend string, seed any) error {
		id := uuid.New()
		_, err := db.ExecContext(t.Context(), `INSERT INTO knowledge_sources (id, public_id, org_id, project_id, kind, backend, name, seed_url, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $2, $7, 'idle', now(), now())`, id, "s"+id.String()[:12], s.orgA, s.prjA, kind, backend, seed)
		return err
	}
	require.NoError(t, insert("url", "pgvector", "https://example.com"))
	require.NoError(t, insert("upload", "pgvector", nil))
	assert.Equal(t, "knowledge_sources_seed_url_shape", pgConstraint(insert("url", "pgvector", nil)), "a URL source needs its seed")
	assert.Equal(t, "knowledge_sources_seed_url_shape", pgConstraint(insert("upload", "pgvector", "https://example.com")), "an upload source has none")
	assert.Equal(t, "knowledge_sources_kind_check", pgConstraint(insert("bedrock_kb", "pgvector", nil)))
	assert.Equal(t, "knowledge_sources_backend_check", pgConstraint(insert("upload", "bedrock", nil)))
}

func TestKnowledgeSources_NameUniquePerProject(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedTwoOrgs(t, db)
	insert := func(org, prj uuid.UUID) error {
		id := uuid.New()
		_, err := db.ExecContext(t.Context(), `INSERT INTO knowledge_sources (id, public_id, org_id, project_id, kind, backend, name, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'upload', 'pgvector', 'Handbook', 'idle', now(), now())`, id, "s"+id.String()[:12], org, prj)
		return err
	}
	require.NoError(t, insert(s.orgA, s.prjA))
	require.NoError(t, insert(s.orgB, s.prjB), "another project may use the name")
	assert.Equal(t, "knowledge_sources_project_name_key", pgConstraint(insert(s.orgA, s.prjA)))
}

func TestChunks_SourceInSameProject(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedKnowledge(t, db)
	require.NoError(t, insertChunk(t, db, s.orgA, s.prjA, s.sourceA, s.documentA, 0, 768))
	err := insertChunk(t, db, s.orgA, s.prjA, s.sourceB, s.documentA, 1, 768)
	assert.Equal(t, "23503", pgCode(err), "a chunk never sits under another project's source: %v", err)

	otherSource := uuid.New()
	require.NoError(t, insertSource(t, db, otherSource, s.orgA, s.prjA, "upload", nil))
	err = insertChunk(t, db, s.orgA, s.prjA, otherSource, s.documentA, 2, 768)
	assert.Equal(t, "knowledge_chunks_document_fkey", pgConstraint(err), "nor under a source its document does not belong to")
}

func TestChunks_DimsMatchTheVector(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedKnowledge(t, db)
	_, err := db.ExecContext(t.Context(), `INSERT INTO knowledge_chunks (id, org_id, project_id, source_id, document_id, ordinal, text, embedding, dims, model_key, created_at)
		VALUES ($1, $2, $3, $4, $5, 0, 't', $6::public.vector, 1536, 'm', now())`, uuid.New(), s.orgA, s.prjA, s.sourceA, s.documentA, vectorLiteral(768))
	assert.Equal(t, "knowledge_chunks_embedding_dims", pgConstraint(err))
	assert.Equal(t, "knowledge_chunks_dims_check", pgConstraint(insertChunk(t, db, s.orgA, s.prjA, s.sourceA, s.documentA, 1, 384)))
}

func TestChunks_CascadeWithTheirDocument(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedKnowledge(t, db)
	require.NoError(t, insertChunk(t, db, s.orgA, s.prjA, s.sourceA, s.documentA, 0, 768))
	_, err := db.ExecContext(t.Context(), `DELETE FROM knowledge_documents WHERE id = $1`, s.documentA)
	require.NoError(t, err)
	var n int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM knowledge_chunks WHERE document_id = $1`, s.documentA).Scan(&n))
	assert.Zero(t, n)
}

func TestChunks_HNSWIndexUsed(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedKnowledge(t, db)
	for i, dims := range []int{1536, 3072, 768} {
		require.NoError(t, insertChunk(t, db, s.orgA, s.prjA, s.sourceA, s.documentA, i, dims))
	}
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(t.Context(), `SET LOCAL enable_seqscan = off`)
	require.NoError(t, err)

	for _, c := range []struct {
		dims        int
		cast, index string
	}{
		{1536, "public.vector(1536)", "knowledge_chunks_hnsw_1536"},
		{3072, "public.halfvec(3072)", "knowledge_chunks_hnsw_3072"},
	} {
		rows, err := tx.QueryContext(t.Context(), `EXPLAIN SELECT id FROM knowledge_chunks WHERE dims = `+strconv.Itoa(c.dims)+`
			ORDER BY embedding::`+c.cast+` OPERATOR(public.<=>) $1::`+c.cast+` LIMIT 5`, vectorLiteral(c.dims))
		require.NoError(t, err)
		var plan strings.Builder
		for rows.Next() {
			var line string
			require.NoError(t, rows.Scan(&line))
			plan.WriteString(line + "\n")
		}
		require.NoError(t, rows.Err())
		_ = rows.Close()
		assert.Contains(t, plan.String(), c.index, "dims %d uses its partial HNSW index:\n%s", c.dims, plan.String())
	}
}

func TestKnowledgeSettings_ProviderInSameProject(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedTwoOrgs(t, db)
	provider := func(org, prj uuid.UUID) uuid.UUID {
		id := uuid.New()
		_, err := db.ExecContext(t.Context(), `INSERT INTO model_providers (id, public_id, org_id, project_id, name, kind, base_url, api_key_sealed, models, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $2, 'openai_compatible', 'https://e.com/v1', '\x01', '[]', now(), now())`, id, "r"+id.String()[:12], org, prj)
		require.NoError(t, err)
		return id
	}
	own, foreign := provider(s.orgA, s.prjA), provider(s.orgB, s.prjB)
	set := func(p uuid.UUID, dims int) error {
		_, err := db.ExecContext(t.Context(), `INSERT INTO knowledge_settings (project_id, org_id, provider_id, model, dims, updated_at)
			VALUES ($1, $2, $3, 'e', $4, now()) ON CONFLICT (project_id) DO UPDATE SET provider_id = EXCLUDED.provider_id, dims = EXCLUDED.dims`, s.prjA, s.orgA, p, dims)
		return err
	}
	assert.Equal(t, "knowledge_settings_provider_fkey", pgConstraint(set(foreign, 1536)), "another project's provider is refused")
	assert.Equal(t, "knowledge_settings_dims_check", pgConstraint(set(own, 384)))
	require.NoError(t, set(own, 1536))
	_, err := db.ExecContext(t.Context(), `DELETE FROM model_providers WHERE id = $1`, own)
	assert.Equal(t, "knowledge_settings_provider_fkey", pgConstraint(err), "the embedding provider can't vanish under the setting")
}

func TestKnowledgeDocuments_FileGoneLeavesTheDocument(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedKnowledge(t, db)
	file := uuid.New()
	_, err := db.ExecContext(t.Context(), `INSERT INTO files (id, public_id, org_id, project_id, purpose, visibility, object_key, content_type, size_bytes, status, created_at, expires_at)
		VALUES ($1, $2, $3, $4, 'knowledge', 'private', $2, 'application/pdf', 10, 'pending', now(), now())`, file, "f"+file.String()[:12], s.orgA, s.prjA)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `UPDATE knowledge_documents SET file_id = $2, status = 'uploading' WHERE id = $1`, s.documentA, file)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `UPDATE knowledge_documents SET url = 'https://e.com' WHERE id = $1`, s.documentA)
	assert.Equal(t, "knowledge_documents_origin_shape", pgConstraint(err), "a file or a URL, never both")

	_, err = db.ExecContext(t.Context(), `DELETE FROM files WHERE id = $1`, file)
	require.NoError(t, err)
	var fileID uuid.NullUUID
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT file_id FROM knowledge_documents WHERE id = $1`, s.documentA).Scan(&fileID))
	assert.False(t, fileID.Valid, "the sweep removing a file clears the link only")
}

func TestChunks_VectorResolvesForTheServiceRole(t *testing.T) {
	f := newRLSFixture(t)
	scopeA := tenant.Into(t.Context(), tenant.Context{OrgID: f.a.org, ProjectID: f.a.project})
	var dims int
	require.NoError(t, db.Read(scopeA, f.svc, func(q qrm.DB) error {
		rows, err := q.QueryContext(scopeA, `SELECT public.vector_dims(embedding) FROM `+f.schema+`.knowledge_chunks LIMIT 1`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			if err := rows.Scan(&dims); err != nil {
				return err
			}
		}
		return rows.Err()
	}))
	assert.Equal(t, 768, dims)
	assert.Equal(t, int64(1), f.affected(t, scopeA, `UPDATE `+f.schema+`.knowledge_chunks SET embedding = $1::public.vector WHERE document_id = $2`,
		vectorLiteral(768), f.a.document))
}
