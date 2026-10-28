-- +goose Up
-- +goose StatementBegin

-- SECURITY: a transaction-local set_config resets to '' rather than NULL when the transaction ends, so a pooled
-- connection that once carried a scope would make ''::uuid throw 22P02; NULLIF turns "no scope" into "no tenant rows".
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}current_org_id()
RETURNS uuid
LANGUAGE sql STABLE
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT NULLIF(current_setting('app.current_org_id', true), '')::uuid;
$$;

CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}current_user_id()
RETURNS uuid
LANGUAGE sql STABLE
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT NULLIF(current_setting('app.current_user_id', true), '')::uuid;
$$;

CREATE TABLE {{.Schema}}.{{.TablePrefix}}orgs (
  id                  UUID PRIMARY KEY,
  public_id           TEXT NOT NULL UNIQUE,
  slug                TEXT NOT NULL UNIQUE,
  name                TEXT NOT NULL,
  created_by          UUID,
  created_at          TIMESTAMPTZ NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL
);

-- NOTE: the orgs policies read memberships, so they are created in 008_memberships.sql.
ALTER TABLE {{.Schema}}.{{.TablePrefix}}orgs ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}orgs FORCE ROW LEVEL SECURITY;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}orgs;
DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}current_user_id();
DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}current_org_id();

-- +goose StatementEnd
