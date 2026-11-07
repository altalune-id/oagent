-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}channel_connections (
  id                              UUID PRIMARY KEY,
  public_id                       TEXT NOT NULL UNIQUE,
  org_id                          UUID NOT NULL,
  project_id                      UUID NOT NULL,
  agent_id                        UUID NOT NULL,
  provider                        TEXT NOT NULL,
  status                          TEXT NOT NULL,
  base_url                        TEXT NOT NULL,
  openwa_org                      TEXT NOT NULL,
  openwa_project                  TEXT NOT NULL,
  device_id                       TEXT NOT NULL,
  signing_secret_primary_sealed   BYTEA,
  signing_secret_secondary_sealed BYTEA,
  device_key_sealed               BYTEA NOT NULL,
  quiet_window_ms                 INT NOT NULL DEFAULT 3000,
  quiet_max_ms                    INT NOT NULL DEFAULT 10000,
  reply_failure                   TEXT NOT NULL,
  reply_voice                     TEXT NOT NULL,
  reply_photo_unreadable          TEXT NOT NULL,
  last_result                     TEXT,
  last_result_at                  TIMESTAMPTZ,
  last_bad_signature_at           TIMESTAMPTZ,
  created_at                      TIMESTAMPTZ NOT NULL,
  updated_at                      TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}channel_connections_org_project_id_key UNIQUE (org_id, project_id, id),
  CONSTRAINT {{.TablePrefix}}channel_connections_agent_fkey FOREIGN KEY (org_id, project_id, agent_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}agents (org_id, project_id, id) ON DELETE CASCADE,
  CONSTRAINT {{.TablePrefix}}channel_connections_provider_check CHECK (provider IN ('openwa')),
  CONSTRAINT {{.TablePrefix}}channel_connections_status_check CHECK (status IN ('active', 'disabled')),
  CONSTRAINT {{.TablePrefix}}channel_connections_quiet_window_check CHECK (quiet_window_ms BETWEEN 0 AND 10000),
  CONSTRAINT {{.TablePrefix}}channel_connections_quiet_max_check CHECK (quiet_max_ms >= quiet_window_ms AND quiet_max_ms <= 10000),
  CONSTRAINT {{.TablePrefix}}channel_connections_secondary_needs_primary CHECK (signing_secret_secondary_sealed IS NULL OR signing_secret_primary_sealed IS NOT NULL),
  CONSTRAINT {{.TablePrefix}}channel_connections_last_result_check CHECK (last_result IN ('connected', 'device_mismatch', 'key_refused')),
  CONSTRAINT {{.TablePrefix}}channel_connections_reply_failure_length CHECK (length(reply_failure) BETWEEN 1 AND 1000),
  CONSTRAINT {{.TablePrefix}}channel_connections_reply_voice_length CHECK (length(reply_voice) BETWEEN 1 AND 1000),
  CONSTRAINT {{.TablePrefix}}channel_connections_reply_photo_unreadable_length CHECK (length(reply_photo_unreadable) BETWEEN 1 AND 1000)
);

CREATE INDEX {{.TablePrefix}}channel_connections_project_idx ON {{.Schema}}.{{.TablePrefix}}channel_connections (project_id);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}channel_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}channel_connections FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}channel_connections_tenant ON {{.Schema}}.{{.TablePrefix}}channel_connections
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

CREATE TABLE {{.Schema}}.{{.TablePrefix}}ingest_deliveries (
  org_id        UUID NOT NULL,
  project_id    UUID NOT NULL,
  connection_id UUID NOT NULL,
  delivery_id   TEXT NOT NULL,
  received_at   TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}ingest_deliveries_pkey PRIMARY KEY (connection_id, delivery_id),
  CONSTRAINT {{.TablePrefix}}ingest_deliveries_connection_fkey FOREIGN KEY (org_id, project_id, connection_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}channel_connections (org_id, project_id, id) ON DELETE CASCADE,
  CONSTRAINT {{.TablePrefix}}ingest_deliveries_delivery_id_length CHECK (length(delivery_id) BETWEEN 1 AND 128)
);

CREATE INDEX {{.TablePrefix}}ingest_deliveries_received_at_idx ON {{.Schema}}.{{.TablePrefix}}ingest_deliveries (received_at);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}ingest_deliveries ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}ingest_deliveries FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}ingest_deliveries_tenant ON {{.Schema}}.{{.TablePrefix}}ingest_deliveries
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

ALTER TABLE {{.Schema}}.{{.TablePrefix}}threads
  ADD COLUMN connection_id UUID,
  ADD COLUMN channel_address TEXT,
  ADD COLUMN reply_via TEXT,
  ADD CONSTRAINT {{.TablePrefix}}threads_connection_fkey FOREIGN KEY (org_id, project_id, connection_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}channel_connections (org_id, project_id, id) ON DELETE SET NULL (connection_id),
  ADD CONSTRAINT {{.TablePrefix}}threads_reply_via_check CHECK (reply_via IN ('wa_phone', 'wa_lid')),
  ADD CONSTRAINT {{.TablePrefix}}threads_channel_address_length CHECK (length(channel_address) BETWEEN 1 AND 256),
  DROP CONSTRAINT {{.TablePrefix}}threads_agent_external_key_key;

