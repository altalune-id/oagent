package knowledge

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

// SourceDocumentCounts sums one source's documents.
func (s *postgresStore) SourceDocumentCounts(ctx context.Context, sourceID uuid.UUID) (DocumentCounts, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return DocumentCounts{}, err
	}
	var c DocumentCounts
	err = db.Read(ctx, s.pool, func(q qrm.DB) error {
		rows, err := q.QueryContext(ctx, s.sql.sourceCounts, tc.OrgID, sourceID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			if err := rows.Scan(&c.Total, &c.Ready, &c.Failed); err != nil {
				return err
			}
		}
		return rows.Err()
	})
	if err != nil {
		return DocumentCounts{}, fmt.Errorf("knowledge.postgres: SourceDocumentCounts: %w", err)
	}
	return c, nil
}

// DocumentStats reads each document's file size and passage count.
func (s *postgresStore) DocumentStats(ctx context.Context, documentIDs []uuid.UUID) (map[uuid.UUID]DocumentStats, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]DocumentStats, len(documentIDs))
	if len(documentIDs) == 0 {
		return out, nil
	}
	ids := make([]string, len(documentIDs))
	for i, id := range documentIDs {
		ids[i] = id.String()
	}
	err = db.Read(ctx, s.pool, func(q qrm.DB) error {
		rows, err := q.QueryContext(ctx, s.sql.documentStats, tc.OrgID, ids)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var (
				id uuid.UUID
				st DocumentStats
			)
			if err := rows.Scan(&id, &st.SizeBytes, &st.Chunks); err != nil {
				return err
			}
			out[id] = st
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("knowledge.postgres: DocumentStats: %w", err)
	}
	return out, nil
}
