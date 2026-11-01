-- +goose Up
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}projects
  ADD COLUMN environment TEXT NOT NULL DEFAULT '',
  ADD CONSTRAINT {{.TablePrefix}}projects_environment_check CHECK (environment IN ('', 'production', 'staging', 'development', 'labs'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}projects DROP COLUMN IF EXISTS environment;

-- +goose StatementEnd
