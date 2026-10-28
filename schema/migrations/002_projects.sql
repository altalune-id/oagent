-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}projects (
  id                  UUID PRIMARY KEY,
  public_id           TEXT NOT NULL UNIQUE,
  org_id              UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}orgs (id) ON DELETE CASCADE,
  slug                TEXT NOT NULL,
  name                TEXT NOT NULL,
  created_by          UUID,
  created_at          TIMESTAMPTZ NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL,
  UNIQUE (org_id, slug),
  CONSTRAINT {{.TablePrefix}}projects_org_id_id_key UNIQUE (org_id, id)
);

CREATE INDEX {{.TablePrefix}}projects_org_id_idx ON {{.Schema}}.{{.TablePrefix}}projects (org_id);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}projects FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}projects_tenant ON {{.Schema}}.{{.TablePrefix}}projects
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}projects;

-- +goose StatementEnd
