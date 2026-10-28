# Change the database schema

Use this when you add or change a table, column, constraint or index. Migrations are goose SQL files embedded in `schema`, run by the app-agnostic `migrator` package. The store code that reads the new shape is [store-method.md](store-method.md); a new aggregate is [module.md](module.md).

## Plan

- [ ] **One migration per change**, at the next number. Never edit a migration that already shipped on `main`.
- [ ] **Fresh DB.** Nothing runs oagent in production yet and the DB is reset when the schema breaks. Write the plain DDL: no backfill `UPDATE`, no `RAISE EXCEPTION` data guard, no SQL id generator. Add `NOT NULL` columns directly and say in the hand-over that the DB must be reset.
- [ ] **Ids.** `id UUID PRIMARY KEY` and, for any row a surface shows, `public_id TEXT NOT NULL UNIQUE`. Both come from Go (the aggregate's constructor sets `uuid.New()` and a `nanoid.New(21)` public id), never a column default. See [public-ids.md](public-ids.md).
- [ ] **Tenant columns.** A project-scoped row carries `org_id` and `project_id` and a composite FK `(org_id, project_id) → projects (org_id, id) ON DELETE CASCADE`, so its project always belongs to its org. An org-scoped row carries `org_id` with `ON DELETE CASCADE`.
- [ ] **Tenant table.** A table with an `org_id` column (or `orgs` itself) is org-owned. In the same migration it gets `ENABLE` and `FORCE ROW LEVEL SECURITY` and one `FOR ALL` policy `<table>_tenant` whose `USING` and `WITH CHECK` read the scope through `{{.Schema}}.{{.TablePrefix}}current_org_id()`, never an inline `current_setting` (`TestMigrations_PoliciesUseTheGuardedScopeHelpers`). Only `orgs` and `memberships` have a `FOR SELECT` policy that also reads `current_user_id()`, with separate org-only write policies. The boot RLS guard refuses to start otherwise. A table without `org_id` (user-scoped or global) gets no RLS; list it as global in `schema/README.md`.
- [ ] **Pre-sign-in or cross-tenant reads** go through a narrow `SECURITY DEFINER` function in a migration: explicit `RETURNS TABLE` with public columns only (never a hash, a sealed value or a secret), one exact key, `SET search_path = {{.Schema}}, pg_catalog, pg_temp`, `REVOKE ALL … FROM PUBLIC`. The guard refuses a definer that PUBLIC may execute or that the schema owner does not own.
- [ ] **Constraints in SQL.** Uniqueness, shape checks and partial unique indexes live in the table, not only in Go.
- [ ] **Secrets.** Store a hash (`token_hash`, `secret_hash`) or a sealed value, never the raw token.

## Build

1. **`schema/migrations/NNN_<name>.sql`** (three digits, snake_case). Every name is templated: `{{.Schema}}.{{.TablePrefix}}<table>` for tables, `{{.TablePrefix}}<table>_<what>` for constraints and indexes. Copy the shape of `017_password_resets.sql`:

   ```sql
   -- +goose Up
   -- +goose StatementBegin

   CREATE TABLE {{.Schema}}.{{.TablePrefix}}password_resets (
     id         UUID PRIMARY KEY,
     public_id  TEXT NOT NULL UNIQUE,
     user_id    UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}users (id) ON DELETE CASCADE,
     created_at TIMESTAMPTZ NOT NULL
   );

   -- +goose StatementEnd

   -- +goose Down
   -- +goose StatementBegin

   DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}password_resets;

   -- +goose StatementEnd
   ```

   An org-owned table ends its `Up` with its policy:

   ```sql
   ALTER TABLE {{.Schema}}.{{.TablePrefix}}agents ENABLE ROW LEVEL SECURITY;
   ALTER TABLE {{.Schema}}.{{.TablePrefix}}agents FORCE ROW LEVEL SECURITY;
   CREATE POLICY {{.TablePrefix}}agents_tenant ON {{.Schema}}.{{.TablePrefix}}agents
     FOR ALL
     USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
     WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());
   ```

   Times are `TIMESTAMPTZ`, set from Go. A `-- SECURITY:` comment explains a column that holds a hash. `Down` undoes exactly what `Up` did (`DROP ... IF EXISTS`, in reverse order).

2. **Bump `schema/migrations/VERSION`** to the new number. `TestNew_TargetVersion` fails otherwise.
3. **jet binding** in `schema/entity/<table>.go`: a `<Table>` struct with one column field per column plus `AllColumns`, and `New<Table>(schema, tablePrefix string)`. Add the table and its column list to `TestBindings` (`schema/entity/entity_test.go`). For a changed table, update both.
4. **Store** builds the binding once in its constructor (`entity.NewPasswordResets(schema, tablePrefix)`) and reads and writes through it ([store-method.md](store-method.md)).
5. **`schema/README.md` in the same commit:**
   - the mermaid `erDiagram`: the relationship lines and the table block with every column (`uuid id PK`, `text public_id UK`, `uuid org_id FK`, notes in quotes);
   - the **Constraints and indexes** table: every new unique, check, partial or foreign-key rule the diagram cannot show, names without `{{.TablePrefix}}`.
6. **Integration tests** (`//go:build integration`, `internal/testutil/pgtest`):
   - the store's `postgres_integration_test.go` proves each constraint the code relies on (duplicate refused, cascade on delete, cross-org project refused);
   - a schema-wide rule (one FK shape on many tables) gets a test in `schema/` next to `project_fk_integration_test.go`;
   - an org-owned table joins `rlsInserts()` in `schema/rls_isolation_integration_test.go`, so `TestRLS_EveryOrgOwnedTableIsolatesTenants` proves, as the service role, one tenant's rows with a scope, none without, and a refused cross-tenant write; a definer function gets a case in `schema/lookups_integration_test.go`;
   - store tests run as the service role through `pgtest.MigratedRLS(t)` and seed other tenants through its admin handle;
   - `TestMigrations_UpDownUp` and `TestMigrations_TwoPrefixesShareASchema` (`schema/migrate_integration_test.go`) run every migration up, fully down and up again under a prefix, so a broken `Down` or an unprefixed name fails there.

## How migrations run

- `boot.migrate` (`internal/boot/db.go`) runs them at boot when `db.autoMigrate` is true (the default), on `postgres.migrator.dsn` (falls back to `postgres.dsn`), with `SET ROLE postgres.migrator.role` when set, under the advisory lock `migrate:<tablePrefix>` so concurrent instances migrate once.
- After migrating, the boot RLS guard (`guardRLS`, `internal/boot/db.go`) audits the schema: every org-owned table is `ENABLE` + `FORCE` with a scoped read and write policy and shows no rows to a scope no tenant owns, and every definer is owned by the schema owner and closed to PUBLIC.
- `postgres.dsn` may point at a pooler; `postgres.migrator.dsn` must reach Postgres directly.
- `schema.New(cfg).WithUpgrade(name, fn)` runs another tool's migrations after ours; that tool owns its own table prefix.

## Review

- [ ] Next number, `VERSION` bumped, `Up` and `Down` both present and symmetric.
- [ ] Every table, constraint and index name goes through `{{.Schema}}` / `{{.TablePrefix}}`.
- [ ] No backfill, no data guard, no SQL-generated ids or timestamps.
- [ ] Project-scoped rows use the composite `(org_id, project_id)` FK; surfaced rows have `public_id`.
- [ ] Raw tokens and secrets are never stored.
- [ ] An org-owned table has `ENABLE` + `FORCE` + a `current_org_id()` policy in the same migration, and a row in `rlsInserts()`; a definer returns public columns only and is closed to PUBLIC.
- [ ] `schema/entity` binding and `TestBindings` match the columns.
- [ ] `schema/README.md` diagram and constraints table updated in the same commit.
- [ ] Gates: `make check`, `make test-integration`, `make cover` (≥ 85% per package).

## Reference code

- [schema/migrations/017_password_resets.sql](../../schema/migrations/017_password_resets.sql): a plain new table with a partial index
- [schema/migrations/014_invites.sql](../../schema/migrations/014_invites.sql): checks, a named constraint and a partial unique index
- [schema/migrations/016_public_ids.sql](../../schema/migrations/016_public_ids.sql): adding `NOT NULL` columns with no backfill
- [schema/migrations/003_agents.sql](../../schema/migrations/003_agents.sql): an org-owned table with its policy; [schema/migrations/001_init.sql](../../schema/migrations/001_init.sql): the `current_*_id()` helpers; [schema/migrations/008_memberships.sql](../../schema/migrations/008_memberships.sql): split read and write policies
- [schema/migrations/018_tenant_lookups.sql](../../schema/migrations/018_tenant_lookups.sql): the definer functions
- [schema/rls_isolation_integration_test.go](../../schema/rls_isolation_integration_test.go), [schema/tenant_policy_guard_test.go](../../schema/tenant_policy_guard_test.go), [schema/lookups_integration_test.go](../../schema/lookups_integration_test.go)
- [schema/schema.go](../../schema/schema.go): `New`, `MigrateUp`, `MigrationsFS`
- [migrator/config.go](../../migrator/config.go): `Config` (`Schema`, `TablePrefix`, `Role`)
- [schema/entity/password_resets.go](../../schema/entity/password_resets.go): `NewPasswordResets`
- [schema/schema_test.go](../../schema/schema_test.go): `TestNew_TargetVersion`
- [schema/migrate_integration_test.go](../../schema/migrate_integration_test.go): `TestMigrations_UpDownUp`
- [schema/project_fk_integration_test.go](../../schema/project_fk_integration_test.go)
- [internal/boot/db.go](../../internal/boot/db.go): `migrate`, `MigrationLockName`
- [schema/README.md](../../schema/README.md)
