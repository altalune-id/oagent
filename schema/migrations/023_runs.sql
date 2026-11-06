-- +goose Up
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}agent_versions
  ADD CONSTRAINT {{.TablePrefix}}agent_versions_org_project_id_key UNIQUE (org_id, project_id, id);

CREATE TABLE {{.Schema}}.{{.TablePrefix}}threads (
  id               UUID PRIMARY KEY,
  public_id        TEXT NOT NULL UNIQUE,
  org_id           UUID NOT NULL,
  project_id       UUID NOT NULL,
  agent_id         UUID NOT NULL,
  kind             TEXT NOT NULL,
  external_key     TEXT NOT NULL,
  lease_owner      TEXT,
  lease_until      TIMESTAMPTZ,
  cancel_requested BOOLEAN NOT NULL DEFAULT false,
  next_seq         BIGINT NOT NULL DEFAULT 1,
  created_at       TIMESTAMPTZ NOT NULL,
  updated_at       TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}threads_kind_check CHECK (kind IN ('dm', 'group', 'api')),
  CONSTRAINT {{.TablePrefix}}threads_lease_shape CHECK ((lease_owner IS NULL) = (lease_until IS NULL)),
  CONSTRAINT {{.TablePrefix}}threads_next_seq_positive CHECK (next_seq >= 1),
  CONSTRAINT {{.TablePrefix}}threads_agent_external_key_key UNIQUE (agent_id, external_key),
  CONSTRAINT {{.TablePrefix}}threads_org_project_id_key UNIQUE (org_id, project_id, id),
  CONSTRAINT {{.TablePrefix}}threads_agent_fkey FOREIGN KEY (org_id, project_id, agent_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}agents (org_id, project_id, id) ON DELETE CASCADE
);

CREATE INDEX {{.TablePrefix}}threads_lease_until_idx ON {{.Schema}}.{{.TablePrefix}}threads (lease_until);