CREATE UNIQUE INDEX {{.TablePrefix}}threads_agent_external_key_key ON {{.Schema}}.{{.TablePrefix}}threads (agent_id, external_key)
  WHERE kind = 'api';
CREATE UNIQUE INDEX {{.TablePrefix}}threads_connection_external_key_key ON {{.Schema}}.{{.TablePrefix}}threads (connection_id, external_key)
  WHERE connection_id IS NOT NULL;

ALTER TABLE {{.Schema}}.{{.TablePrefix}}messages
  ADD COLUMN channel_time TIMESTAMPTZ,
  ADD COLUMN skip_reason TEXT,
  ADD CONSTRAINT {{.TablePrefix}}messages_skip_reason_check CHECK (skip_reason IN ('stale', 'sender_rate'));

CREATE INDEX {{.TablePrefix}}messages_channel_pending_idx ON {{.Schema}}.{{.TablePrefix}}messages (thread_id, channel_time, seq)
  WHERE status = 'pending';

ALTER TABLE {{.Schema}}.{{.TablePrefix}}end_users
  ADD COLUMN display_name TEXT,
  ADD CONSTRAINT {{.TablePrefix}}end_users_display_name_length CHECK (length(display_name) <= 256);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs
  ADD COLUMN reply_state TEXT,
  ADD COLUMN reply_parts_sent SMALLINT NOT NULL DEFAULT 0,
  ADD COLUMN reply_attempts SMALLINT NOT NULL DEFAULT 0,
  ADD COLUMN reply_next_at TIMESTAMPTZ,
  ADD COLUMN is_group BOOLEAN NOT NULL DEFAULT false,
  ADD CONSTRAINT {{.TablePrefix}}runs_reply_state_check CHECK (reply_state IN ('due', 'sent', 'failed', 'none'));

CREATE INDEX {{.TablePrefix}}runs_reply_due_idx ON {{.Schema}}.{{.TablePrefix}}runs (reply_next_at)
  WHERE reply_state = 'due';

-- SECURITY: resolves a connection for an inbound delivery before any tenant is bound; it returns only the public columns, never a secret, key or URL.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_channel_connection_by_public_id(p_public_id text)
RETURNS TABLE (id uuid, org_id uuid, project_id uuid, agent_id uuid, provider text, device_id text, status text, secured boolean)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT c.id, c.org_id, c.project_id, c.agent_id, c.provider, c.device_id, c.status, c.signing_secret_primary_sealed IS NOT NULL
  FROM {{.Schema}}.{{.TablePrefix}}channel_connections c
  WHERE c.public_id = p_public_id
  LIMIT 1;
$$;

REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_channel_connection_by_public_id(text) FROM PUBLIC;

-- SECURITY: resolves an end user for an operator, who has no active org; it returns only the ids every later read is scoped to.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_end_user_by_public_id(p_public_id text)
RETURNS TABLE (id uuid, org_id uuid, project_id uuid)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT e.id, e.org_id, e.project_id
  FROM {{.Schema}}.{{.TablePrefix}}end_users e
  WHERE e.public_id = p_public_id
  LIMIT 1;
$$;

REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_end_user_by_public_id(text) FROM PUBLIC;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_end_user_by_public_id(text);
DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_channel_connection_by_public_id(text);
DROP INDEX IF EXISTS {{.Schema}}.{{.TablePrefix}}runs_reply_due_idx;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs
  DROP CONSTRAINT IF EXISTS {{.TablePrefix}}runs_reply_state_check,
  DROP COLUMN IF EXISTS is_group,
  DROP COLUMN IF EXISTS reply_next_at,
  DROP COLUMN IF EXISTS reply_attempts,
  DROP COLUMN IF EXISTS reply_parts_sent,
  DROP COLUMN IF EXISTS reply_state;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}end_users
  DROP CONSTRAINT IF EXISTS {{.TablePrefix}}end_users_display_name_length,
  DROP COLUMN IF EXISTS display_name;
DROP INDEX IF EXISTS {{.Schema}}.{{.TablePrefix}}messages_channel_pending_idx;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}messages
  DROP CONSTRAINT IF EXISTS {{.TablePrefix}}messages_skip_reason_check,
  DROP COLUMN IF EXISTS skip_reason,
  DROP COLUMN IF EXISTS channel_time;
DROP INDEX IF EXISTS {{.Schema}}.{{.TablePrefix}}threads_connection_external_key_key;
DROP INDEX IF EXISTS {{.Schema}}.{{.TablePrefix}}threads_agent_external_key_key;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}threads
  DROP CONSTRAINT IF EXISTS {{.TablePrefix}}threads_channel_address_length,
  DROP CONSTRAINT IF EXISTS {{.TablePrefix}}threads_reply_via_check,
  DROP CONSTRAINT IF EXISTS {{.TablePrefix}}threads_connection_fkey,
  DROP COLUMN IF EXISTS reply_via,
  DROP COLUMN IF EXISTS channel_address,
  DROP COLUMN IF EXISTS connection_id,
  ADD CONSTRAINT {{.TablePrefix}}threads_agent_external_key_key UNIQUE (agent_id, external_key);
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}ingest_deliveries;
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}channel_connections;

-- +goose StatementEnd
