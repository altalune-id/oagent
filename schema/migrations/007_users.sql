-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}users (
  id                  UUID PRIMARY KEY,
  public_id           TEXT NOT NULL UNIQUE,
  email               TEXT NOT NULL UNIQUE,
  name                TEXT NOT NULL DEFAULT '',
  avatar_url          TEXT NOT NULL DEFAULT '',
  password_hash       TEXT NOT NULL DEFAULT '',
  idp_issuer          TEXT,
  idp_subject         TEXT,
  locale              TEXT NOT NULL DEFAULT '',
  terms_accepted_at   TIMESTAMPTZ,
  created_at          TIMESTAMPTZ NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}users_email_normalized CHECK (email = lower(btrim(email))),
  CONSTRAINT {{.TablePrefix}}users_idp_pair CHECK ((idp_issuer IS NULL) = (idp_subject IS NULL))
);

CREATE UNIQUE INDEX {{.TablePrefix}}users_idp_idx
  ON {{.Schema}}.{{.TablePrefix}}users (idp_issuer, idp_subject)
  WHERE idp_issuer IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}users;

-- +goose StatementEnd
