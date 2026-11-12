-- +goose Up
-- +goose StatementBegin

-- NOTE: a run's approval is read on every open, awaiting settle, prompt send and reconcile; the run cascade deletes by it too.
CREATE INDEX {{.TablePrefix}}approvals_run_idx ON {{.Schema}}.{{.TablePrefix}}approvals (org_id, run_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS {{.Schema}}.{{.TablePrefix}}approvals_run_idx;

-- +goose StatementEnd
