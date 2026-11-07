package channel

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

// Resolve finds a connection by public id across tenants; every later read is scoped to the org it returns.
func (s *postgresStore) Resolve(ctx context.Context, publicID string) (Ref, error) {
	var row struct {
		ID        uuid.UUID `alias:"ref.id"`
		OrgID     uuid.UUID `alias:"ref.org_id"`
		ProjectID uuid.UUID `alias:"ref.project_id"`
		AgentID   uuid.UUID `alias:"ref.agent_id"`
		Provider  string    `alias:"ref.provider"`
		DeviceID  string    `alias:"ref.device_id"`
		Status    string    `alias:"ref.status"`
		SecretSet bool      `alias:"ref.secured"`
	}
	stmt := postgres.RawStatement(s.resolve, postgres.RawArgs{"#publicID": publicID})
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return Ref{}, &NotFoundError{}
		}
		return Ref{}, fmt.Errorf("channel.postgres: Resolve: %w", err)
	}
	return Ref(row), nil
}

// ByID loads a connection of the bound org.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*Connection, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.conns
	stmt := postgres.SELECT(t.AllColumns).FROM(t).WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).LIMIT(1)
	var row connRow
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("channel.postgres: ByID: %w", err)
	}
	return row.toConnection(), nil
}

// List lists a project's connections, oldest first.
func (s *postgresStore) List(ctx context.Context, projectID uuid.UUID) ([]*Connection, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.conns
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.ProjectID.EQ(postgres.UUID(projectID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		ORDER_BY(t.CreatedAt.ASC(), t.ID.ASC())
	var rows []connRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("channel.postgres: List: %w", err)
	}
	out := make([]*Connection, len(rows))
	for i := range rows {
		out[i] = rows[i].toConnection()
	}
	return out, nil
}
