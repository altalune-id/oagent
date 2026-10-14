-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}password_resets (
  id         UUID PRIMARY KEY,
  public_id  TEXT NOT NULL UNIQUE,
  user_id    UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}users (id) ON DELETE CASCADE,
  -- SECURITY: only the SHA-256 of the token is stored.
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  used_at    TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX {{.TablePrefix}}password_resets_user_idx
  ON {{.Schema}}.{{.TablePrefix}}password_resets (user_id)
  WHERE used_at IS NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}password_resets;

-- +goose StatementEnd
