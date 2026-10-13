-- +goose Up
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}outbox_entries
  ADD COLUMN public_id TEXT NOT NULL,
  ADD COLUMN event_public_id TEXT NOT NULL,
  ADD CONSTRAINT {{.TablePrefix}}outbox_entries_public_id_key UNIQUE (public_id);

CREATE UNIQUE INDEX {{.TablePrefix}}api_keys_live_hint_key
  ON {{.Schema}}.{{.TablePrefix}}api_keys (org_id, secret_hint)
  WHERE revoked_at IS NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS {{.Schema}}.{{.TablePrefix}}api_keys_live_hint_key;

ALTER TABLE {{.Schema}}.{{.TablePrefix}}outbox_entries
  DROP COLUMN IF EXISTS event_public_id,
  DROP COLUMN IF EXISTS public_id;

-- +goose StatementEnd
