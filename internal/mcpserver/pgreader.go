package mcpserver

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

// ByID loads a server of the org on ctx by id.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*Server, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.servers
	stmt := postgres.SELECT(t.AllColumns).
		FROM(t).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		LIMIT(1)
	var row serverRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{ID: id.String()}
		}
		return nil, fmt.Errorf("mcpserver.postgres: ByID: %w", err)
	}
	return row.toServer(), nil
}

// List returns a project's servers, oldest first.
func (s *postgresStore) List(ctx context.Context, projectID uuid.UUID) ([]*Server, error) {
	t := s.servers
	stmt := postgres.SELECT(t.AllColumns).
		FROM(t).
		WHERE(t.ProjectID.EQ(postgres.UUID(projectID))).
		ORDER_BY(t.CreatedAt.ASC(), t.ID.ASC())
	var rows []serverRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil {
		return nil, fmt.Errorf("mcpserver.postgres: List: %w", err)
	}
	return toServers(rows), nil
}

// Resolve names publicID's server and its org through a definer, so a caller with no active org can check reach and then scope.
func (s *postgresStore) Resolve(ctx context.Context, publicID string) (Ref, error) {
	var row struct {
		ID        uuid.UUID `alias:"mcp_servers.id"`
		OrgID     uuid.UUID `alias:"mcp_servers.org_id"`
		ProjectID uuid.UUID `alias:"mcp_servers.project_id"`
	}
	stmt := postgres.RawStatement(s.resolve, postgres.RawArgs{"#publicID": publicID})
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return Ref{}, &NotFoundError{}
		}
		return Ref{}, fmt.Errorf("mcpserver.postgres: Resolve: %w", err)
	}
	return Ref(row), nil
}
