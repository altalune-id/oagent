package agent

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
)

// ByID loads an agent by id.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*Agent, error) {
	stmt := postgres.SELECT(s.agents.AllColumns).
		FROM(s.agents).
		WHERE(s.agents.ID.EQ(postgres.UUID(id))).
		LIMIT(1)
	var row agentRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{ID: id.String()}
		}
		return nil, fmt.Errorf("agent.postgres: ByID: %w", err)
	}
	return row.toAgent()
}

// Versions returns up to limit of the agent's versions, newest first.
func (s *postgresStore) Versions(ctx context.Context, agentID uuid.UUID, limit int) ([]*Version, error) {
	return s.selectVersions(ctx, "Versions", agentID, int64(max(limit, 1)))
}

// LatestVersion returns the agent's newest version, or nil before the first publish.
func (s *postgresStore) LatestVersion(ctx context.Context, agentID uuid.UUID) (*Version, error) {
	vs, err := s.selectVersions(ctx, "LatestVersion", agentID, 1)
	if err != nil || len(vs) == 0 {
		return nil, err
	}
	return vs[0], nil
}

func (s *postgresStore) selectVersions(ctx context.Context, op string, agentID uuid.UUID, limit int64) ([]*Version, error) {
	stmt := postgres.SELECT(s.versions.AllColumns).
		FROM(s.versions).
		WHERE(s.versions.AgentID.EQ(postgres.UUID(agentID))).
		ORDER_BY(s.versions.Version.DESC())
	if limit > 0 {
		stmt = stmt.LIMIT(limit)
	}
	var rows []versionRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil {
		return nil, fmt.Errorf("agent.postgres: %s: %w", op, err)
	}
	out := make([]*Version, 0, len(rows))
	for i := range rows {
		v, err := rows[i].toVersion()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// List returns a project's agents ordered by creation time.
func (s *postgresStore) List(ctx context.Context, projectID uuid.UUID) ([]*Agent, error) {
	stmt := postgres.SELECT(s.agents.AllColumns).
		FROM(s.agents).
		WHERE(s.agents.ProjectID.EQ(postgres.UUID(projectID))).
		ORDER_BY(s.agents.CreatedAt.ASC())
	var rows []agentRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil {
		return nil, fmt.Errorf("agent.postgres: List: %w", err)
	}
	out := make([]*Agent, 0, len(rows))
	for i := range rows {
		a, err := rows[i].toAgent()
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// Resolve names publicID's agent and its org through a definer, so a caller with no active org can check reach and then scope.
func (s *postgresStore) Resolve(ctx context.Context, publicID string) (Ref, error) {
	var row struct {
		ID        uuid.UUID `alias:"agents.id"`
		OrgID     uuid.UUID `alias:"agents.org_id"`
		ProjectID uuid.UUID `alias:"agents.project_id"`
	}
	stmt := postgres.RawStatement(s.resolve, postgres.RawArgs{"#publicID": publicID})
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return Ref{}, &NotFoundError{}
		}
		return Ref{}, fmt.Errorf("agent.postgres: Resolve: %w", err)
	}
	return Ref(row), nil
}
