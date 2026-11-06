-- +goose Up
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}messages
  ADD COLUMN request_hash BYTEA;

ALTER TABLE {{.Schema}}.{{.TablePrefix}}threads
  ADD COLUMN actor_end_user_id UUID,
  ADD CONSTRAINT {{.TablePrefix}}threads_actor_fkey FOREIGN KEY (org_id, project_id, actor_end_user_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}end_users (org_id, project_id, id) ON DELETE SET NULL (actor_end_user_id);

CREATE INDEX {{.TablePrefix}}threads_project_activity_idx ON {{.Schema}}.{{.TablePrefix}}threads (project_id, updated_at DESC, public_id DESC);
CREATE INDEX {{.TablePrefix}}runs_thread_started_idx ON {{.Schema}}.{{.TablePrefix}}runs (thread_id, started_at DESC, public_id DESC);

-- SECURITY: resolves a thread for an operator, who has no active org; it returns only the ids every later read is scoped to.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_thread_by_public_id(p_public_id text)
RETURNS TABLE (id uuid, org_id uuid, project_id uuid, agent_id uuid)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT t.id, t.org_id, t.project_id, t.agent_id
  FROM {{.Schema}}.{{.TablePrefix}}threads t
  WHERE t.public_id = p_public_id
  LIMIT 1;
$$;

REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_thread_by_public_id(text) FROM PUBLIC;

-- SECURITY: resolves a run for an operator, who has no active org; it returns only the ids every later read is scoped to.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_run_by_public_id(p_public_id text)
RETURNS TABLE (id uuid, org_id uuid, project_id uuid, agent_id uuid)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT r.id, r.org_id, r.project_id, t.agent_id
  FROM {{.Schema}}.{{.TablePrefix}}runs r
  JOIN {{.Schema}}.{{.TablePrefix}}threads t ON t.id = r.thread_id
  WHERE r.public_id = p_public_id
  LIMIT 1;
$$;

REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_run_by_public_id(text) FROM PUBLIC;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_run_by_public_id(text);
DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_thread_by_public_id(text);
DROP INDEX IF EXISTS {{.Schema}}.{{.TablePrefix}}runs_thread_started_idx;
DROP INDEX IF EXISTS {{.Schema}}.{{.TablePrefix}}threads_project_activity_idx;

ALTER TABLE {{.Schema}}.{{.TablePrefix}}threads
  DROP CONSTRAINT IF EXISTS {{.TablePrefix}}threads_actor_fkey,
  DROP COLUMN IF EXISTS actor_end_user_id;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}messages
  DROP COLUMN IF EXISTS request_hash;

-- +goose StatementEnd
