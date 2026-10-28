\set ON_ERROR_STOP on
\set QUIET on
SET client_min_messages = notice;

-- Run by provision.sh after bootstrap, and again after migrations; raises on any role or privilege drift.
-- @@PREFIX@@ is the role prefix, @@SCHEMA@@ the schema and @@TABLE_PREFIX@@ the table prefix oagent uses.

DO $$
DECLARE
  owner_r    text := '@@PREFIX@@_owner';
  migrator_r text := '@@PREFIX@@_migrator';
  service_r  text := '@@PREFIX@@_service';
  schema_n   text := '@@SCHEMA@@';
  goose_t    text := '@@SCHEMA@@.@@TABLE_PREFIX@@goose_db_version';
  problems   text[] := '{}';
  r          record;
  offenders  text;
BEGIN
  SELECT rolcanlogin, rolbypassrls, rolsuper INTO r FROM pg_roles WHERE rolname = owner_r;
  IF NOT FOUND THEN
    problems := problems || format('%s does not exist', owner_r);
  ELSE
    IF r.rolcanlogin THEN problems := problems || format('%s can log in', owner_r); END IF;
    IF NOT r.rolbypassrls THEN problems := problems || format('%s lacks BYPASSRLS', owner_r); END IF;
    IF r.rolsuper THEN problems := problems || format('%s is a superuser', owner_r); END IF;
  END IF;

  SELECT rolcanlogin, rolbypassrls, rolsuper INTO r FROM pg_roles WHERE rolname = migrator_r;
  IF NOT FOUND THEN
    problems := problems || format('%s does not exist', migrator_r);
  ELSE
    IF NOT r.rolcanlogin THEN problems := problems || format('%s cannot log in', migrator_r); END IF;
    IF r.rolbypassrls THEN problems := problems || format('%s has BYPASSRLS', migrator_r); END IF;
    IF r.rolsuper THEN problems := problems || format('%s is a superuser', migrator_r); END IF;
    IF NOT pg_has_role(migrator_r, owner_r, 'MEMBER') THEN
      problems := problems || format('%s is not a member of %s', migrator_r, owner_r);
    END IF;
  END IF;

  SELECT rolcanlogin, rolbypassrls, rolsuper, rolcreaterole, rolcreatedb INTO r FROM pg_roles WHERE rolname = service_r;
  IF NOT FOUND THEN
    problems := problems || format('%s does not exist', service_r);
  ELSE
    IF NOT r.rolcanlogin THEN problems := problems || format('%s cannot log in', service_r); END IF;
    IF r.rolbypassrls THEN problems := problems || format('%s has BYPASSRLS', service_r); END IF;
    IF r.rolsuper OR r.rolcreaterole OR r.rolcreatedb THEN
      problems := problems || format('%s has SUPERUSER, CREATEROLE or CREATEDB', service_r);
    END IF;
    SELECT string_agg(g.rolname, ', ') INTO offenders
      FROM pg_auth_members am JOIN pg_roles g ON g.oid = am.roleid JOIN pg_roles m ON m.oid = am.member
     WHERE m.rolname = service_r;
    IF offenders IS NOT NULL THEN
      problems := problems || format('%s is a member of %s', service_r, offenders);
    END IF;
    SELECT string_agg(c.relname, ', ') INTO offenders
      FROM pg_class c JOIN pg_roles o ON o.oid = c.relowner WHERE o.rolname = service_r;
    IF offenders IS NOT NULL THEN
      problems := problems || format('%s owns relations: %s', service_r, offenders);
    END IF;
    SELECT string_agg(p.proname, ', ') INTO offenders
      FROM pg_proc p JOIN pg_roles o ON o.oid = p.proowner WHERE o.rolname = service_r;
    IF offenders IS NOT NULL THEN
      problems := problems || format('%s owns functions: %s', service_r, offenders);
    END IF;
    IF has_database_privilege(service_r, current_database(), 'CREATE')
       OR has_database_privilege(service_r, current_database(), 'TEMP') THEN
      problems := problems || format('%s has CREATE or TEMP on database %s', service_r, current_database());
    END IF;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = schema_n) THEN
    problems := problems || format('schema %s does not exist', schema_n);
  ELSE
    IF (SELECT r2.rolname FROM pg_namespace n JOIN pg_roles r2 ON r2.oid = n.nspowner WHERE n.nspname = schema_n) <> owner_r THEN
      problems := problems || format('schema %s is not owned by %s', schema_n, owner_r);
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = service_r)
       AND has_schema_privilege(service_r, schema_n, 'CREATE') THEN
      problems := problems || format('%s can create objects in schema %s', service_r, schema_n);
    END IF;

    SELECT string_agg(format('%s(%s)', c.relname, o.rolname), ', ') INTO offenders
      FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace JOIN pg_roles o ON o.oid = c.relowner
     WHERE n.nspname = schema_n AND c.relkind IN ('r', 'p', 'S', 'v', 'm') AND o.rolname <> owner_r;
    IF offenders IS NOT NULL THEN
      problems := problems || format('not owned by %s: %s (migrations must run as postgres.migrator.role: %s)', owner_r, offenders, owner_r);
    END IF;

    SELECT string_agg(format('%s(%s)', p.proname, o.rolname), ', ') INTO offenders
      FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace JOIN pg_roles o ON o.oid = p.proowner
     WHERE n.nspname = schema_n AND o.rolname <> owner_r;
    IF offenders IS NOT NULL THEN
      problems := problems || format('functions not owned by %s: %s', owner_r, offenders);
    END IF;

    SELECT string_agg(p.oid::regprocedure::text, ', ') INTO offenders
      FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
     WHERE n.nspname = schema_n AND has_function_privilege('public', p.oid, 'EXECUTE');
    IF offenders IS NOT NULL THEN
      problems := problems || format('PUBLIC can execute: %s', offenders);
    END IF;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_default_acl d JOIN pg_roles g ON g.oid = d.defaclrole
     WHERE g.rolname = owner_r AND d.defaclnamespace = 0 AND d.defaclobjtype = 'f'
       AND NOT EXISTS (SELECT 1 FROM aclexplode(d.defaclacl) a WHERE a.grantee = 0 AND a.privilege_type = 'EXECUTE')
  ) THEN
    problems := problems || format('functions %s creates are executable by PUBLIC by default', owner_r);
  END IF;

  FOR r IN SELECT * FROM (VALUES ('r', 'SELECT'), ('r', 'INSERT'), ('r', 'UPDATE'), ('r', 'DELETE'),
                                  ('S', 'USAGE'), ('f', 'EXECUTE')) AS want(objtype, priv)
  LOOP
    IF NOT EXISTS (
      SELECT 1 FROM pg_default_acl d
        JOIN pg_roles g ON g.oid = d.defaclrole
        JOIN pg_namespace n ON n.oid = d.defaclnamespace
        CROSS JOIN LATERAL aclexplode(d.defaclacl) a
        JOIN pg_roles s ON s.oid = a.grantee
       WHERE g.rolname = owner_r AND n.nspname = schema_n AND d.defaclobjtype = r.objtype::"char"
         AND s.rolname = service_r AND a.privilege_type = r.priv
    ) THEN
      problems := problems || format('no default %s for %s on objects %s creates in %s (type %s)', r.priv, service_r, owner_r, schema_n, r.objtype);
    END IF;
  END LOOP;

  IF to_regclass(goose_t) IS NULL THEN
    problems := problems || format('%s does not exist', goose_t);
  ELSIF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = service_r) THEN
    IF NOT has_table_privilege(service_r, goose_t, 'SELECT') THEN
      problems := problems || format('%s cannot read %s', service_r, goose_t);
    END IF;
    IF has_table_privilege(service_r, goose_t, 'INSERT, UPDATE, DELETE, TRUNCATE') THEN
      problems := problems || format('%s can write %s', service_r, goose_t);
    END IF;
  END IF;

  IF cardinality(problems) > 0 THEN
    RAISE EXCEPTION E'verify: % problem(s):\n  - %', cardinality(problems), array_to_string(problems, E'\n  - ')
      USING HINT = 'Re-run scripts/db/provision.sh with the same PREFIX, SCHEMA and TABLE_PREFIX.';
  END IF;
  RAISE NOTICE 'verify: OK (%, %, % on %.%)', owner_r, migrator_r, service_r, current_database(), schema_n;
END $$;
