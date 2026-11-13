-- +goose Up
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs
  ADD COLUMN no_reply_reason TEXT,
  ADD CONSTRAINT {{.TablePrefix}}runs_no_reply_reason_check CHECK (no_reply_reason IN ('no_text', 'set_reply_empty'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs
  DROP CONSTRAINT IF EXISTS {{.TablePrefix}}runs_no_reply_reason_check,
  DROP COLUMN IF EXISTS no_reply_reason;

-- +goose StatementEnd
