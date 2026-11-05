-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}model_providers (
  id             UUID PRIMARY KEY,
  public_id      TEXT NOT NULL UNIQUE,
  org_id         UUID NOT NULL,
  project_id     UUID NOT NULL,
  name           TEXT NOT NULL,
  kind           TEXT NOT NULL,
  base_url       TEXT NOT NULL,
  api_key_sealed BYTEA NOT NULL,
  models         JSONB NOT NULL,
  created_at     TIMESTAMPTZ NOT NULL,
  updated_at     TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}model_providers_kind_check CHECK (kind IN ('openai_compatible')),
  CONSTRAINT {{.TablePrefix}}model_providers_models_array CHECK (jsonb_typeof(models) = 'array'),
  CONSTRAINT {{.TablePrefix}}model_providers_project_name_key UNIQUE (project_id, name),
  CONSTRAINT {{.TablePrefix}}model_providers_org_project_fkey FOREIGN KEY (org_id, project_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE
);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}model_providers ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}model_providers FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}model_providers_tenant ON {{.Schema}}.{{.TablePrefix}}model_providers
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

-- SECURITY: resolves a provider for an operator, who has no active org; it returns only the ids every later read is scoped to.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_model_provider_by_public_id(p_public_id text)
RETURNS TABLE (id uuid, org_id uuid, project_id uuid)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT p.id, p.org_id, p.project_id
  FROM {{.Schema}}.{{.TablePrefix}}model_providers p
  WHERE p.public_id = p_public_id
  LIMIT 1;
$$;

REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_model_provider_by_public_id(text) FROM PUBLIC;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_model_provider_by_public_id(text);
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}model_providers;

-- +goose StatementEnd