CREATE TABLE {{.Schema}}.{{.TablePrefix}}runs (
  id                       UUID PRIMARY KEY,
  public_id                TEXT NOT NULL UNIQUE,
  org_id                   UUID NOT NULL,
  project_id               UUID NOT NULL,
  thread_id                UUID NOT NULL,
  agent_version_id         UUID NOT NULL,
  status                   TEXT NOT NULL,
  idempotency_key          TEXT,
  attempt                  INT NOT NULL DEFAULT 1,
  input_tokens             BIGINT NOT NULL DEFAULT 0,
  output_tokens            BIGINT NOT NULL DEFAULT 0,
  cache_read_tokens        BIGINT NOT NULL DEFAULT 0,
  cache_write_tokens       BIGINT NOT NULL DEFAULT 0,
  usage_estimated          BOOLEAN NOT NULL DEFAULT false,
  price_input_micros       BIGINT,
  price_output_micros      BIGINT,
  price_cache_read_micros  BIGINT,
  price_cache_write_micros BIGINT,
  cost_micros              BIGINT,
  latency_ms               INT,
  error_code               TEXT,
  started_at               TIMESTAMPTZ NOT NULL,
  ended_at                 TIMESTAMPTZ,
  created_at               TIMESTAMPTZ NOT NULL,
  updated_at               TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}runs_status_check CHECK (status IN ('running', 'awaiting_approval', 'completed', 'failed', 'cancelled', 'interrupted', 'limit_reached', 'expired')),
  CONSTRAINT {{.TablePrefix}}runs_attempt_positive CHECK (attempt >= 1),
  CONSTRAINT {{.TablePrefix}}runs_org_project_id_key UNIQUE (org_id, project_id, id),
  CONSTRAINT {{.TablePrefix}}runs_thread_fkey FOREIGN KEY (org_id, project_id, thread_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}threads (org_id, project_id, id) ON DELETE CASCADE,
  CONSTRAINT {{.TablePrefix}}runs_agent_version_fkey FOREIGN KEY (org_id, project_id, agent_version_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}agent_versions (org_id, project_id, id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX {{.TablePrefix}}runs_thread_idempotency_key ON {{.Schema}}.{{.TablePrefix}}runs (thread_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX {{.TablePrefix}}runs_one_open_per_thread ON {{.Schema}}.{{.TablePrefix}}runs (thread_id)
  WHERE status IN ('running', 'interrupted', 'awaiting_approval');

CREATE TABLE {{.Schema}}.{{.TablePrefix}}messages (
  id                  UUID PRIMARY KEY,
  org_id              UUID NOT NULL,
  project_id          UUID NOT NULL,
  thread_id           UUID NOT NULL,
  seq                 BIGINT NOT NULL,
  role                TEXT NOT NULL,
  body                JSONB NOT NULL,
  speaker_id          UUID,
  status              TEXT NOT NULL,
  external_message_id TEXT,
  run_id              UUID,
  tool_call_id        TEXT,
  created_at          TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}messages_role_check CHECK (role IN ('user', 'assistant', 'tool', 'system')),
  CONSTRAINT {{.TablePrefix}}messages_body_object CHECK (jsonb_typeof(body) = 'object'),
  CONSTRAINT {{.TablePrefix}}messages_status_check CHECK (status IN ('pending', 'consumed')),
  CONSTRAINT {{.TablePrefix}}messages_thread_seq_key UNIQUE (thread_id, seq),
  CONSTRAINT {{.TablePrefix}}messages_thread_fkey FOREIGN KEY (org_id, project_id, thread_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}threads (org_id, project_id, id) ON DELETE CASCADE,
  CONSTRAINT {{.TablePrefix}}messages_speaker_fkey FOREIGN KEY (org_id, project_id, speaker_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}end_users (org_id, project_id, id) ON DELETE SET NULL (speaker_id),
  CONSTRAINT {{.TablePrefix}}messages_run_fkey FOREIGN KEY (org_id, project_id, run_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}runs (org_id, project_id, id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX {{.TablePrefix}}messages_thread_external_message_id_key ON {{.Schema}}.{{.TablePrefix}}messages (thread_id, external_message_id)
  WHERE external_message_id IS NOT NULL;
CREATE UNIQUE INDEX {{.TablePrefix}}messages_run_tool_call_id_key ON {{.Schema}}.{{.TablePrefix}}messages (run_id, tool_call_id)
  WHERE tool_call_id IS NOT NULL;
CREATE INDEX {{.TablePrefix}}messages_pending_idx ON {{.Schema}}.{{.TablePrefix}}messages (thread_id)
  WHERE status = 'pending';

CREATE TABLE {{.Schema}}.{{.TablePrefix}}thread_snapshots (
  thread_id        UUID PRIMARY KEY,
  org_id           UUID NOT NULL,
  project_id       UUID NOT NULL,
  agent_version_id UUID NOT NULL,
  sdk_version      TEXT NOT NULL,
  blob             BYTEA NOT NULL,
  updated_at       TIMESTAMPTZ NOT NULL,
  CONSTRAINT {{.TablePrefix}}thread_snapshots_thread_fkey FOREIGN KEY (org_id, project_id, thread_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}threads (org_id, project_id, id) ON DELETE CASCADE,
  CONSTRAINT {{.TablePrefix}}thread_snapshots_agent_version_fkey FOREIGN KEY (org_id, project_id, agent_version_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}agent_versions (org_id, project_id, id) ON DELETE CASCADE
);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}threads ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}threads FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}threads_tenant ON {{.Schema}}.{{.TablePrefix}}threads
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}runs_tenant ON {{.Schema}}.{{.TablePrefix}}runs
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

ALTER TABLE {{.Schema}}.{{.TablePrefix}}messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}messages FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}messages_tenant ON {{.Schema}}.{{.TablePrefix}}messages
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

ALTER TABLE {{.Schema}}.{{.TablePrefix}}thread_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}thread_snapshots FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}thread_snapshots_tenant ON {{.Schema}}.{{.TablePrefix}}thread_snapshots
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}thread_snapshots;
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}messages;
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}runs;
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}threads;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}agent_versions DROP CONSTRAINT IF EXISTS {{.TablePrefix}}agent_versions_org_project_id_key;

-- +goose StatementEnd
