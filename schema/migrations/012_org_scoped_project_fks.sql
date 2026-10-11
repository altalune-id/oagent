-- +goose Up
-- +goose StatementBegin

DO $$
DECLARE n bigint;
BEGIN
  SELECT count(*) INTO n FROM {{.Schema}}.{{.TablePrefix}}agents r JOIN {{.Schema}}.{{.TablePrefix}}projects p ON p.id = r.project_id WHERE p.org_id <> r.org_id;
  IF n > 0 THEN
    RAISE EXCEPTION 'migration 012: % row(s) in {{.TablePrefix}}agents have a project from another org; fix them, then migrate again', n;
  END IF;
  SELECT count(*) INTO n FROM {{.Schema}}.{{.TablePrefix}}webhook_endpoints r JOIN {{.Schema}}.{{.TablePrefix}}projects p ON p.id = r.project_id WHERE p.org_id <> r.org_id;
  IF n > 0 THEN
    RAISE EXCEPTION 'migration 012: % row(s) in {{.TablePrefix}}webhook_endpoints have a project from another org; fix them, then migrate again', n;
  END IF;
  SELECT count(*) INTO n FROM {{.Schema}}.{{.TablePrefix}}webhook_deliveries r JOIN {{.Schema}}.{{.TablePrefix}}projects p ON p.id = r.project_id WHERE p.org_id <> r.org_id;
  IF n > 0 THEN
    RAISE EXCEPTION 'migration 012: % row(s) in {{.TablePrefix}}webhook_deliveries have a project from another org; fix them, then migrate again', n;
  END IF;
  SELECT count(*) INTO n FROM {{.Schema}}.{{.TablePrefix}}outbox_entries r JOIN {{.Schema}}.{{.TablePrefix}}projects p ON p.id = r.project_id WHERE p.org_id <> r.org_id;
  IF n > 0 THEN
    RAISE EXCEPTION 'migration 012: % row(s) in {{.TablePrefix}}outbox_entries have a project from another org; fix them, then migrate again', n;
  END IF;
  SELECT count(*) INTO n FROM {{.Schema}}.{{.TablePrefix}}api_keys r JOIN {{.Schema}}.{{.TablePrefix}}projects p ON p.id = r.project_id WHERE p.org_id <> r.org_id;
  IF n > 0 THEN
    RAISE EXCEPTION 'migration 012: % row(s) in {{.TablePrefix}}api_keys have a project from another org; fix them, then migrate again', n;
  END IF;
END $$;

DO $$
DECLARE r record;
BEGIN
  FOR r IN
    SELECT c.conrelid::regclass AS tbl, c.conname
    FROM pg_constraint c
    WHERE c.contype = 'f'
      AND c.confrelid = '{{.Schema}}.{{.TablePrefix}}projects'::regclass
      AND c.conrelid IN ('{{.Schema}}.{{.TablePrefix}}agents'::regclass, '{{.Schema}}.{{.TablePrefix}}webhook_endpoints'::regclass, '{{.Schema}}.{{.TablePrefix}}webhook_deliveries'::regclass, '{{.Schema}}.{{.TablePrefix}}outbox_entries'::regclass, '{{.Schema}}.{{.TablePrefix}}api_keys'::regclass)
      AND c.conkey = ARRAY[(SELECT a.attnum FROM pg_attribute a WHERE a.attrelid = c.conrelid AND a.attname = 'project_id')]
  LOOP
    EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', r.tbl, r.conname);
  END LOOP;
END $$;

ALTER TABLE {{.Schema}}.{{.TablePrefix}}agents
  ADD CONSTRAINT {{.TablePrefix}}agents_org_project_fkey
  FOREIGN KEY (org_id, project_id) REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE;

ALTER TABLE {{.Schema}}.{{.TablePrefix}}webhook_endpoints
  ADD CONSTRAINT {{.TablePrefix}}webhook_endpoints_org_project_fkey
  FOREIGN KEY (org_id, project_id) REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE;

ALTER TABLE {{.Schema}}.{{.TablePrefix}}webhook_deliveries
  ADD CONSTRAINT {{.TablePrefix}}webhook_deliveries_org_project_fkey
  FOREIGN KEY (org_id, project_id) REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE;

ALTER TABLE {{.Schema}}.{{.TablePrefix}}outbox_entries
  ADD CONSTRAINT {{.TablePrefix}}outbox_entries_org_project_fkey
  FOREIGN KEY (org_id, project_id) REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE;

ALTER TABLE {{.Schema}}.{{.TablePrefix}}api_keys
  ADD CONSTRAINT {{.TablePrefix}}api_keys_org_project_fkey
  FOREIGN KEY (org_id, project_id) REFERENCES {{.Schema}}.{{.TablePrefix}}projects (org_id, id) ON DELETE CASCADE;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}agents DROP CONSTRAINT IF EXISTS {{.TablePrefix}}agents_org_project_fkey;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}agents
  ADD CONSTRAINT {{.TablePrefix}}agents_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES {{.Schema}}.{{.TablePrefix}}projects (id) ON DELETE CASCADE;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}webhook_endpoints DROP CONSTRAINT IF EXISTS {{.TablePrefix}}webhook_endpoints_org_project_fkey;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}webhook_endpoints
  ADD CONSTRAINT {{.TablePrefix}}webhook_endpoints_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES {{.Schema}}.{{.TablePrefix}}projects (id) ON DELETE CASCADE;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}webhook_deliveries DROP CONSTRAINT IF EXISTS {{.TablePrefix}}webhook_deliveries_org_project_fkey;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}webhook_deliveries
  ADD CONSTRAINT {{.TablePrefix}}webhook_deliveries_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES {{.Schema}}.{{.TablePrefix}}projects (id) ON DELETE CASCADE;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}outbox_entries DROP CONSTRAINT IF EXISTS {{.TablePrefix}}outbox_entries_org_project_fkey;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}outbox_entries
  ADD CONSTRAINT {{.TablePrefix}}outbox_entries_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES {{.Schema}}.{{.TablePrefix}}projects (id) ON DELETE CASCADE;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}api_keys DROP CONSTRAINT IF EXISTS {{.TablePrefix}}api_keys_org_project_fkey;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}api_keys
  ADD CONSTRAINT {{.TablePrefix}}api_keys_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES {{.Schema}}.{{.TablePrefix}}projects (id) ON DELETE CASCADE;

-- +goose StatementEnd
