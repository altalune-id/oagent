-- +goose Up
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}orgs
  ADD COLUMN is_system BOOLEAN NOT NULL DEFAULT false;

CREATE UNIQUE INDEX {{.TablePrefix}}orgs_one_system
  ON {{.Schema}}.{{.TablePrefix}}orgs (is_system)
  WHERE is_system;

UPDATE {{.Schema}}.{{.TablePrefix}}orgs o SET created_by = NULL
  WHERE created_by IS NOT NULL
    AND NOT EXISTS (SELECT 1 FROM {{.Schema}}.{{.TablePrefix}}users u WHERE u.id = o.created_by);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}orgs
  ADD CONSTRAINT {{.TablePrefix}}orgs_created_by_fkey
  FOREIGN KEY (created_by) REFERENCES {{.Schema}}.{{.TablePrefix}}users (id) ON DELETE SET NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}orgs DROP CONSTRAINT IF EXISTS {{.TablePrefix}}orgs_created_by_fkey;
DROP INDEX IF EXISTS {{.Schema}}.{{.TablePrefix}}orgs_one_system;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}orgs DROP COLUMN IF EXISTS is_system;

-- +goose StatementEnd
