package apikey

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
)

// ByID loads a key by id.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*APIKey, error) {
	stmt := postgres.SELECT(s.selectColumns()).
		FROM(s.keys).
		WHERE(s.keys.ID.EQ(postgres.UUID(id))).
		LIMIT(1)
	var row apiKeyRow
	if err := stmt.QueryContext(ctx, db.Reader(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{ID: id.String()}
		}
		return nil, fmt.Errorf("apikey.postgres: ByID: %w", err)
	}
	return row.toKey()
}

// ByHint loads orgID's live key whose secret ends with hint.
func (s *postgresStore) ByHint(ctx context.Context, orgID uuid.UUID, hint string) (*APIKey, error) {
	stmt := postgres.SELECT(s.selectColumns()).
		FROM(s.keys).
		WHERE(s.keys.OrgID.EQ(postgres.UUID(orgID)).
			AND(s.keys.SecretHint.EQ(postgres.String(hint))).
			AND(s.keys.RevokedAt.IS_NULL())).
		LIMIT(1)
	var row apiKeyRow
	if err := stmt.QueryContext(ctx, db.Reader(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("apikey.postgres: ByHint: %w", err)
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
	if err := stmt.QueryContext(ctx, db.Reader(ctx, s.pool), &rows); err != nil {
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

// BySecretHash loads the key whose secret hashes to hash. SECURITY: reads the writer, so a revoked key is never stale.
func (s *postgresStore) BySecretHash(ctx context.Context, hash [32]byte) (*APIKey, error) {
	stmt := postgres.SELECT(s.selectColumns()).
		FROM(s.keys).
		WHERE(s.keys.SecretHash.EQ(postgres.Bytea(hash[:]))).
		LIMIT(1)
	var row apiKeyRow
	if err := stmt.QueryContext(ctx, db.Writer(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("apikey.postgres: BySecretHash: %w", err)
	}
	return row.toKey()
}
