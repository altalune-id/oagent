package webtool

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

// Provider loads the search provider of a project of the org on ctx.
func (s *postgresStore) Provider(ctx context.Context, projectID uuid.UUID) (*Provider, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.providers
	stmt := postgres.SELECT(t.AllColumns).
		FROM(t).
		WHERE(t.ProjectID.EQ(postgres.UUID(projectID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		LIMIT(1)
	var row providerRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("webtool.postgres: Provider: %w", err)
	}
	return row.toProvider(), nil
}

// Secrets lists a project's HTTP secrets by host, then header.
func (s *postgresStore) Secrets(ctx context.Context, projectID uuid.UUID) ([]*HTTPSecret, error) {
	return s.secretsWhere(ctx, "Secrets", s.secrets.ProjectID.EQ(postgres.UUID(projectID)))
}

// SecretsForHosts lists a project's HTTP secrets for the given hosts only.
func (s *postgresStore) SecretsForHosts(ctx context.Context, projectID uuid.UUID, hosts []string) ([]*HTTPSecret, error) {
	if len(hosts) == 0 {
		return nil, nil
	}
	in := make([]postgres.Expression, 0, len(hosts))
	for _, h := range hosts {
		in = append(in, postgres.String(h))
	}
	t := s.secrets
	return s.secretsWhere(ctx, "SecretsForHosts", t.ProjectID.EQ(postgres.UUID(projectID)).AND(t.Host.IN(in...)))
}

func (s *postgresStore) secretsWhere(ctx context.Context, op string, where postgres.BoolExpression) ([]*HTTPSecret, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.secrets
	stmt := postgres.SELECT(t.AllColumns).
		FROM(t).
		WHERE(where.AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		ORDER_BY(t.Host.ASC(), t.Header.ASC(), t.ID.ASC())
	var rows []secretRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("webtool.postgres: %s: %w", op, err)
	}
	return toSecrets(rows), nil
}

// SecretCount counts a project's HTTP secrets and says whether one exists for host and header.
func (s *postgresStore) SecretCount(ctx context.Context, projectID uuid.UUID, host, header string) (total int, exists bool, err error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return 0, false, err
	}
	t := s.secrets
	match := t.Host.EQ(postgres.String(host)).AND(t.Header.EQ(postgres.String(header)))
	stmt := postgres.SELECT(
		postgres.COUNT(postgres.STAR).AS("count.total"),
		postgres.COALESCE(postgres.BOOL_OR(match), postgres.Bool(false)).AS("count.exists"),
	).
		FROM(t).
		WHERE(t.ProjectID.EQ(postgres.UUID(projectID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	var row struct {
		Total  int64 `alias:"count.total"`
		Exists bool  `alias:"count.exists"`
	}
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		return 0, false, fmt.Errorf("webtool.postgres: SecretCount: %w", err)
	}
	return int(row.Total), row.Exists, nil
}

// ResolveSecret names publicID's secret and its org through a definer, so a caller with no active org can check reach and then scope.
func (s *postgresStore) ResolveSecret(ctx context.Context, publicID string) (Ref, error) {
	var row struct {
		ID        uuid.UUID `alias:"http_secrets.id"`
		OrgID     uuid.UUID `alias:"http_secrets.org_id"`
		ProjectID uuid.UUID `alias:"http_secrets.project_id"`
	}
	stmt := postgres.RawStatement(s.resolve, postgres.RawArgs{"#publicID": publicID})
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return Ref{}, &NotFoundError{}
		}
		return Ref{}, fmt.Errorf("webtool.postgres: ResolveSecret: %w", err)
	}
	return Ref(row), nil
}
