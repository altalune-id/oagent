-- +goose Up
-- +goose StatementBegin

-- NOTE: Down keeps the extension; other schemas on the server may use it.
CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;

ALTER TABLE {{.Schema}}.{{.TablePrefix}}model_providers
  ADD CONSTRAINT {{.TablePrefix}}model_providers_project_id_key UNIQUE (project_id, id);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs
  ADD COLUMN embedding_tokens BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN embedding_price_input_micros BIGINT,
  ADD COLUMN knowledge_retrieve_calls INT NOT NULL DEFAULT 0,
  ADD COLUMN knowledge_retrieve_ms BIGINT NOT NULL DEFAULT 0;

CREATE TABLE {{.Schema}}.{{.TablePrefix}}files (
  id           UUID PRIMARY KEY,
  public_id    TEXT NOT NULL UNIQUE,
  org_id       UUID NOT NULL,
  project_id   UUID NOT NULL,
  purpose      TEXT NOT NULL,
  visibility   TEXT NOT NULL,
  object_key   TEXT NOT NULL,
  content_type TEXT NOT NULL,
  size_bytes   BIGINT NOT NULL,
  status       TEXT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  expires_at   TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}files_object_key_key UNIQUE (object_key),
  CONSTRAINT {{.TablePrefix}}files_org_project_id_key UNIQUE (org_id, project_id, id),
  CONSTRAINT {{.TablePrefix}}files_purpose_check CHECK (purpose IN ('knowledge')),
  CONSTRAINT {{.TablePrefix}}files_visibility_check CHECK (visibility IN ('public', 'private')),
  CONSTRAINT {{.TablePrefix}}files_status_check CHECK (status IN ('pending', 'ready', 'deleting')),
  CONSTRAINT {{.TablePrefix}}files_size_bytes_check CHECK (size_bytes > 0),
  CONSTRAINT {{.TablePrefix}}files_org_project_fkey FOREIGN KEY (org_id, project_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE
);

CREATE INDEX {{.TablePrefix}}files_status_expires_at_idx ON {{.Schema}}.{{.TablePrefix}}files (status, expires_at);

CREATE TABLE {{.Schema}}.{{.TablePrefix}}knowledge_settings (
  project_id  UUID PRIMARY KEY,
  org_id      UUID NOT NULL,
  provider_id UUID NOT NULL,
  model       TEXT NOT NULL,
  dims        SMALLINT NOT NULL,
  updated_at  TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}knowledge_settings_dims_check CHECK (dims IN (768, 1024, 1536, 3072)),
  CONSTRAINT {{.TablePrefix}}knowledge_settings_org_project_fkey FOREIGN KEY (org_id, project_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE,
  CONSTRAINT {{.TablePrefix}}knowledge_settings_provider_fkey FOREIGN KEY (project_id, provider_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}model_providers (project_id, id)
);

CREATE INDEX {{.TablePrefix}}knowledge_settings_provider_idx ON {{.Schema}}.{{.TablePrefix}}knowledge_settings (provider_id);

CREATE TABLE {{.Schema}}.{{.TablePrefix}}knowledge_sources (
  id          UUID PRIMARY KEY,
  public_id   TEXT NOT NULL UNIQUE,
  org_id      UUID NOT NULL,
  project_id  UUID NOT NULL,
  kind        TEXT NOT NULL,
  backend     TEXT NOT NULL,
  name        TEXT NOT NULL,
  seed_url    TEXT,
  status       TEXT NOT NULL,
  error_code   TEXT,
  error_reason TEXT,
  refresh_requested BOOLEAN NOT NULL DEFAULT false,
  refreshed_at TIMESTAMPTZ,
  lease_owner  TEXT,
  lease_until TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL,
  updated_at  TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}knowledge_sources_kind_check CHECK (kind IN ('upload', 'url')),
  CONSTRAINT {{.TablePrefix}}knowledge_sources_backend_check CHECK (backend IN ('pgvector')),
  CONSTRAINT {{.TablePrefix}}knowledge_sources_seed_url_shape CHECK ((kind = 'url') = (seed_url IS NOT NULL)),
  CONSTRAINT {{.TablePrefix}}knowledge_sources_status_check CHECK (status IN ('idle', 'crawling', 'failed')),
  CONSTRAINT {{.TablePrefix}}knowledge_sources_lease_shape CHECK ((lease_owner IS NULL) = (lease_until IS NULL)),
  CONSTRAINT {{.TablePrefix}}knowledge_sources_project_name_key UNIQUE (project_id, name),
  CONSTRAINT {{.TablePrefix}}knowledge_sources_project_id_key UNIQUE (project_id, id),
  CONSTRAINT {{.TablePrefix}}knowledge_sources_org_project_id_key UNIQUE (org_id, project_id, id),
  CONSTRAINT {{.TablePrefix}}knowledge_sources_org_project_fkey FOREIGN KEY (org_id, project_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE
);

