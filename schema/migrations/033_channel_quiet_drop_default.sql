-- +goose Up
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}channel_connections
  ALTER COLUMN quiet_window_ms DROP DEFAULT,
  ALTER COLUMN quiet_max_ms DROP DEFAULT;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}channel_connections
  ALTER COLUMN quiet_window_ms SET DEFAULT 3000,
  ALTER COLUMN quiet_max_ms SET DEFAULT 10000;

-- +goose StatementEnd
