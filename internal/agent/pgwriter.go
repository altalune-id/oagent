package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

// Save upserts an agent by id with its draft and publish state, bumping updated_at.
func (s *postgresStore) Save(ctx context.Context, a *Agent) error {
	now := time.Now().UTC()
	draft, err := jsonb(a.Draft)
	if err != nil {
		return fmt.Errorf("agent.postgres: Save: %w", err)
	}
	stmt := s.agents.INSERT(s.agents.AllColumns).
		VALUES(
			a.ID, a.PublicID, a.OrgID, a.ProjectID, a.Slug, a.Name, a.Description, createdByExpr(a), a.CreatedAt.UTC(), now,
			draft, timeExpr(a.DraftUpdatedAt), a.PublishedVersion, timeExpr(a.PublishedAt),
		).
		ON_CONFLICT(s.agents.ID).
		DO_UPDATE(postgres.SET(
			s.agents.Slug.SET(postgres.String(a.Slug)),
			s.agents.Name.SET(postgres.String(a.Name)),
			s.agents.Description.SET(postgres.String(a.Description)),
			s.agents.UpdatedAt.SET(postgres.TimestampzT(now)),
			s.agents.DraftConfig.SET(draft),
			s.agents.DraftUpdatedAt.SET(timeExpr(a.DraftUpdatedAt)),
			s.agents.PublishedVersion.SET(postgres.Int32(a.PublishedVersion)),
			s.agents.PublishedAt.SET(timeExpr(a.PublishedAt)),
		))
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		if db.IsUniqueViolation(err) {
			return &AlreadyExistsError{Slug: a.Slug}
		}
		return fmt.Errorf("agent.postgres: Save: %w", err)
	}
	return nil
}

// Delete removes an agent by id.
func (s *postgresStore) Delete(ctx context.Context, id uuid.UUID) error {
	stmt := s.agents.DELETE().WHERE(s.agents.ID.EQ(postgres.UUID(id)))
	var res sql.Result
	err := db.Write(ctx, s.pool, func(q qrm.DB) (err error) {
		res, err = stmt.ExecContext(ctx, q)
		return err
	})
	if err != nil {
		return fmt.Errorf("agent.postgres: Delete: %w", err)
	}
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
		return &NotFoundError{ID: id.String()}
	}
	return nil
}

// LockForPublish loads the agent FOR UPDATE on the writer, so concurrent publishes of one agent queue up.
func (s *postgresStore) LockForPublish(ctx context.Context, id uuid.UUID) (*Agent, error) {
	stmt := postgres.SELECT(s.agents.AllColumns).
		FROM(s.agents).
		WHERE(s.agents.ID.EQ(postgres.UUID(id))).
		FOR(postgres.UPDATE())
	var row agentRow
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{ID: id.String()}
		}
		return nil, fmt.Errorf("agent.postgres: LockForPublish: %w", err)
	}
	return row.toAgent()
}

// InsertVersion adds an immutable version; a second version with the same number is a unique violation.
func (s *postgresStore) InsertVersion(ctx context.Context, v *Version) error {
	cfg, err := jsonb(v.Config)
	if err != nil {
		return fmt.Errorf("agent.postgres: InsertVersion: %w", err)
	}
	by := entity.NullUUID()
	if v.CreatedBy.Valid {
		by = postgres.UUID(v.CreatedBy.UUID)
	}
	stmt := s.versions.INSERT(s.versions.AllColumns).
		VALUES(v.ID, v.PublicID, v.OrgID, v.ProjectID, v.AgentID, v.Version, cfg, by, v.CreatedAt.UTC())
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		return fmt.Errorf("agent.postgres: InsertVersion: %w", err)
	}
	return nil
}

func jsonb(c Config) (postgres.StringExpression, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	return postgres.StringExp(postgres.CAST(postgres.String(string(raw))).AS("jsonb")), nil
}

func timeExpr(t *time.Time) postgres.TimestampzExpression {
	if t == nil {
		return postgres.TimestampzExp(postgres.NULL)
	}
	return postgres.TimestampzT(t.UTC())
}

func createdByExpr(a *Agent) postgres.Expression {
	if a.CreatedBy.Valid {
		return postgres.UUID(a.CreatedBy.UUID)
	}
	return entity.NullUUID()
}
