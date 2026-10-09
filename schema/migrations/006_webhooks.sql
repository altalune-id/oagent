-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}webhook_endpoints (
  id                  UUID PRIMARY KEY,
  public_id           TEXT NOT NULL,
  org_id              UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}orgs (id) ON DELETE CASCADE,
  project_id          UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}projects (id) ON DELETE CASCADE,
  url                 TEXT NOT NULL,
  description         TEXT NOT NULL DEFAULT '',
  event_types         TEXT NOT NULL,
  secret_primary      BYTEA NOT NULL,
  secret_secondary    BYTEA,
  active              BOOLEAN NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}webhook_endpoints_public_id_key UNIQUE (public_id),
  CONSTRAINT {{.TablePrefix}}webhook_endpoints_secondary_nonempty CHECK (secret_secondary IS NULL OR length(secret_secondary) > 0)
);

CREATE INDEX {{.TablePrefix}}webhook_endpoints_org_project_idx
  ON {{.Schema}}.{{.TablePrefix}}webhook_endpoints (org_id, project_id);

CREATE TABLE {{.Schema}}.{{.TablePrefix}}webhook_deliveries (
  id                  UUID PRIMARY KEY,
  org_id              UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}orgs (id) ON DELETE CASCADE,
  project_id          UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}projects (id) ON DELETE CASCADE,
  endpoint_id         UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}webhook_endpoints (id) ON DELETE CASCADE,
  delivery_id         UUID NOT NULL,
  event_id            UUID NOT NULL,
  event_type          TEXT NOT NULL,
  attempt             INTEGER NOT NULL,
  status_code         INTEGER NOT NULL DEFAULT 0,
  error               TEXT NOT NULL DEFAULT '',
  response_body       TEXT NOT NULL DEFAULT '',
  response_truncated  BOOLEAN NOT NULL DEFAULT false,
  response_headers    TEXT NOT NULL DEFAULT '[]',
  duration_ms         INTEGER NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL
);

CREATE INDEX {{.TablePrefix}}webhook_deliveries_org_delivery_idx
  ON {{.Schema}}.{{.TablePrefix}}webhook_deliveries (org_id, delivery_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}webhook_deliveries;
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}webhook_endpoints;

-- +goose StatementEnd
