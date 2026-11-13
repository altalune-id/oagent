-- +goose Up
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs
  ADD COLUMN knowledge_passages INT NOT NULL DEFAULT 0;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs DROP COLUMN IF EXISTS knowledge_passages;

-- +goose StatementEnd
