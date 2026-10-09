package apikey

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"
)

// ByID loads a key by id.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*APIKey, error) {
	stmt := postgres.SELECT(s.selectColumns()).
		FROM(s.keys).
		WHERE(s.keys.ID.EQ(postgres.UUID(id))).
		LIMIT(1)
	var row apiKeyRow
	if err := stmt.QueryContext(ctx, s.reader(ctx), &row); err != nil {
		if errors.Is(err, qrm.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, &NotFoundError{ID: id.String()}
		}
		return nil, fmt.Errorf("apikey.postgres: ByID: %w", err)
	}
	return row.toKey()
}

// List returns an organization's keys, newest first.
func (s *postgresStore) List(ctx context.Context, orgID uuid.UUID) ([]*APIKey, error) {
	stmt := postgres.SELECT(s.selectColumns()).
		FROM(s.keys).
		WHERE(s.keys.OrgID.EQ(postgres.UUID(orgID))).
		ORDER_BY(s.keys.CreatedAt.DESC(), s.keys.ID.DESC())
	var rows []apiKeyRow
	if err := stmt.QueryContext(ctx, s.reader(ctx), &rows); err != nil {
		return nil, fmt.Errorf("apikey.postgres: List: %w", err)
	}
	out := make([]*APIKey, 0, len(rows))
	for i := range rows {
		k, err := rows[i].toKey()
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, nil
}
