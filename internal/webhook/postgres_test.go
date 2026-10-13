package webhook

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/schema/entity"
)

// errBoom is the shared sentinel error the store tests simulate driver failures with.
var errBoom = context.DeadlineExceeded

func endpointColumns() []string {
	return []string{
		"webhook_endpoints.id", "webhook_endpoints.public_id", "webhook_endpoints.org_id", "webhook_endpoints.project_id",
		"webhook_endpoints.url", "webhook_endpoints.description", "webhook_endpoints.event_types",
		"webhook_endpoints.secret_primary", "webhook_endpoints.secret_secondary", "webhook_endpoints.active",
		"webhook_endpoints.created_at", "webhook_endpoints.updated_at",
	}
}

func attemptColumns() []string {
	return []string{
		"webhook_deliveries.id", "webhook_deliveries.org_id", "webhook_deliveries.project_id",
		"webhook_deliveries.endpoint_id", "webhook_deliveries.delivery_id", "webhook_deliveries.event_id",
		"webhook_deliveries.event_type", "webhook_deliveries.attempt", "webhook_deliveries.status_code",
		"webhook_deliveries.error", "webhook_deliveries.response_body", "webhook_deliveries.response_truncated",
		"webhook_deliveries.response_headers", "webhook_deliveries.duration_ms", "webhook_deliveries.created_at",
	}
}

func newStoreTest(t *testing.T) (*postgresStore, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	return &postgresStore{
		pool:      db.Pool{W: sqlDB, R: sqlDB},
		endpoints: entity.NewWebhookEndpoints("public", ""),
		attempts:  entity.NewWebhookDeliveries("public", ""),
	}, mock
}

func storeCtx(org, project uuid.UUID) context.Context {
	return tenant.Into(context.Background(), tenant.Context{OrgID: org, ProjectID: project})
}

func TestPgEndpointRowToEndpoint(t *testing.T) {
	raw, _ := marshalEventTypes([]events.Type{events.AgentCreated})
	row := endpointRow{ID: uuid.New(), PublicID: "pub", OrgID: uuid.New(), ProjectID: uuid.New(), URL: "u", EventTypes: raw, Active: true}
	e, err := row.toEndpoint()
	require.NoError(t, err)
	assert.Equal(t, "u", e.URL)
	assert.Equal(t, "pub", e.PublicID)
	assert.Equal(t, []events.Type{events.AgentCreated}, e.EventTypes)

	_, err = (&endpointRow{EventTypes: "nope"}).toEndpoint()
	assert.Error(t, err)
}

func TestPgAttemptRowToAttempt(t *testing.T) {
	row := attemptRow{ID: uuid.New(), EventType: string(events.AgentCreated), DurationMs: 5, ResponseHeaders: "[]"}
	a, err := row.toAttempt()
	require.NoError(t, err)
	assert.Equal(t, events.AgentCreated, a.EventType)
	assert.Equal(t, 5*time.Millisecond, a.Duration)

	_, err = (&attemptRow{ResponseHeaders: "nope"}).toAttempt()
	assert.Error(t, err)
}

func TestNewStoreNotNil(t *testing.T) {
	assert.NotNil(t, NewStore(db.Pool{}, "public", ""))
}

func TestReaderSelection(t *testing.T) {
	s, mock := newStoreTest(t)
	sqlDB := s.pool.R
	assert.Same(t, sqlDB, s.reader(context.Background()).(*sql.DB))
	mock.ExpectBegin()
	tx, err := s.pool.W.Begin()
	require.NoError(t, err)
	assert.Same(t, tx, s.reader(db.ContextWithTx(context.Background(), tx)).(*sql.Tx))
}

func TestTxAcquireAndEnd(t *testing.T) {
	org, project := uuid.New(), uuid.New()

	t.Run("missing tenant", func(t *testing.T) {
		s, _ := newStoreTest(t)
		_, _, _, err := s.txAcquire(context.Background())
		assert.True(t, tenant.IsMissingError(err))
	})

	t.Run("begins", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		_, owned, _, err := s.txAcquire(storeCtx(org, project))
		require.NoError(t, err)
		assert.True(t, owned)
	})

	t.Run("ambient", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		tx, err := s.pool.W.Begin()
		require.NoError(t, err)
		got, owned, _, err := s.txAcquire(db.ContextWithTx(storeCtx(org, project), tx))
		require.NoError(t, err)
		assert.False(t, owned)
		assert.Same(t, tx, got)
	})

	t.Run("endTx not owned", func(t *testing.T) {
		s, _ := newStoreTest(t)
		assert.Equal(t, errBoom, s.endTx(nil, false, errBoom))
	})
}
