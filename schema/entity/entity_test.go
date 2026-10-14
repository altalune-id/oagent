package entity_test

import (
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
			[]string{"id", "public_id", "org_id", "project_id", "slug", "name", "description", "created_by", "created_at", "updated_at"},
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
			"invites",
			func(s, p string) binding { b := entity.NewInvites(s, p); return binding{b, b.AllColumns} },
			[]string{
				"id", "public_id", "org_id", "email", "role", "token_hash", "draft", "invited_by",
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
			[]string{"id", "public_id", "org_id", "slug", "name", "created_by", "created_at", "updated_at"},
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
