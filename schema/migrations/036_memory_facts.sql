-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}memory_stores (
  id         UUID PRIMARY KEY,
  public_id  TEXT NOT NULL UNIQUE,
  org_id     UUID NOT NULL,
  project_id UUID NOT NULL,
  name       TEXT NOT NULL,
  backend    TEXT NOT NULL,
  remember_api_users BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}memory_stores_backend_check CHECK (backend IN ('native')),
  CONSTRAINT {{.TablePrefix}}memory_stores_name_check CHECK (char_length(name) BETWEEN 1 AND 200),
  CONSTRAINT {{.TablePrefix}}memory_stores_project_name_key UNIQUE (project_id, name),
  CONSTRAINT {{.TablePrefix}}memory_stores_org_project_id_key UNIQUE (org_id, project_id, id),
  CONSTRAINT {{.TablePrefix}}memory_stores_org_project_fkey FOREIGN KEY (org_id, project_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE
);

CREATE TABLE {{.Schema}}.{{.TablePrefix}}memory_facts (
  id            UUID PRIMARY KEY,
  public_id     TEXT NOT NULL UNIQUE,
  org_id        UUID NOT NULL,
  project_id    UUID NOT NULL,
  store_id      UUID NOT NULL,
  agent_id      UUID NOT NULL,
  end_user_id   UUID NOT NULL,
  text          TEXT NOT NULL,
  text_hash     BYTEA NOT NULL,
  source_run_id UUID,
  created_at    TIMESTAMPTZ NOT NULL,
  search        tsvector GENERATED ALWAYS AS (to_tsvector('simple', text)) STORED,
  CONSTRAINT {{.TablePrefix}}memory_facts_text_check CHECK (char_length(text) BETWEEN 1 AND 500),
  CONSTRAINT {{.TablePrefix}}memory_facts_hash_key UNIQUE (store_id, end_user_id, agent_id, text_hash),
  CONSTRAINT {{.TablePrefix}}memory_facts_org_project_fkey FOREIGN KEY (org_id, project_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE,
  CONSTRAINT {{.TablePrefix}}memory_facts_store_fkey FOREIGN KEY (org_id, project_id, store_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}memory_stores (org_id, project_id, id) ON DELETE CASCADE,
  CONSTRAINT {{.TablePrefix}}memory_facts_end_user_fkey FOREIGN KEY (org_id, project_id, end_user_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}end_users (org_id, project_id, id) ON DELETE CASCADE,
  CONSTRAINT {{.TablePrefix}}memory_facts_agent_fkey FOREIGN KEY (org_id, project_id, agent_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}agents (org_id, project_id, id) ON DELETE CASCADE,
  CONSTRAINT {{.TablePrefix}}memory_facts_run_fkey FOREIGN KEY (org_id, project_id, source_run_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}runs (org_id, project_id, id) ON DELETE SET NULL (source_run_id)
);

CREATE INDEX {{.TablePrefix}}memory_facts_scope_idx ON {{.Schema}}.{{.TablePrefix}}memory_facts
  (project_id, store_id, end_user_id, agent_id, created_at DESC, public_id DESC);
CREATE INDEX {{.TablePrefix}}memory_facts_store_idx ON {{.Schema}}.{{.TablePrefix}}memory_facts (store_id);
CREATE INDEX {{.TablePrefix}}memory_facts_agent_idx ON {{.Schema}}.{{.TablePrefix}}memory_facts (agent_id);
CREATE INDEX {{.TablePrefix}}memory_facts_end_user_idx ON {{.Schema}}.{{.TablePrefix}}memory_facts (end_user_id);
CREATE INDEX {{.TablePrefix}}memory_facts_run_idx ON {{.Schema}}.{{.TablePrefix}}memory_facts (source_run_id)
  WHERE source_run_id IS NOT NULL;

ALTER TABLE {{.Schema}}.{{.TablePrefix}}memory_stores ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}memory_stores FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}memory_stores_tenant ON {{.Schema}}.{{.TablePrefix}}memory_stores
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

ALTER TABLE {{.Schema}}.{{.TablePrefix}}memory_facts ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}memory_facts FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}memory_facts_tenant ON {{.Schema}}.{{.TablePrefix}}memory_facts
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

-- SECURITY: resolves a store for an operator, who has no active org; it returns only the ids every later read is scoped to.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_memory_store_by_public_id(p_public_id text)
RETURNS TABLE (id uuid, org_id uuid, project_id uuid)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT m.id, m.org_id, m.project_id
  FROM {{.Schema}}.{{.TablePrefix}}memory_stores m
  WHERE m.public_id = p_public_id
  LIMIT 1;
$$;

REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_memory_store_by_public_id(text) FROM PUBLIC;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_memory_store_by_public_id(text);
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}memory_facts;
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}memory_stores;

-- +goose StatementEnd
