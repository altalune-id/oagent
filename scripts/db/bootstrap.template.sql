\set ON_ERROR_STOP on
\set QUIET on
SET client_min_messages = warning;

-- Run by provision.sh inside the application database, in one transaction.
-- @@PREFIX@@ is the role prefix, @@SCHEMA@@ the schema (postgres.schema) and @@TABLE_PREFIX@@ the table prefix (postgres.tablePrefix).

DO $$
DECLARE
  caller_super  bool;
  caller_create bool;
  caller_bypass bool;
BEGIN
  SELECT rolsuper, rolcreaterole, rolbypassrls INTO caller_super, caller_create, caller_bypass
    FROM pg_roles WHERE rolname = current_user;
  IF NOT (caller_super OR caller_create) THEN
    RAISE EXCEPTION 'provisioning needs CREATEROLE or SUPERUSER (current role: %)', current_user;
  END IF;
  IF NOT (caller_super OR caller_bypass)
     AND NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '@@PREFIX@@_owner' AND rolbypassrls) THEN
    RAISE EXCEPTION 'provisioning needs BYPASSRLS or SUPERUSER to give it to @@PREFIX@@_owner (current role: %)', current_user
      USING HINT = 'Have a superuser run: CREATE ROLE @@PREFIX@@_owner NOLOGIN BYPASSRLS; then re-run.';
  END IF;
  IF current_database() IN ('postgres', 'template0', 'template1') THEN
    RAISE EXCEPTION 'run inside the application database, not %', current_database();
  END IF;
END $$;

-- @@PREFIX@@_owner owns the schema, every table and every SECURITY DEFINER function; nobody logs in as it.
-- SECURITY: BYPASSRLS lets the definer functions it owns read across tenants. It is a role attribute, so membership
-- does not pass it on; only SET ROLE (the migrator) or a definer function runs with it.
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '@@PREFIX@@_owner') THEN
    CREATE ROLE @@PREFIX@@_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT BYPASSRLS;
  ELSE
    ALTER ROLE @@PREFIX@@_owner NOLOGIN NOSUPERUSER BYPASSRLS;
  END IF;
END $$;

-- psql :'var' substitution does not reach inside DO blocks, so the password statements use \gset + \if.
-- A re-run sets the passwords it is given.
SELECT NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '@@PREFIX@@_migrator') AS need_migrator \gset
\if :need_migrator
CREATE ROLE @@PREFIX@@_migrator LOGIN PASSWORD :'migrator_password' NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS INHERIT;
\else
ALTER ROLE @@PREFIX@@_migrator LOGIN PASSWORD :'migrator_password' NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
\endif

SELECT NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '@@PREFIX@@_service') AS need_service \gset
\if :need_service
CREATE ROLE @@PREFIX@@_service LOGIN PASSWORD :'service_password' NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS INHERIT;
\else
ALTER ROLE @@PREFIX@@_service LOGIN PASSWORD :'service_password' NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
\endif

-- The migrator runs migrations as the owner (postgres.migrator.role: @@PREFIX@@_owner).
GRANT @@PREFIX@@_owner TO @@PREFIX@@_migrator;

-- SECURITY: the service must not reach the owner by SET ROLE, or RLS stops applying to it; it holds no membership at all.
DO $$
DECLARE
  grp text;
BEGIN
  FOR grp IN
    SELECT g.rolname FROM pg_auth_members am
      JOIN pg_roles g ON g.oid = am.roleid JOIN pg_roles m ON m.oid = am.member
     WHERE m.rolname = '@@PREFIX@@_service'
  LOOP
    EXECUTE format('REVOKE %I FROM @@PREFIX@@_service', grp);
  END LOOP;
END $$;

DO $$ BEGIN
  EXECUTE format('GRANT CONNECT ON DATABASE %I TO @@PREFIX@@_migrator, @@PREFIX@@_service', current_database());
  EXECUTE format('REVOKE ALL ON DATABASE %I FROM PUBLIC', current_database());