CREATE INDEX {{.TablePrefix}}knowledge_sources_crawling_idx ON {{.Schema}}.{{.TablePrefix}}knowledge_sources (lease_until)
  WHERE status = 'crawling';

CREATE TABLE {{.Schema}}.{{.TablePrefix}}knowledge_documents (
  id                UUID PRIMARY KEY,
  public_id         TEXT NOT NULL UNIQUE,
  org_id            UUID NOT NULL,
  project_id        UUID NOT NULL,
  source_id         UUID NOT NULL,
  title             TEXT NOT NULL,
  file_id           UUID,
  url               TEXT,
  content           JSONB,
  content_hash      TEXT,
  status            TEXT NOT NULL,
  error_code        TEXT,
  error_reason      TEXT,
  page_count        INT,
  embed_tokens      BIGINT NOT NULL DEFAULT 0,
  embed_cost_micros BIGINT NOT NULL DEFAULT 0,
  attempts          INT NOT NULL DEFAULT 0,
  lease_owner       TEXT,
  lease_until       TIMESTAMPTZ,
  created_at        TIMESTAMPTZ NOT NULL,
  updated_at        TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}knowledge_documents_status_check CHECK (status IN ('uploading', 'pending', 'processing', 'ready', 'failed')),
  CONSTRAINT {{.TablePrefix}}knowledge_documents_origin_shape CHECK (file_id IS NULL OR url IS NULL),
  CONSTRAINT {{.TablePrefix}}knowledge_documents_content_array CHECK (content IS NULL OR jsonb_typeof(content) = 'array'),
  CONSTRAINT {{.TablePrefix}}knowledge_documents_content_size CHECK (content IS NULL OR octet_length(content::text) <= 2097152),
  CONSTRAINT {{.TablePrefix}}knowledge_documents_lease_shape CHECK ((lease_owner IS NULL) = (lease_until IS NULL)),
  CONSTRAINT {{.TablePrefix}}knowledge_documents_org_project_id_key UNIQUE (org_id, project_id, id),
  CONSTRAINT {{.TablePrefix}}knowledge_documents_source_id_key UNIQUE (org_id, project_id, source_id, id),
  CONSTRAINT {{.TablePrefix}}knowledge_documents_source_fkey FOREIGN KEY (org_id, project_id, source_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}knowledge_sources (org_id, project_id, id) ON DELETE CASCADE,
  CONSTRAINT {{.TablePrefix}}knowledge_documents_file_fkey FOREIGN KEY (org_id, project_id, file_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}files (org_id, project_id, id) ON DELETE SET NULL (file_id)
);

CREATE UNIQUE INDEX {{.TablePrefix}}knowledge_documents_source_url_key ON {{.Schema}}.{{.TablePrefix}}knowledge_documents (source_id, url)
  WHERE url IS NOT NULL;
CREATE UNIQUE INDEX {{.TablePrefix}}knowledge_documents_file_key ON {{.Schema}}.{{.TablePrefix}}knowledge_documents (file_id)
  WHERE file_id IS NOT NULL;
CREATE INDEX {{.TablePrefix}}knowledge_documents_work_idx ON {{.Schema}}.{{.TablePrefix}}knowledge_documents (project_id, lease_until)
  WHERE status IN ('pending', 'processing');

CREATE TABLE {{.Schema}}.{{.TablePrefix}}knowledge_chunks (
  id           UUID PRIMARY KEY,
  org_id       UUID NOT NULL,
  project_id   UUID NOT NULL,
  source_id    UUID NOT NULL,
  document_id  UUID NOT NULL,
  ordinal      INT NOT NULL,
  heading_path TEXT NOT NULL DEFAULT '',
  page         INT,
  url          TEXT,
  text         TEXT NOT NULL,
  embedding    public.vector NOT NULL,
  dims         SMALLINT NOT NULL,
  model_key    TEXT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}knowledge_chunks_dims_check CHECK (dims IN (768, 1024, 1536, 3072)),
  CONSTRAINT {{.TablePrefix}}knowledge_chunks_embedding_dims CHECK (public.vector_dims(embedding) = dims),
  CONSTRAINT {{.TablePrefix}}knowledge_chunks_ordinal_check CHECK (ordinal >= 0),
  CONSTRAINT {{.TablePrefix}}knowledge_chunks_document_ordinal_key UNIQUE (document_id, ordinal),
  CONSTRAINT {{.TablePrefix}}knowledge_chunks_document_fkey FOREIGN KEY (org_id, project_id, source_id, document_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}knowledge_documents (org_id, project_id, source_id, id) ON DELETE CASCADE,
  CONSTRAINT {{.TablePrefix}}knowledge_chunks_source_fkey FOREIGN KEY (project_id, source_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}knowledge_sources (project_id, id) ON DELETE CASCADE
);

