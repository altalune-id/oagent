-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}agents (
  id                  UUID PRIMARY KEY,
  public_id           TEXT NOT NULL UNIQUE,
  org_id              UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}orgs (id) ON DELETE CASCADE,
  project_id          UUID NOT NULL,
  slug                TEXT NOT NULL,
  name                TEXT NOT NULL,
  description         TEXT NOT NULL DEFAULT '',
  created_by          UUID,
  created_at          TIMESTAMPTZ NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL,
  UNIQUE (project_id, slug),
  CONSTRAINT {{.TablePrefix}}agents_org_project_fkey FOREIGN KEY (org_id, project_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE
);

CREATE INDEX {{.TablePrefix}}agents_project_id_idx ON {{.Schema}}.{{.TablePrefix}}agents (project_id);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}agents ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}agents FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}agents_tenant ON {{.Schema}}.{{.TablePrefix}}agents
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}agents;

-- +goose StatementEnd
