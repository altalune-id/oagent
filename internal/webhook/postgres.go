package webhook

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/schema/entity"
)

func errorIsNoRows(err error) bool {
	return errors.Is(err, qrm.ErrNoRows) || errors.Is(err, sql.ErrNoRows)
}

type postgresStore struct {
	pool      db.Pool
	endpoints *entity.WebhookEndpoints
	attempts  *entity.WebhookDeliveries
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	return &postgresStore{
		pool:      pool,
		endpoints: entity.NewWebhookEndpoints(schema, tablePrefix),
		attempts:  entity.NewWebhookDeliveries(schema, tablePrefix),
	}
}

func (s *postgresStore) reader(ctx context.Context) qrm.DB {
	if tx, ok := db.CurrentTx(ctx); ok {
		return tx
	}
	return s.pool.R
}

// txAcquire returns the ambient transaction, or begins one and reports that it owns it.
func (s *postgresStore) txAcquire(ctx context.Context) (*sql.Tx, bool, tenant.Context, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, false, tenant.Context{}, err
	}
	if tx, ok := db.CurrentTx(ctx); ok {
		return tx, false, tc, nil
	}
	tx, err := s.pool.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, tenant.Context{}, fmt.Errorf("webhook.postgres: begin: %w", err)
	}
	return tx, true, tc, nil
}

// endTx commits or rolls back a transaction txAcquire started; it is a no-op for an ambient one.
func (s *postgresStore) endTx(tx *sql.Tx, owned bool, err error) error {
	if !owned {
		return err
	}
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if cerr := tx.Commit(); cerr != nil {
		return fmt.Errorf("webhook.postgres: commit: %w", cerr)
	}
	return nil
}

type pgEndpointRow struct {
	ID              uuid.UUID `alias:"webhook_endpoints.id"`
	OrgID           uuid.UUID `alias:"webhook_endpoints.org_id"`
	ProjectID       uuid.UUID `alias:"webhook_endpoints.project_id"`
	URL             string    `alias:"webhook_endpoints.url"`
	Description     string    `alias:"webhook_endpoints.description"`
	EventTypes      string    `alias:"webhook_endpoints.event_types"`
	SecretPrimary   []byte    `alias:"webhook_endpoints.secret_primary"`
	SecretSecondary []byte    `alias:"webhook_endpoints.secret_secondary"`
	Active          bool      `alias:"webhook_endpoints.active"`
	CreatedAt       time.Time `alias:"webhook_endpoints.created_at"`
	UpdatedAt       time.Time `alias:"webhook_endpoints.updated_at"`
}

func (r *pgEndpointRow) toEndpoint() (*Endpoint, error) {
	types, err := unmarshalEventTypes(r.EventTypes)
	if err != nil {
		return nil, fmt.Errorf("webhook.postgres: %w", err)
	}
	return &Endpoint{
		ID:          r.ID,
		OrgID:       r.OrgID,
		ProjectID:   r.ProjectID,
		URL:         r.URL,
		Description: r.Description,
		EventTypes:  types,
		Secrets:     SealedSecrets{Primary: r.SecretPrimary, Secondary: r.SecretSecondary},
		Active:      r.Active,
		CreatedAt:   r.CreatedAt.UTC(),
		UpdatedAt:   r.UpdatedAt.UTC(),
	}, nil
}

type pgAttemptRow struct {
	ID                uuid.UUID `alias:"webhook_deliveries.id"`
	OrgID             uuid.UUID `alias:"webhook_deliveries.org_id"`
	ProjectID         uuid.UUID `alias:"webhook_deliveries.project_id"`
	EndpointID        uuid.UUID `alias:"webhook_deliveries.endpoint_id"`
	DeliveryID        uuid.UUID `alias:"webhook_deliveries.delivery_id"`
	EventID           uuid.UUID `alias:"webhook_deliveries.event_id"`
	EventType         string    `alias:"webhook_deliveries.event_type"`
	Attempt           int       `alias:"webhook_deliveries.attempt"`
	StatusCode        int       `alias:"webhook_deliveries.status_code"`
	Error             string    `alias:"webhook_deliveries.error"`
	ResponseBody      string    `alias:"webhook_deliveries.response_body"`
	ResponseTruncated bool      `alias:"webhook_deliveries.response_truncated"`
	ResponseHeaders   string    `alias:"webhook_deliveries.response_headers"`
	DurationMs        int64     `alias:"webhook_deliveries.duration_ms"`
	CreatedAt         time.Time `alias:"webhook_deliveries.created_at"`
}

func (r *pgAttemptRow) toAttempt() (Attempt, error) {
	headers, err := unmarshalHeaders(r.ResponseHeaders)
	if err != nil {
		return Attempt{}, fmt.Errorf("webhook.postgres: attempt %w", err)
	}
	return Attempt{
		ID:                r.ID,
		OrgID:             r.OrgID,
		ProjectID:         r.ProjectID,
		EndpointID:        r.EndpointID,
		DeliveryID:        r.DeliveryID,
		EventID:           r.EventID,
		EventType:         events.Type(r.EventType),
		Attempt:           r.Attempt,
		StatusCode:        r.StatusCode,
		Error:             r.Error,
		ResponseBody:      r.ResponseBody,
		ResponseTruncated: r.ResponseTruncated,
		ResponseHeaders:   headers,
		Duration:          time.Duration(r.DurationMs) * time.Millisecond,
		CreatedAt:         r.CreatedAt.UTC(),
	}, nil
}
