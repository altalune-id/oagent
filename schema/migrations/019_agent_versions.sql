-- +goose Up
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}agents
  ADD COLUMN draft_config JSONB NOT NULL DEFAULT '{}',
  ADD COLUMN draft_updated_at TIMESTAMPTZ,
  ADD COLUMN published_version INT NOT NULL DEFAULT 0,
  ADD COLUMN published_at TIMESTAMPTZ,
  ADD CONSTRAINT {{.TablePrefix}}agents_draft_config_object CHECK (jsonb_typeof(draft_config) = 'object'),
  ADD CONSTRAINT {{.TablePrefix}}agents_published_version_nonnegative CHECK (published_version >= 0),
  ADD CONSTRAINT {{.TablePrefix}}agents_org_project_id_key UNIQUE (org_id, project_id, id);

CREATE TABLE {{.Schema}}.{{.TablePrefix}}agent_versions (
  id         UUID PRIMARY KEY,
  public_id  TEXT NOT NULL UNIQUE,
  org_id     UUID NOT NULL,
  project_id UUID NOT NULL,
  agent_id   UUID NOT NULL,
  version    INT NOT NULL,
  config     JSONB NOT NULL,
  created_by UUID,
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}agent_versions_version_positive CHECK (version > 0),
  CONSTRAINT {{.TablePrefix}}agent_versions_config_object CHECK (jsonb_typeof(config) = 'object'),
  CONSTRAINT {{.TablePrefix}}agent_versions_agent_version_key UNIQUE (agent_id, version),
  CONSTRAINT {{.TablePrefix}}agent_versions_agent_fkey FOREIGN KEY (org_id, project_id, agent_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}agents (org_id, project_id, id) ON DELETE CASCADE
);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}agent_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}agent_versions FORCE ROW LEVEL SECURITY;
-- SECURITY: versions are immutable: the service role may read and insert them, never update or delete one; they go with their agent.
CREATE POLICY {{.TablePrefix}}agent_versions_read ON {{.Schema}}.{{.TablePrefix}}agent_versions
  FOR SELECT
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());
CREATE POLICY {{.TablePrefix}}agent_versions_insert ON {{.Schema}}.{{.TablePrefix}}agent_versions
  FOR INSERT
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}agent_versions;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}agents
  DROP CONSTRAINT IF EXISTS {{.TablePrefix}}agents_org_project_id_key,
  DROP COLUMN IF EXISTS published_at,
  DROP COLUMN IF EXISTS published_version,
  DROP COLUMN IF EXISTS draft_updated_at,
  DROP COLUMN IF EXISTS draft_config;

-- +goose StatementEnd
