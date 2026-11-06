-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}end_users (
  id         UUID PRIMARY KEY,
  public_id  TEXT NOT NULL UNIQUE,
  org_id     UUID NOT NULL,
  project_id UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}end_users_org_project_id_key UNIQUE (org_id, project_id, id),
  CONSTRAINT {{.TablePrefix}}end_users_org_project_fkey FOREIGN KEY (org_id, project_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE
);

CREATE TABLE {{.Schema}}.{{.TablePrefix}}end_user_identities (
  id          UUID PRIMARY KEY,
  org_id      UUID NOT NULL,
  project_id  UUID NOT NULL,
  end_user_id UUID NOT NULL,
  kind        TEXT NOT NULL,
  value       TEXT NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}end_user_identities_kind_check CHECK (kind IN ('api_user', 'wa_phone', 'wa_lid', 'widget_anon', 'widget_sub')),
  CONSTRAINT {{.TablePrefix}}end_user_identities_value_length CHECK (length(value) BETWEEN 1 AND 256),
  CONSTRAINT {{.TablePrefix}}end_user_identities_project_kind_value_key UNIQUE (project_id, kind, value),
  CONSTRAINT {{.TablePrefix}}end_user_identities_end_user_fkey FOREIGN KEY (org_id, project_id, end_user_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}end_users (org_id, project_id, id) ON DELETE CASCADE
);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}end_users ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}end_users FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}end_users_tenant ON {{.Schema}}.{{.TablePrefix}}end_users
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

ALTER TABLE {{.Schema}}.{{.TablePrefix}}end_user_identities ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}end_user_identities FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}end_user_identities_tenant ON {{.Schema}}.{{.TablePrefix}}end_user_identities
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}end_user_identities;
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}end_users;

-- +goose StatementEnd
