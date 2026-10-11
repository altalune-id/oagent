-- +goose Up
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}api_keys
  DROP CONSTRAINT {{.TablePrefix}}api_keys_kind_shape;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}api_keys
  ADD CONSTRAINT {{.TablePrefix}}api_keys_kind_shape CHECK (
    (kind = 'project' AND project_id IS NOT NULL AND NOT all_projects)
    OR (kind IN ('org', 'personal') AND project_id IS NULL)
  );

UPDATE {{.Schema}}.{{.TablePrefix}}api_keys k SET created_by = NULL
  WHERE created_by IS NOT NULL
    AND NOT EXISTS (SELECT 1 FROM {{.Schema}}.{{.TablePrefix}}users u WHERE u.id = k.created_by);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}api_keys
  ADD CONSTRAINT {{.TablePrefix}}api_keys_created_by_fkey
  FOREIGN KEY (created_by) REFERENCES {{.Schema}}.{{.TablePrefix}}users (id) ON DELETE SET NULL;

CREATE INDEX {{.TablePrefix}}api_keys_personal_idx
  ON {{.Schema}}.{{.TablePrefix}}api_keys (org_id, created_by, created_at DESC)
  WHERE kind = 'personal';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DELETE FROM {{.Schema}}.{{.TablePrefix}}api_keys WHERE kind = 'personal';
DROP INDEX IF EXISTS {{.Schema}}.{{.TablePrefix}}api_keys_personal_idx;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}api_keys DROP CONSTRAINT IF EXISTS {{.TablePrefix}}api_keys_created_by_fkey;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}api_keys DROP CONSTRAINT {{.TablePrefix}}api_keys_kind_shape;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}api_keys
  ADD CONSTRAINT {{.TablePrefix}}api_keys_kind_shape CHECK (
    (kind = 'project' AND project_id IS NOT NULL AND NOT all_projects)
    OR (kind = 'org' AND project_id IS NULL)
  );

-- +goose StatementEnd
