-- +goose Up
-- +goose StatementBegin

-- SECURITY: pg_temp must stay last in search_path — https://www.postgresql.org/docs/17/sql-createfunction.html#SQL-CREATEFUNCTION-SECURITY
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}list_org_ids()
RETURNS TABLE (id uuid, created_at timestamptz)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT o.id, o.created_at FROM {{.Schema}}.{{.TablePrefix}}orgs o ORDER BY o.created_at ASC, o.id ASC;
$$;

-- SECURITY: resolves the system org on every refresh and at install, before any tenant scope exists.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_system_org()
RETURNS TABLE (id uuid, public_id text, slug text, name text, created_by uuid, created_at timestamptz, updated_at timestamptz, is_system boolean)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT o.id, o.public_id, o.slug, o.name, o.created_by, o.created_at, o.updated_at, o.is_system
  FROM {{.Schema}}.{{.TablePrefix}}orgs o
  WHERE o.is_system
  LIMIT 1;
$$;

CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_org_by_slug(p_slug text)
RETURNS TABLE (id uuid, public_id text, slug text, name text, created_by uuid, created_at timestamptz, updated_at timestamptz, is_system boolean)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT o.id, o.public_id, o.slug, o.name, o.created_by, o.created_at, o.updated_at, o.is_system
  FROM {{.Schema}}.{{.TablePrefix}}orgs o
  WHERE o.slug = p_slug
  LIMIT 1;
$$;

CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_org_by_public_id(p_public_id text)
RETURNS TABLE (id uuid, public_id text, slug text, name text, created_by uuid, created_at timestamptz, updated_at timestamptz, is_system boolean)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT o.id, o.public_id, o.slug, o.name, o.created_by, o.created_at, o.updated_at, o.is_system
  FROM {{.Schema}}.{{.TablePrefix}}orgs o
  WHERE o.public_id = p_public_id
  LIMIT 1;
$$;

-- SECURITY: resolves an invite before any tenant scope exists; the row names the org. The hash only matches, it is never returned.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_invite_by_token_hash(p_hash text)
RETURNS TABLE (id uuid, public_id text, org_id uuid, email text, role text, draft boolean, invited_by uuid, expires_at timestamptz, accepted_at timestamptz, accepted_by uuid, revoked_at timestamptz, created_at timestamptz, updated_at timestamptz)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT i.id, i.public_id, i.org_id, i.email, i.role, i.draft, i.invited_by, i.expires_at, i.accepted_at, i.accepted_by, i.revoked_at, i.created_at, i.updated_at
  FROM {{.Schema}}.{{.TablePrefix}}invites i
  WHERE i.token_hash = p_hash
  LIMIT 1;
$$;

-- SECURITY: resolves the invite named by the landing cookie before any tenant scope exists; the row names the org.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_invite_by_public_id(p_public_id text)
RETURNS TABLE (id uuid, public_id text, org_id uuid, email text, role text, draft boolean, invited_by uuid, expires_at timestamptz, accepted_at timestamptz, accepted_by uuid, revoked_at timestamptz, created_at timestamptz, updated_at timestamptz)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT i.id, i.public_id, i.org_id, i.email, i.role, i.draft, i.invited_by, i.expires_at, i.accepted_at, i.accepted_by, i.revoked_at, i.created_at, i.updated_at
  FROM {{.Schema}}.{{.TablePrefix}}invites i
  WHERE i.public_id = p_public_id
  LIMIT 1;
$$;

-- SECURITY: resolves a key by its secret hash before any tenant scope exists; the row names the org. The hash only matches, it is never returned.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_api_key_by_secret_hash(p_hash bytea)
RETURNS TABLE (id uuid, org_id uuid, project_id uuid, kind text, all_projects boolean, project_ids text, name text, secret_hint text, scopes text, resource_ids text, created_by uuid, created_at timestamptz, expires_at timestamptz, revoked_at timestamptz, last_used_at timestamptz)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT k.id, k.org_id, k.project_id, k.kind, k.all_projects,
         COALESCE((SELECT array_agg(g.project_id ORDER BY g.created_at, g.project_id)
                   FROM {{.Schema}}.{{.TablePrefix}}api_key_projects g
                   WHERE g.org_id = k.org_id AND g.key_id = k.id), '{}')::text,
         k.name, k.secret_hint, k.scopes::text, k.resource_ids::text,
         k.created_by, k.created_at, k.expires_at, k.revoked_at, k.last_used_at
  FROM {{.Schema}}.{{.TablePrefix}}api_keys k
  WHERE k.secret_hash = p_hash
  LIMIT 1;
$$;

-- SECURITY: resolves a project for an operator, who has no active org; it returns only the ids every later read is scoped to.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_project_by_public_id(p_public_id text)
RETURNS TABLE (id uuid, org_id uuid)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT p.id, p.org_id
  FROM {{.Schema}}.{{.TablePrefix}}projects p
  WHERE p.public_id = p_public_id
  LIMIT 1;
$$;

-- SECURITY: resolves an agent for an operator, who has no active org; it returns only the ids every later read is scoped to.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_agent_by_public_id(p_public_id text)
RETURNS TABLE (id uuid, org_id uuid, project_id uuid)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT a.id, a.org_id, a.project_id
  FROM {{.Schema}}.{{.TablePrefix}}agents a
  WHERE a.public_id = p_public_id
  LIMIT 1;
$$;

REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}list_org_ids() FROM PUBLIC;
REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_system_org() FROM PUBLIC;
REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_org_by_slug(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_org_by_public_id(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_invite_by_token_hash(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_invite_by_public_id(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_api_key_by_secret_hash(bytea) FROM PUBLIC;
REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_project_by_public_id(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_agent_by_public_id(text) FROM PUBLIC;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_agent_by_public_id(text);
DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_project_by_public_id(text);
DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_api_key_by_secret_hash(bytea);
DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_invite_by_public_id(text);
DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_invite_by_token_hash(text);
DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_org_by_public_id(text);
DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_org_by_slug(text);
DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_system_org();
DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}list_org_ids();

-- +goose StatementEnd