END $$;

-- The caller needs owner membership to hand the schema over.
DO $$ BEGIN
  EXECUTE format('GRANT @@PREFIX@@_owner TO %I', current_user);
END $$;

CREATE SCHEMA IF NOT EXISTS @@SCHEMA@@ AUTHORIZATION @@PREFIX@@_owner;
ALTER SCHEMA @@SCHEMA@@ OWNER TO @@PREFIX@@_owner;
REVOKE CREATE ON SCHEMA @@SCHEMA@@ FROM PUBLIC;
REVOKE CREATE ON SCHEMA @@SCHEMA@@ FROM @@PREFIX@@_service;
GRANT USAGE ON SCHEMA @@SCHEMA@@ TO @@PREFIX@@_service;
DO $$ BEGIN
  EXECUTE format('REVOKE CREATE, TEMP ON DATABASE %I FROM @@PREFIX@@_service', current_database());
END $$;

-- Tables and sequences the owner creates later are readable and writable by the service.
ALTER DEFAULT PRIVILEGES FOR ROLE @@PREFIX@@_owner IN SCHEMA @@SCHEMA@@
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO @@PREFIX@@_service;
ALTER DEFAULT PRIVILEGES FOR ROLE @@PREFIX@@_owner IN SCHEMA @@SCHEMA@@
  GRANT USAGE, SELECT ON SEQUENCES TO @@PREFIX@@_service;
ALTER DEFAULT PRIVILEGES FOR ROLE @@PREFIX@@_owner IN SCHEMA @@SCHEMA@@
  GRANT EXECUTE ON FUNCTIONS TO @@PREFIX@@_service;
-- NOTE: Postgres grants EXECUTE to PUBLIC on every new function; a per-schema default cannot revoke a global one, so this one is global.
ALTER DEFAULT PRIVILEGES FOR ROLE @@PREFIX@@_owner
  REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC;

-- Tables that already exist (a re-run after migrations) get the same grants.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA @@SCHEMA@@ TO @@PREFIX@@_service;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA @@SCHEMA@@ TO @@PREFIX@@_service;

-- goose's version table, created here with goose's own DDL and zero row so the service can read it but never write it.
SET LOCAL ROLE @@PREFIX@@_owner;
CREATE TABLE IF NOT EXISTS @@SCHEMA@@.@@TABLE_PREFIX@@goose_db_version (
  id integer PRIMARY KEY GENERATED BY DEFAULT AS IDENTITY,
  version_id bigint NOT NULL,
  is_applied boolean NOT NULL,
  tstamp timestamp NOT NULL DEFAULT now()
);
INSERT INTO @@SCHEMA@@.@@TABLE_PREFIX@@goose_db_version (version_id, is_applied)
  SELECT 0, true WHERE NOT EXISTS (SELECT 1 FROM @@SCHEMA@@.@@TABLE_PREFIX@@goose_db_version);
RESET ROLE;
REVOKE ALL ON @@SCHEMA@@.@@TABLE_PREFIX@@goose_db_version FROM @@PREFIX@@_service;
GRANT SELECT ON @@SCHEMA@@.@@TABLE_PREFIX@@goose_db_version TO @@PREFIX@@_service;
DO $$ BEGIN
  EXECUTE format('REVOKE ALL ON SEQUENCE %s FROM @@PREFIX@@_service',
    pg_get_serial_sequence('@@SCHEMA@@.@@TABLE_PREFIX@@goose_db_version', 'id'));
END $$;

-- Functions that already exist (a re-run after migrations): EXECUTE for the service only.
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA @@SCHEMA@@ FROM PUBLIC;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA @@SCHEMA@@ TO @@PREFIX@@_service;

DO $$ BEGIN
  EXECUTE format('REVOKE @@PREFIX@@_owner FROM %I', current_user);
END $$;
