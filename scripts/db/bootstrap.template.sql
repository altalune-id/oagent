\set ON_ERROR_STOP on
\set QUIET on
SET client_min_messages = warning;

-- Run by provision.sh inside the application database, in one transaction.
-- @@PREFIX@@ is the role prefix and @@SCHEMA@@ the schema oagent uses (postgres.schema).

DO $$
DECLARE
  caller_super  bool;
  caller_create bool;
BEGIN
  SELECT rolsuper, rolcreaterole INTO caller_super, caller_create
    FROM pg_roles WHERE rolname = current_user;
  IF NOT (caller_super OR caller_create) THEN
    RAISE EXCEPTION 'provisioning needs CREATEROLE or SUPERUSER (current role: %)', current_user;
  END IF;
  IF current_database() IN ('postgres', 'template0', 'template1') THEN
    RAISE EXCEPTION 'run inside the application database, not %', current_database();
  END IF;
END $$;

-- @@PREFIX@@_owner owns the schema and every table; nobody logs in as it.
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '@@PREFIX@@_owner') THEN
    CREATE ROLE @@PREFIX@@_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT;
  END IF;
END $$;

-- psql :'var' substitution does not reach inside DO blocks, so the password statements use \gset + \if.
-- A re-run sets the passwords it is given.
SELECT NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '@@PREFIX@@_migrator') AS need_migrator \gset
\if :need_migrator
CREATE ROLE @@PREFIX@@_migrator LOGIN PASSWORD :'migrator_password' NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT;
\else
ALTER ROLE @@PREFIX@@_migrator LOGIN PASSWORD :'migrator_password';
\endif

SELECT NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '@@PREFIX@@_service') AS need_service \gset
\if :need_service
CREATE ROLE @@PREFIX@@_service LOGIN PASSWORD :'service_password' NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT;
\else
ALTER ROLE @@PREFIX@@_service LOGIN PASSWORD :'service_password';
\endif

-- The migrator runs migrations as the owner (postgres.migrator.role: @@PREFIX@@_owner).
GRANT @@PREFIX@@_owner TO @@PREFIX@@_migrator;

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
GRANT USAGE ON SCHEMA @@SCHEMA@@ TO @@PREFIX@@_service;

-- Tables and sequences the owner creates later are readable and writable by the service.
ALTER DEFAULT PRIVILEGES FOR ROLE @@PREFIX@@_owner IN SCHEMA @@SCHEMA@@
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO @@PREFIX@@_service;
ALTER DEFAULT PRIVILEGES FOR ROLE @@PREFIX@@_owner IN SCHEMA @@SCHEMA@@
  GRANT USAGE, SELECT ON SEQUENCES TO @@PREFIX@@_service;
ALTER DEFAULT PRIVILEGES FOR ROLE @@PREFIX@@_owner IN SCHEMA @@SCHEMA@@
  GRANT EXECUTE ON FUNCTIONS TO @@PREFIX@@_service;

-- Tables that already exist (a re-run after migrations) get the same grants.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA @@SCHEMA@@ TO @@PREFIX@@_service;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA @@SCHEMA@@ TO @@PREFIX@@_service;

DO $$ BEGIN
  EXECUTE format('REVOKE @@PREFIX@@_owner FROM %I', current_user);
END $$;
