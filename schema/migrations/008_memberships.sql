-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}memberships (
  id                  UUID PRIMARY KEY,
  public_id           TEXT NOT NULL UNIQUE,
  org_id              UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}orgs (id) ON DELETE CASCADE,
  user_id             UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}users (id) ON DELETE CASCADE,
  role                TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
  created_at          TIMESTAMPTZ NOT NULL,
  UNIQUE (org_id, user_id)
);

CREATE INDEX {{.TablePrefix}}memberships_user_idx
  ON {{.Schema}}.{{.TablePrefix}}memberships (user_id, created_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}memberships;

-- +goose StatementEnd
