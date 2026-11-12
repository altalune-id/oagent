-- +goose Up
-- +goose StatementBegin

-- SECURITY: only messages after the prompt was sent may decide an approval; prompt_after_seq is the thread's last seq when the prompt went out.
ALTER TABLE {{.Schema}}.{{.TablePrefix}}approvals
  ADD COLUMN prompt_after_seq BIGINT;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}approvals
  DROP COLUMN IF EXISTS prompt_after_seq;

-- +goose StatementEnd
