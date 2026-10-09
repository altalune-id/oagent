package apikey

import (
	"context"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
)

// Save upserts a key and syncs its project grant in one transaction.
func (s *postgresStore) Save(ctx context.Context, k *APIKey) error {
	return db.RunInTx(ctx, s.pool, func(ctx context.Context) error {
		if err := s.save(ctx, k); err != nil {
			return err
		}
		return s.syncGrants(ctx, k)
	})
}

func (s *postgresStore) save(ctx context.Context, k *APIKey) error {
	stmt := s.keys.INSERT(s.keys.AllColumns).
		VALUES(
			k.ID, k.OrgID, pgNullableUUID(k.ProjectID), string(k.Kind), k.AllProjects, k.Name, k.SecretHash[:], k.SecretHint,
			pgTextArrayExpr(k.Scopes), pgUUIDArrayExpr(k.ResourceIDs), pgNullableUUID(k.CreatedBy),
			k.CreatedAt.UTC(), pgNullableTimeExpr(k.ExpiresAt), pgNullableTimeExpr(k.RevokedAt), pgNullableTimeExpr(k.LastUsedAt),
		).
		ON_CONFLICT(s.keys.ID).
		DO_UPDATE(postgres.SET(
			s.keys.Name.SET(postgres.String(k.Name)),
			s.keys.AllProjects.SET(postgres.Bool(k.AllProjects)),
			s.keys.Scopes.SET(pgTextArrayExpr(k.Scopes)),
			s.keys.ResourceIDs.SET(pgUUIDArrayExpr(k.ResourceIDs)),
			s.keys.ExpiresAt.SET(pgNullableTimeExpr(k.ExpiresAt)),
			s.keys.RevokedAt.SET(pgNullableTimeExpr(k.RevokedAt)),
			s.keys.LastUsedAt.SET(pgNullableTimeExpr(k.LastUsedAt)),
		))
	if _, err := stmt.ExecContext(ctx, s.writer(ctx)); err != nil {
		return fmt.Errorf("apikey.postgres: Save: %w", err)
	}
	return nil
}

// syncGrants writes the key's project grant: rows not kept are pruned, kept ones upserted.
func (s *postgresStore) syncGrants(ctx context.Context, k *APIKey) error {
	keep := make([]postgres.Expression, 0, len(k.ProjectIDs))
	for _, id := range k.ProjectIDs {
		keep = append(keep, postgres.UUID(id))
	}
	where := s.grants.KeyID.EQ(postgres.UUID(k.ID))
	if len(keep) > 0 {
		where = where.AND(s.grants.ProjectID.NOT_IN(keep...))
	}
	if _, err := s.grants.DELETE().WHERE(where).ExecContext(ctx, s.writer(ctx)); err != nil {
		return fmt.Errorf("apikey.postgres: prune grants: %w", err)
	}
	if len(k.ProjectIDs) == 0 {
		return nil
	}
	now := time.Now().UTC()
	ins := s.grants.INSERT(s.grants.AllColumns)
	for _, id := range k.ProjectIDs {
		ins = ins.VALUES(k.OrgID, k.ID, id, now)
	}
	if _, err := ins.ON_CONFLICT(s.grants.KeyID, s.grants.ProjectID).DO_NOTHING().ExecContext(ctx, s.writer(ctx)); err != nil {
		return fmt.Errorf("apikey.postgres: add grants: %w", err)
	}
	return nil
}

func pgNullableUUID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}
