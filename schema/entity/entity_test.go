package entity_test

import (
	"reflect"
	"testing"

	jetpg "github.com/go-jet/jet/v2/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/schema/entity"
)

type binding struct {
	table   jetpg.Table
	columns jetpg.ColumnList
}

func TestBindings(t *testing.T) {
	cases := []struct {
		name    string
		build   func(schema, prefix string) binding
		columns []string
	}{
		{
			"agents",
			func(s, p string) binding { b := entity.NewAgents(s, p); return binding{b, b.AllColumns} },
			[]string{
				"id", "public_id", "org_id", "project_id", "slug", "name", "description", "created_by", "created_at", "updated_at",
				"draft_config", "draft_updated_at", "published_version", "published_at",
			},
		},
		{
			"agent_versions",
			func(s, p string) binding { b := entity.NewAgentVersions(s, p); return binding{b, b.AllColumns} },
			[]string{"id", "public_id", "org_id", "project_id", "agent_id", "version", "config", "created_by", "created_at", "kind", "config_hash"},
		},
		{
			"api_key_projects",
			func(s, p string) binding { b := entity.NewAPIKeyProjects(s, p); return binding{b, b.AllColumns} },
			[]string{"org_id", "key_id", "project_id", "created_at"},
		},
		{
			"api_keys",
			func(s, p string) binding { b := entity.NewAPIKeys(s, p); return binding{b, b.AllColumns} },
			[]string{
				"id", "org_id", "project_id", "kind", "all_projects", "name", "secret_hash", "secret_hint",
				"scopes", "resource_ids", "created_by", "created_at", "expires_at", "revoked_at", "last_used_at",
			},
		},
		{
			"channel_connections",
			func(s, p string) binding { b := entity.NewChannelConnections(s, p); return binding{b, b.AllColumns} },
			[]string{
				"id", "public_id", "org_id", "project_id", "agent_id", "provider", "status", "base_url", "openwa_org", "openwa_project", "device_id",
				"signing_secret_primary_sealed", "signing_secret_secondary_sealed", "device_key_sealed", "quiet_window_ms", "quiet_max_ms",
				"reply_failure", "reply_voice", "reply_photo_unreadable", "last_result", "last_result_at", "last_bad_signature_at",
				"created_at", "updated_at",
			},
		},
		{
			"ingest_deliveries",
			func(s, p string) binding { b := entity.NewIngestDeliveries(s, p); return binding{b, b.AllColumns} },
			[]string{"org_id", "project_id", "connection_id", "delivery_id", "received_at"},
		},
		{
			"threads",
			func(s, p string) binding { b := entity.NewThreads(s, p); return binding{b, b.AllColumns} },
			[]string{
				"id", "public_id", "org_id", "project_id", "agent_id", "kind", "external_key", "lease_owner", "lease_until", "cancel_requested", "next_seq", "created_at", "updated_at", "actor_end_user_id",
				"connection_id", "channel_address", "reply_via",
			},
		},
		{
			"runs",
			func(s, p string) binding { b := entity.NewRuns(s, p); return binding{b, b.AllColumns} },
			[]string{
				"id", "public_id", "org_id", "project_id", "thread_id", "agent_version_id", "status", "idempotency_key", "attempt", "input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens", "usage_estimated", "price_input_micros", "price_output_micros", "price_cache_read_micros", "price_cache_write_micros", "cost_micros", "latency_ms", "error_code", "started_at", "ended_at", "created_at", "updated_at",
				"reply_state", "reply_parts_sent", "reply_attempts", "reply_next_at", "is_group",
			},
		},
		{
			"messages",
			func(s, p string) binding { b := entity.NewMessages(s, p); return binding{b, b.AllColumns} },
			[]string{
				"id", "org_id", "project_id", "thread_id", "seq", "role", "body", "speaker_id", "status", "external_message_id", "run_id", "tool_call_id", "created_at", "request_hash",
				"channel_time", "skip_reason",
			},
		},
		{
			"thread_snapshots",
			func(s, p string) binding { b := entity.NewThreadSnapshots(s, p); return binding{b, b.AllColumns} },
			[]string{
				"thread_id", "org_id", "project_id", "agent_version_id", "sdk_version", "blob", "updated_at",
			},
		},
		{
			"end_users",
			func(s, p string) binding { b := entity.NewEndUsers(s, p); return binding{b, b.AllColumns} },
			[]string{"id", "public_id", "org_id", "project_id", "created_at", "updated_at", "display_name"},
		},
		{
			"end_user_identities",
			func(s, p string) binding { b := entity.NewEndUserIdentities(s, p); return binding{b, b.AllColumns} },
			[]string{"id", "org_id", "project_id", "end_user_id", "kind", "value", "created_at"},
		},
		{
			"invites",
			func(s, p string) binding { b := entity.NewInvites(s, p); return binding{b, b.AllColumns} },
			[]string{
				"id", "public_id", "org_id", "email", "role", "token_hash", "draft", "email_only", "invited_by",
				"expires_at", "accepted_at", "accepted_by", "revoked_at", "created_at", "updated_at",
			},
		},
		{
			"legal_documents",
			func(s, p string) binding { b := entity.NewLegalDocuments(s, p); return binding{b, b.AllColumns} },
			[]string{"id", "slug", "version", "title", "body", "sha256", "published_at"},
		},
		{
			"mail_queue",
			func(s, p string) binding { b := entity.NewMailQueue(s, p); return binding{b, b.AllColumns} },
			[]string{"id", "payload", "sealed", "status", "attempt", "next_attempt_at", "last_error", "created_at", "settled_at"},
		},
		{
			"memberships",
			func(s, p string) binding { b := entity.NewMemberships(s, p); return binding{b, b.AllColumns} },
			[]string{"id", "public_id", "org_id", "user_id", "role", "created_at"},
		},
		{
			"mcp_servers",
			func(s, p string) binding { b := entity.NewMCPServers(s, p); return binding{b, b.AllColumns} },
			[]string{
				"id", "public_id", "org_id", "project_id", "name", "url", "auth_kind", "auth_header", "auth_sealed",
				"timeout_ms", "trusted", "created_at", "updated_at",
			},
		},
		{
			"model_providers",
			func(s, p string) binding { b := entity.NewModelProviders(s, p); return binding{b, b.AllColumns} },
			[]string{
				"id", "public_id", "org_id", "project_id", "name", "kind", "base_url", "api_key_sealed", "models",
				"created_at", "updated_at",
			},
		},
		{
			"orgs",
			func(s, p string) binding { b := entity.NewOrgs(s, p); return binding{b, b.AllColumns} },
			[]string{"id", "public_id", "slug", "name", "created_by", "created_at", "updated_at", "is_system"},
		},
		{
			"outbox_entries",
			func(s, p string) binding { b := entity.NewOutboxEntries(s, p); return binding{b, b.AllColumns} },
			[]string{
				"id", "event_id", "org_id", "project_id", "target", "payload", "attempt",
				"next_attempt_at", "status", "last_error", "created_at", "delivered_at", "public_id", "event_public_id",
			},
		},
		{
			"projects",
			func(s, p string) binding { b := entity.NewProjects(s, p); return binding{b, b.AllColumns} },
			[]string{"id", "public_id", "org_id", "slug", "name", "created_by", "created_at", "updated_at", "environment"},
		},
		{
			"password_resets",
			func(s, p string) binding { b := entity.NewPasswordResets(s, p); return binding{b, b.AllColumns} },
			[]string{"id", "public_id", "user_id", "token_hash", "expires_at", "used_at", "created_at"},
		},
		{
			"sessions",
			func(s, p string) binding { b := entity.NewSessions(s, p); return binding{b, b.AllColumns} },
			[]string{"sid", "user_id", "payload", "expires_at", "created_at"},
		},
		{
			"terms_acceptances",
			func(s, p string) binding { b := entity.NewTermsAcceptances(s, p); return binding{b, b.AllColumns} },
			[]string{"id", "public_id", "user_id", "terms_document_id", "privacy_document_id", "source", "accepted_at"},
		},
		{
			"users",
			func(s, p string) binding { b := entity.NewUsers(s, p); return binding{b, b.AllColumns} },
			[]string{
				"id", "public_id", "email", "name", "avatar_url", "password_hash", "idp_issuer", "idp_subject",
				"locale", "terms_accepted_at", "created_at", "updated_at",
			},
		},
		{
			"webhook_deliveries",
			func(s, p string) binding { b := entity.NewWebhookDeliveries(s, p); return binding{b, b.AllColumns} },
			[]string{
				"id", "org_id", "project_id", "endpoint_id", "delivery_id", "event_id", "event_type", "attempt",
				"status_code", "error", "response_body", "response_truncated", "response_headers", "duration_ms", "created_at",
			},
		},
		{
			"webhook_endpoints",
			func(s, p string) binding { b := entity.NewWebhookEndpoints(s, p); return binding{b, b.AllColumns} },
			[]string{
				"id", "public_id", "org_id", "project_id", "url", "description", "event_types",
				"secret_primary", "secret_secondary", "active", "created_at", "updated_at",
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.build("tenant", "oa_")
			assert.Equal(t, "tenant", b.table.SchemaName())
			assert.Equal(t, "oa_"+tt.name, b.table.TableName())
			assert.Equal(t, tt.name, b.table.Alias())

			names := make([]string, 0, len(b.columns))
			for _, c := range b.columns {
				names = append(names, c.Name())
				assert.Equal(t, tt.name, c.TableName(), "column %s", c.Name())
			}
			assert.Equal(t, tt.columns, names)

			query, _ := jetpg.SELECT(b.columns).FROM(b.table).Sql()
			assert.Contains(t, query, "FROM tenant.oa_"+tt.name+" AS "+tt.name)
		})
		t.Run(tt.name+" defaults to public schema", func(t *testing.T) {
			b := tt.build("", "")
			require.NotNil(t, b.table)
			assert.Equal(t, "public", b.table.SchemaName())
			assert.Equal(t, tt.name, b.table.TableName())
		})
	}
}

func TestBindings_EveryColumnFieldIsSet(t *testing.T) {
	for name, b := range map[string]any{
		"threads": entity.NewThreads("s", ""), "runs": entity.NewRuns("s", ""), "messages": entity.NewMessages("s", ""),
		"thread_snapshots": entity.NewThreadSnapshots("s", ""), "end_users": entity.NewEndUsers("s", ""),
		"end_user_identities": entity.NewEndUserIdentities("s", ""),
		"channel_connections": entity.NewChannelConnections("s", ""), "ingest_deliveries": entity.NewIngestDeliveries("s", ""),
	} {
		v := reflect.ValueOf(b).Elem()
		for i := range v.NumField() {
			f := v.Field(i)
			if f.Kind() == reflect.Interface {
				assert.False(t, f.IsNil(), "%s.%s is never assigned", name, v.Type().Field(i).Name)
			}
		}
	}
}
