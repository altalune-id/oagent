package approval

import (
	"context"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

// Get loads an approval of the bound org.
func (s *postgresStore) Get(ctx context.Context, id uuid.UUID) (*Approval, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.approvals
	return s.one(ctx, "Get", db.Read, postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))))
}

// PendingForThread loads the thread's pending approval from the primary.
func (s *postgresStore) PendingForThread(ctx context.Context, threadID uuid.UUID) (*Approval, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.approvals
	return s.one(ctx, "PendingForThread", db.ReadPrimary, postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.ThreadID.EQ(postgres.UUID(threadID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))).
			AND(t.Status.EQ(postgres.String(string(StatusPending))))))
}

// LatestForRun loads the run's newest approval from the primary, so a resume sees the decision that woke it.
func (s *postgresStore) LatestForRun(ctx context.Context, runID uuid.UUID) (*Approval, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.approvals
	return s.one(ctx, "LatestForRun", db.ReadPrimary, postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.RunID.EQ(postgres.UUID(runID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		ORDER_BY(t.CreatedAt.DESC(), t.PublicID.DESC()).LIMIT(1))
}

type reader func(ctx context.Context, pool db.Pool, fn func(q qrm.DB) error) error

func (s *postgresStore) one(ctx context.Context, op string, read reader, stmt postgres.SelectStatement) (*Approval, error) {
	var row approvalRow
	if err := read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("approval.postgres: %s: %w", op, err)
	}
	return row.toApproval()
}

// List pages a project's approvals newest first after the cursor.
func (s *postgresStore) List(ctx context.Context, f Filter, after Cursor, limit int) ([]*Approval, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.approvals
	where := t.OrgID.EQ(postgres.UUID(tc.OrgID)).AND(t.ProjectID.EQ(postgres.UUID(f.ProjectID)))
	if len(f.AgentIDs) > 0 {
		where = where.AND(t.AgentID.IN(uuids(f.AgentIDs)...))
	}
	switch f.Status {
	case "":
	case StatusDecided:
		where = where.AND(t.Status.IN(postgres.String(string(StatusApproved)), postgres.String(string(StatusRejected))))
	default:
		where = where.AND(t.Status.EQ(postgres.String(string(f.Status))))
	}
	// NOTE: a pending row past its expiry is expired already, even before the sweep writes it.
	if f.Status == StatusPending {
		where = where.AND(t.ExpiresAt.GT(postgres.TimestampzT(f.Now)))
	}
	if !after.At.IsZero() {
		where = where.AND(postgres.ROW(t.CreatedAt, t.PublicID).LT(postgres.ROW(postgres.TimestampzT(after.At), postgres.String(after.PublicID))))
	}
	stmt := postgres.SELECT(t.AllColumns).FROM(t).WHERE(where).ORDER_BY(t.CreatedAt.DESC(), t.PublicID.DESC()).LIMIT(int64(max(limit, 1)))
	var rows []approvalRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("approval.postgres: List: %w", err)
	}
	out := make([]*Approval, 0, len(rows))
	for i := range rows {
		a, err := rows[i].toApproval()
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// CountPending counts a project's pending approvals of the given agents, or of every agent when agents is empty.
func (s *postgresStore) CountPending(ctx context.Context, projectID uuid.UUID, agents []uuid.UUID, now time.Time) (int, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return 0, err
	}
	t := s.approvals
	where := t.OrgID.EQ(postgres.UUID(tc.OrgID)).AND(t.ProjectID.EQ(postgres.UUID(projectID))).
		AND(t.Status.EQ(postgres.String(string(StatusPending)))).AND(t.ExpiresAt.GT(postgres.TimestampzT(now)))
	if len(agents) > 0 {
		where = where.AND(t.AgentID.IN(uuids(agents)...))
	}
	var out struct {
		N int64 `alias:"pending.n"`
	}
	stmt := postgres.SELECT(postgres.COUNT(postgres.STAR).AS("pending.n")).FROM(t).WHERE(where)
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &out) }); err != nil {
		return 0, fmt.Errorf("approval.postgres: CountPending: %w", err)
	}
	return int(out.N), nil
}

// Resolve finds an approval by public id in any org, through the definer function.
func (s *postgresStore) Resolve(ctx context.Context, publicID string) (Ref, error) {
	var row struct {
		ID        uuid.UUID `alias:"ref.id"`
		OrgID     uuid.UUID `alias:"ref.org_id"`
		ProjectID uuid.UUID `alias:"ref.project_id"`
		AgentID   uuid.UUID `alias:"ref.agent_id"`
	}
	stmt := postgres.RawStatement(s.resolve, postgres.RawArgs{"#publicID": publicID})
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return Ref{}, &NotFoundError{}
		}
		return Ref{}, fmt.Errorf("approval.postgres: Resolve: %w", err)
	}
	return Ref(row), nil
}

func uuids(ids []uuid.UUID) []postgres.Expression {
	out := make([]postgres.Expression, len(ids))
	for i, id := range ids {
		out[i] = postgres.UUID(id)
	}
	return out
}

// DueExpiries lists pending approvals of the bound org past their expiry on the database clock, by the expiry index.
func (s *postgresStore) DueExpiries(ctx context.Context, limit int) ([]uuid.UUID, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.approvals
	stmt := postgres.SELECT(t.ID).FROM(t).
		WHERE(t.OrgID.EQ(postgres.UUID(tc.OrgID)).
			AND(t.Status.EQ(postgres.String(string(StatusPending)))).
			AND(t.ExpiresAt.LT_EQ(postgres.NOW()))).
		ORDER_BY(t.ExpiresAt.ASC()).
		LIMIT(int64(max(limit, 1)))
	var rows []struct {
		ID uuid.UUID `alias:"approvals.id"`
	}
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("approval.postgres: DueExpiries: %w", err)
	}
	out := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out, nil
}
