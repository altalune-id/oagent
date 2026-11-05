-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}mcp_servers (
  id          UUID PRIMARY KEY,
  public_id   TEXT NOT NULL UNIQUE,
  org_id      UUID NOT NULL,
  project_id  UUID NOT NULL,
  name        TEXT NOT NULL,
  url         TEXT NOT NULL,
  auth_kind   TEXT NOT NULL,
  auth_header TEXT,
  auth_sealed BYTEA,
  timeout_ms  INT NOT NULL,
  trusted     BOOLEAN NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL,
  updated_at  TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}mcp_servers_name_format CHECK (name ~ '^[a-z0-9][a-z0-9-]{0,31}$'),
  CONSTRAINT {{.TablePrefix}}mcp_servers_auth_kind_check CHECK (auth_kind IN ('none', 'bearer', 'header')),
  CONSTRAINT {{.TablePrefix}}mcp_servers_timeout_range CHECK (timeout_ms BETWEEN 1000 AND 20000),
  CONSTRAINT {{.TablePrefix}}mcp_servers_auth_sealed_shape CHECK ((auth_kind = 'none') = (auth_sealed IS NULL)),
  CONSTRAINT {{.TablePrefix}}mcp_servers_auth_header_shape CHECK ((auth_kind = 'header') = (auth_header IS NOT NULL)),
  CONSTRAINT {{.TablePrefix}}mcp_servers_project_name_key UNIQUE (project_id, name),
  CONSTRAINT {{.TablePrefix}}mcp_servers_org_project_fkey FOREIGN KEY (org_id, project_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE
);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}mcp_servers ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}mcp_servers FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}mcp_servers_tenant ON {{.Schema}}.{{.TablePrefix}}mcp_servers
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

-- SECURITY: resolves an MCP server for an operator, who has no active org; it returns only the ids every later read is scoped to.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_mcp_server_by_public_id(p_public_id text)
RETURNS TABLE (id uuid, org_id uuid, project_id uuid)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT m.id, m.org_id, m.project_id
  FROM {{.Schema}}.{{.TablePrefix}}mcp_servers m
  WHERE m.public_id = p_public_id
  LIMIT 1;
$$;

REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_mcp_server_by_public_id(text) FROM PUBLIC;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_mcp_server_by_public_id(text);
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}mcp_servers;

-- +goose StatementEnd
