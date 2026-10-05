-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}orgs (
  id                  UUID PRIMARY KEY,
  public_id           TEXT NOT NULL UNIQUE,
  slug                TEXT NOT NULL UNIQUE,
  name                TEXT NOT NULL,
  created_by          UUID,
  created_at          TIMESTAMPTZ NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}orgs;

-- +goose StatementEnd
