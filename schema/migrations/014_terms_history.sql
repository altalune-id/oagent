-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}legal_documents (
  id           UUID PRIMARY KEY,
  slug         TEXT NOT NULL CHECK (slug IN ('terms', 'privacy')),
  version      TEXT NOT NULL,
  title        TEXT NOT NULL,
  body         TEXT NOT NULL,
  sha256       TEXT NOT NULL,
  published_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}legal_documents_slug_version_key UNIQUE (slug, version)
);

CREATE TABLE {{.Schema}}.{{.TablePrefix}}terms_acceptances (
  id                  UUID PRIMARY KEY,
  public_id           TEXT NOT NULL UNIQUE,
  user_id             UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}users (id) ON DELETE CASCADE,
  terms_document_id   UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}legal_documents (id),
  privacy_document_id UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}legal_documents (id),
  source              TEXT NOT NULL CHECK (source IN ('install', 'join', 'welcome')),
  accepted_at         TIMESTAMPTZ NOT NULL
);

CREATE INDEX {{.TablePrefix}}terms_acceptances_user_idx
  ON {{.Schema}}.{{.TablePrefix}}terms_acceptances (user_id, accepted_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}terms_acceptances;
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}legal_documents;

-- +goose StatementEnd
