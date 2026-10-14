package apikey

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
)

// Save upserts a key and syncs its project grant in one transaction.
func (s *postgresStore) Save(ctx context.Context, k *APIKey) error {
	return db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		if err := s.save(ctx, tx, k); err != nil {
			return err
		}
		return s.syncGrants(ctx, tx, k)
	})
}

func (s *postgresStore) save(ctx context.Context, tx *sql.Tx, k *APIKey) error {
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
	if _, err := stmt.ExecContext(ctx, tx); err != nil {
		if name, ok := db.ViolatedConstraint(err); ok && name == s.hintIndex && db.IsUniqueViolation(err) {
			return &HintTakenError{}
		}
		return fmt.Errorf("apikey.postgres: Save: %w", err)
	}
	return nil
}

func (s *postgresStore) syncGrants(ctx context.Context, tx *sql.Tx, k *APIKey) error {
	keep := make([]postgres.Expression, 0, len(k.ProjectIDs))
	for _, id := range k.ProjectIDs {
		keep = append(keep, postgres.UUID(id))
	}
	where := s.grants.KeyID.EQ(postgres.UUID(k.ID))
	if len(keep) > 0 {
		where = where.AND(s.grants.ProjectID.NOT_IN(keep...))
	}
	if _, err := s.grants.DELETE().WHERE(where).ExecContext(ctx, tx); err != nil {
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
	if _, err := ins.ON_CONFLICT(s.grants.KeyID, s.grants.ProjectID).DO_NOTHING().ExecContext(ctx, tx); err != nil {
		return fmt.Errorf("apikey.postgres: add grants: %w", err)
	}
	return nil
}

// TouchLastUsed moves each key's last_used_at forward to its time; it never moves it backwards.
func (s *postgresStore) TouchLastUsed(ctx context.Context, used map[uuid.UUID]time.Time) error {
	if len(used) == 0 {
		return nil
	}
	return db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		for id, at := range used {
			ts := postgres.TimestampzT(at.UTC())
			stmt := s.keys.UPDATE(s.keys.LastUsedAt).
				SET(postgres.GREATEST(postgres.COALESCE(s.keys.LastUsedAt, ts), ts)).
				WHERE(s.keys.ID.EQ(postgres.UUID(id)))
			if _, err := stmt.ExecContext(ctx, tx); err != nil {
				return fmt.Errorf("apikey.postgres: TouchLastUsed: %w", err)
			}
		}
		return nil
	})
}

// RevokePersonalOf revokes ownerID's live personal keys in orgID at at.
func (s *postgresStore) RevokePersonalOf(ctx context.Context, orgID, ownerID uuid.UUID, at time.Time) (int, error) {
	t := s.keys
	stmt := t.UPDATE(t.RevokedAt).
		SET(postgres.TimestampzT(at.UTC())).
		WHERE(t.OrgID.EQ(postgres.UUID(orgID)).
			AND(t.Kind.EQ(postgres.String(string(KindPersonal)))).
			AND(t.CreatedBy.EQ(postgres.UUID(ownerID))).
			AND(t.RevokedAt.IS_NULL()))
	res, err := stmt.ExecContext(ctx, db.Writer(ctx, s.pool))
	if err != nil {
		return 0, fmt.Errorf("apikey.postgres: RevokePersonalOf: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("apikey.postgres: RevokePersonalOf: rows affected: %w", err)
	}
	return int(n), nil
}

func pgNullableUUID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}
