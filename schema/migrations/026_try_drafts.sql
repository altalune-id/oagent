-- +goose Up
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}threads
  DROP CONSTRAINT {{.TablePrefix}}threads_kind_check,
  ADD CONSTRAINT {{.TablePrefix}}threads_kind_check CHECK (kind IN ('dm', 'group', 'api', 'try'));

CREATE UNIQUE INDEX {{.TablePrefix}}threads_agent_try_key_key ON {{.Schema}}.{{.TablePrefix}}threads (agent_id, external_key)
  WHERE kind = 'try';

ALTER TABLE {{.Schema}}.{{.TablePrefix}}agent_versions
  ADD COLUMN kind TEXT NOT NULL DEFAULT 'published',
  ADD COLUMN config_hash BYTEA,
  ALTER COLUMN version DROP NOT NULL,
  DROP CONSTRAINT {{.TablePrefix}}agent_versions_version_positive,
  ADD CONSTRAINT {{.TablePrefix}}agent_versions_kind_check CHECK (kind IN ('published', 'draft')),
  ADD CONSTRAINT {{.TablePrefix}}agent_versions_kind_shape CHECK (
    (kind = 'published' AND version IS NOT NULL AND version > 0 AND config_hash IS NULL)
    OR (kind = 'draft' AND version IS NULL AND config_hash IS NOT NULL)
  );

ALTER TABLE {{.Schema}}.{{.TablePrefix}}agent_versions
  ALTER COLUMN kind DROP DEFAULT;

CREATE UNIQUE INDEX {{.TablePrefix}}agent_versions_draft_hash_key ON {{.Schema}}.{{.TablePrefix}}agent_versions (agent_id, config_hash)
  WHERE kind = 'draft';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- NOTE: Down drops all Try it history: every try thread (its messages, runs and snapshot cascade) and every draft version.
DELETE FROM {{.Schema}}.{{.TablePrefix}}threads WHERE kind = 'try';
DROP INDEX IF EXISTS {{.Schema}}.{{.TablePrefix}}agent_versions_draft_hash_key;
DELETE FROM {{.Schema}}.{{.TablePrefix}}agent_versions WHERE kind = 'draft';
ALTER TABLE {{.Schema}}.{{.TablePrefix}}agent_versions
  DROP CONSTRAINT IF EXISTS {{.TablePrefix}}agent_versions_kind_shape,
  DROP CONSTRAINT IF EXISTS {{.TablePrefix}}agent_versions_kind_check,
  ALTER COLUMN version SET NOT NULL,
  ADD CONSTRAINT {{.TablePrefix}}agent_versions_version_positive CHECK (version > 0),
  DROP COLUMN IF EXISTS config_hash,
  DROP COLUMN IF EXISTS kind;
DROP INDEX IF EXISTS {{.Schema}}.{{.TablePrefix}}threads_agent_try_key_key;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}threads
  DROP CONSTRAINT IF EXISTS {{.TablePrefix}}threads_kind_check,
  ADD CONSTRAINT {{.TablePrefix}}threads_kind_check CHECK (kind IN ('dm', 'group', 'api'));

-- +goose StatementEnd
