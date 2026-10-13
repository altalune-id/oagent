package outbox

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

// ByID returns the entry id of target within the caller's org, or *NotFoundError.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID, target string) (Entry, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return Entry{}, err
	}
	stmt := postgres.SELECT(s.entries.AllColumns).
		FROM(s.entries).
		WHERE(s.entries.OrgID.EQ(postgres.UUID(tc.OrgID)).
			AND(s.entries.ID.EQ(postgres.UUID(id))).
			AND(s.entries.Target.EQ(postgres.String(target))))

	var row entryRow
	if err := stmt.QueryContext(ctx, s.reader(ctx), &row); err != nil {
		if db.IsNoRows(err) {
			return Entry{}, &NotFoundError{ID: id.String()}
		}
		return Entry{}, fmt.Errorf("outbox.postgres: ByID: %w", err)
	}
	return row.toEntry(), nil
}

// ListByTarget returns up to limit entries of target, newest first.
func (s *postgresStore) ListByTarget(ctx context.Context, target string, limit int) ([]Entry, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	stmt := postgres.SELECT(s.entries.AllColumns).
		FROM(s.entries).
		WHERE(s.entries.OrgID.EQ(postgres.UUID(tc.OrgID)).
			AND(s.entries.Target.EQ(postgres.String(target)))).
		ORDER_BY(s.entries.CreatedAt.DESC(), s.entries.ID.DESC()).
		LIMIT(listBatch(limit))

	var rows []entryRow
	if err := stmt.QueryContext(ctx, s.reader(ctx), &rows); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("outbox.postgres: ListByTarget: %w", err)
	}
	out := make([]Entry, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toEntry())
	}
	return out, nil
}
