-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}web_search_providers (
  project_id            UUID PRIMARY KEY,
  org_id                UUID NOT NULL,
  kind                  TEXT NOT NULL,
  api_key_sealed        BYTEA NOT NULL,
  price_per_call_micros BIGINT NOT NULL,
  created_at            TIMESTAMPTZ NOT NULL,
  updated_at            TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}web_search_providers_price_check CHECK (price_per_call_micros >= 0),
  CONSTRAINT {{.TablePrefix}}web_search_providers_org_project_fkey FOREIGN KEY (org_id, project_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE
);

CREATE TABLE {{.Schema}}.{{.TablePrefix}}http_secrets (
  id           UUID PRIMARY KEY,
  public_id    TEXT NOT NULL UNIQUE,
  org_id       UUID NOT NULL,
  project_id   UUID NOT NULL,
  host         TEXT NOT NULL,
  header       TEXT NOT NULL,
  value_sealed BYTEA NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL,
  updated_at   TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}http_secrets_host_lower CHECK (host = lower(host)),
  CONSTRAINT {{.TablePrefix}}http_secrets_header_lower CHECK (header = lower(header)),
  CONSTRAINT {{.TablePrefix}}http_secrets_key_unique UNIQUE (project_id, host, header),
  CONSTRAINT {{.TablePrefix}}http_secrets_org_project_fkey FOREIGN KEY (org_id, project_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE
);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}web_search_providers ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}web_search_providers FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}web_search_providers_tenant ON {{.Schema}}.{{.TablePrefix}}web_search_providers
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

ALTER TABLE {{.Schema}}.{{.TablePrefix}}http_secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}http_secrets FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}http_secrets_tenant ON {{.Schema}}.{{.TablePrefix}}http_secrets
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

-- SECURITY: resolves a secret before the org is known; it returns only the ids every later read is scoped to, never the sealed value.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_http_secret_by_public_id(p_public_id text)
RETURNS TABLE (id uuid, org_id uuid, project_id uuid)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT s.id, s.org_id, s.project_id
  FROM {{.Schema}}.{{.TablePrefix}}http_secrets s
  WHERE s.public_id = p_public_id
  LIMIT 1;
$$;

REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_http_secret_by_public_id(text) FROM PUBLIC;

ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs
  ADD COLUMN web_search_calls BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN web_search_price_micros BIGINT;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs
  DROP COLUMN IF EXISTS web_search_price_micros,
  DROP COLUMN IF EXISTS web_search_calls;

DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_http_secret_by_public_id(text);
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}http_secrets;
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}web_search_providers;

-- +goose StatementEnd