CREATE INDEX {{.TablePrefix}}knowledge_chunks_scope_idx ON {{.Schema}}.{{.TablePrefix}}knowledge_chunks (project_id, source_id, model_key);
CREATE INDEX {{.TablePrefix}}knowledge_chunks_hnsw_768 ON {{.Schema}}.{{.TablePrefix}}knowledge_chunks
  USING hnsw ((embedding::public.vector(768)) public.vector_cosine_ops) WHERE dims = 768;
CREATE INDEX {{.TablePrefix}}knowledge_chunks_hnsw_1024 ON {{.Schema}}.{{.TablePrefix}}knowledge_chunks
  USING hnsw ((embedding::public.vector(1024)) public.vector_cosine_ops) WHERE dims = 1024;
CREATE INDEX {{.TablePrefix}}knowledge_chunks_hnsw_1536 ON {{.Schema}}.{{.TablePrefix}}knowledge_chunks
  USING hnsw ((embedding::public.vector(1536)) public.vector_cosine_ops) WHERE dims = 1536;
CREATE INDEX {{.TablePrefix}}knowledge_chunks_hnsw_3072 ON {{.Schema}}.{{.TablePrefix}}knowledge_chunks
  USING hnsw ((embedding::public.halfvec(3072)) public.halfvec_cosine_ops) WHERE dims = 3072;

ALTER TABLE {{.Schema}}.{{.TablePrefix}}files ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}files FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}files_tenant ON {{.Schema}}.{{.TablePrefix}}files
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

ALTER TABLE {{.Schema}}.{{.TablePrefix}}knowledge_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}knowledge_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}knowledge_settings_tenant ON {{.Schema}}.{{.TablePrefix}}knowledge_settings
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

ALTER TABLE {{.Schema}}.{{.TablePrefix}}knowledge_sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}knowledge_sources FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}knowledge_sources_tenant ON {{.Schema}}.{{.TablePrefix}}knowledge_sources
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

ALTER TABLE {{.Schema}}.{{.TablePrefix}}knowledge_documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}knowledge_documents FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}knowledge_documents_tenant ON {{.Schema}}.{{.TablePrefix}}knowledge_documents
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

ALTER TABLE {{.Schema}}.{{.TablePrefix}}knowledge_chunks ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}knowledge_chunks FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}knowledge_chunks_tenant ON {{.Schema}}.{{.TablePrefix}}knowledge_chunks
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

-- SECURITY: resolves a source for an operator, who has no active org; it returns only the ids every later read is scoped to.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_knowledge_source_by_public_id(p_public_id text)
RETURNS TABLE (id uuid, org_id uuid, project_id uuid)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT k.id, k.org_id, k.project_id
  FROM {{.Schema}}.{{.TablePrefix}}knowledge_sources k
  WHERE k.public_id = p_public_id
  LIMIT 1;
$$;

REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_knowledge_source_by_public_id(text) FROM PUBLIC;

-- SECURITY: resolves a document for an operator, who has no active org; it returns only the ids every later read is scoped to.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_knowledge_document_by_public_id(p_public_id text)
RETURNS TABLE (id uuid, org_id uuid, project_id uuid, source_id uuid)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT d.id, d.org_id, d.project_id, d.source_id
  FROM {{.Schema}}.{{.TablePrefix}}knowledge_documents d
  WHERE d.public_id = p_public_id
  LIMIT 1;
$$;

REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_knowledge_document_by_public_id(text) FROM PUBLIC;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_knowledge_document_by_public_id(text);
DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_knowledge_source_by_public_id(text);
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}knowledge_chunks;
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}knowledge_documents;
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}knowledge_sources;
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}knowledge_settings;
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}files;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs
  DROP COLUMN IF EXISTS knowledge_retrieve_ms,
  DROP COLUMN IF EXISTS knowledge_retrieve_calls,
  DROP COLUMN IF EXISTS embedding_price_input_micros,
  DROP COLUMN IF EXISTS embedding_tokens;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}model_providers DROP CONSTRAINT IF EXISTS {{.TablePrefix}}model_providers_project_id_key;

-- +goose StatementEnd
