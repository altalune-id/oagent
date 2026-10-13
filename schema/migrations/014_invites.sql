-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}invites (
  id          UUID PRIMARY KEY,
  public_id   TEXT NOT NULL UNIQUE,
  org_id      UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}orgs (id) ON DELETE CASCADE,
  email       TEXT NOT NULL CHECK (email = lower(btrim(email))),
  role        TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
  -- SECURITY: only the SHA-256 of the token is stored; NULL only for a draft.
  token_hash  TEXT UNIQUE,
  draft       BOOLEAN NOT NULL DEFAULT false,
  invited_by  UUID REFERENCES {{.Schema}}.{{.TablePrefix}}users (id) ON DELETE SET NULL,
  expires_at  TIMESTAMPTZ NOT NULL,
  accepted_at TIMESTAMPTZ,
  accepted_by UUID REFERENCES {{.Schema}}.{{.TablePrefix}}users (id) ON DELETE SET NULL,
  revoked_at  TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL,
  updated_at  TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}invites_token_or_draft CHECK (draft OR token_hash IS NOT NULL)
);

CREATE UNIQUE INDEX {{.TablePrefix}}invites_live_idx
  ON {{.Schema}}.{{.TablePrefix}}invites (org_id, email)
  WHERE accepted_at IS NULL AND revoked_at IS NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}invites;

-- +goose StatementEnd
