-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}sessions (
  sid                 TEXT PRIMARY KEY,
  user_id             UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}users (id) ON DELETE CASCADE,
  payload             BYTEA NOT NULL,
  expires_at          TIMESTAMPTZ NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL
);

CREATE INDEX {{.TablePrefix}}sessions_expires_idx ON {{.Schema}}.{{.TablePrefix}}sessions (expires_at);
CREATE INDEX {{.TablePrefix}}sessions_user_idx ON {{.Schema}}.{{.TablePrefix}}sessions (user_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}sessions;

-- +goose StatementEnd
