-- +goose Up
-- +goose StatementBegin

CREATE TABLE {{.Schema}}.{{.TablePrefix}}approvals (
  id                    UUID PRIMARY KEY,
  public_id             TEXT NOT NULL UNIQUE,
  org_id                UUID NOT NULL,
  project_id            UUID NOT NULL,
  agent_id              UUID NOT NULL,
  run_id                UUID NOT NULL,
  thread_id             UUID NOT NULL,
  calls                 JSONB NOT NULL,
  preview               TEXT NOT NULL,
  channel               TEXT NOT NULL,
  requester_end_user_id UUID,
  prompt_external_id    TEXT,
  -- SECURITY: the SHA-256 of the snapshot the interrupt was raised in; a resume against any other snapshot ends the run expired.
  raised_snapshot_hash  BYTEA NOT NULL,
  status                TEXT NOT NULL,
  decided_by_kind       TEXT,
  decided_by_id         TEXT,
  decided_via           TEXT,
  decision_text         TEXT,
  expires_at            TIMESTAMPTZ NOT NULL,
  created_at            TIMESTAMPTZ NOT NULL,
  decided_at            TIMESTAMPTZ,
  CONSTRAINT {{.TablePrefix}}approvals_calls_shape CHECK (jsonb_typeof(calls) = 'array' AND jsonb_array_length(calls) BETWEEN 1 AND 8),
  CONSTRAINT {{.TablePrefix}}approvals_channel_check CHECK (channel IN ('whatsapp', 'api', 'console')),
  CONSTRAINT {{.TablePrefix}}approvals_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'expired')),
  CONSTRAINT {{.TablePrefix}}approvals_decided_shape CHECK ((status = 'pending') = (decided_at IS NULL)),
  CONSTRAINT {{.TablePrefix}}approvals_decided_by_kind_check CHECK (decided_by_kind IN ('end_user', 'api_key', 'user', 'system')),
  CONSTRAINT {{.TablePrefix}}approvals_decided_via_check CHECK (decided_via IN ('console', 'mcp', 'whatsapp', 'api')),
  CONSTRAINT {{.TablePrefix}}approvals_decision_text_length CHECK (char_length(decision_text) <= 4000),
  CONSTRAINT {{.TablePrefix}}approvals_run_fkey FOREIGN KEY (org_id, project_id, run_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}runs (org_id, project_id, id) ON DELETE CASCADE,
  CONSTRAINT {{.TablePrefix}}approvals_thread_fkey FOREIGN KEY (org_id, project_id, thread_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}threads (org_id, project_id, id) ON DELETE CASCADE,
  CONSTRAINT {{.TablePrefix}}approvals_agent_fkey FOREIGN KEY (org_id, project_id, agent_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}agents (org_id, project_id, id) ON DELETE CASCADE,
  CONSTRAINT {{.TablePrefix}}approvals_requester_fkey FOREIGN KEY (org_id, project_id, requester_end_user_id)
    REFERENCES {{.Schema}}.{{.TablePrefix}}end_users (org_id, project_id, id) ON DELETE SET NULL (requester_end_user_id)
);

CREATE UNIQUE INDEX {{.TablePrefix}}approvals_thread_pending_key ON {{.Schema}}.{{.TablePrefix}}approvals (thread_id)
  WHERE status = 'pending';
CREATE INDEX {{.TablePrefix}}approvals_expiry_idx ON {{.Schema}}.{{.TablePrefix}}approvals (expires_at)
  WHERE status = 'pending';
CREATE INDEX {{.TablePrefix}}approvals_project_created_idx ON {{.Schema}}.{{.TablePrefix}}approvals (project_id, created_at DESC, public_id DESC);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}approvals ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}approvals FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}approvals_tenant ON {{.Schema}}.{{.TablePrefix}}approvals
  FOR ALL
  USING (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id())
  WITH CHECK (org_id = {{.Schema}}.{{.TablePrefix}}current_org_id());

-- NOTE: existing connections get the English texts, then the defaults go so Go writes every new row's text.
ALTER TABLE {{.Schema}}.{{.TablePrefix}}channel_connections
  ADD COLUMN approval_prompt TEXT NOT NULL DEFAULT E'{preview}\n\nReply YES or NO',
  ADD COLUMN approval_expired TEXT NOT NULL DEFAULT 'This request expired. Please ask again.';
ALTER TABLE {{.Schema}}.{{.TablePrefix}}channel_connections
  ALTER COLUMN approval_prompt DROP DEFAULT,
  ALTER COLUMN approval_expired DROP DEFAULT,
  ADD CONSTRAINT {{.TablePrefix}}channel_connections_approval_prompt_length CHECK (length(approval_prompt) BETWEEN 1 AND 1000),
  ADD CONSTRAINT {{.TablePrefix}}channel_connections_approval_expired_length CHECK (length(approval_expired) BETWEEN 1 AND 1000);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs
  ADD COLUMN reply_kind TEXT,
  ADD CONSTRAINT {{.TablePrefix}}runs_reply_kind_check CHECK (reply_kind IN ('reply', 'prompt', 'expired'));

-- SECURITY: resolves an approval by public id before reach is checked; it returns ids only.
CREATE FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_approval_by_public_id(p_public_id text)
RETURNS TABLE (id uuid, org_id uuid, project_id uuid, agent_id uuid)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = {{.Schema}}, pg_catalog, pg_temp
AS $$
  SELECT a.id, a.org_id, a.project_id, a.agent_id
  FROM {{.Schema}}.{{.TablePrefix}}approvals a
  WHERE a.public_id = p_public_id
  LIMIT 1;
$$;

REVOKE ALL ON FUNCTION {{.Schema}}.{{.TablePrefix}}resolve_approval_by_public_id(text) FROM PUBLIC;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP FUNCTION IF EXISTS {{.Schema}}.{{.TablePrefix}}resolve_approval_by_public_id(text);
ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs
  DROP CONSTRAINT IF EXISTS {{.TablePrefix}}runs_reply_kind_check,
  DROP COLUMN IF EXISTS reply_kind;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}channel_connections
  DROP CONSTRAINT IF EXISTS {{.TablePrefix}}channel_connections_approval_expired_length,
  DROP CONSTRAINT IF EXISTS {{.TablePrefix}}channel_connections_approval_prompt_length,
  DROP COLUMN IF EXISTS approval_expired,
  DROP COLUMN IF EXISTS approval_prompt;
DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}approvals;

-- +goose StatementEnd
