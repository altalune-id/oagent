package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type resetPostgres struct {
	pool   db.Pool
	resets *entity.PasswordResets
}

// NewResetStore builds the Postgres ResetStore over pool.
func NewResetStore(pool db.Pool, schema, tablePrefix string) ResetStore {
	return &resetPostgres{pool: pool, resets: entity.NewPasswordResets(schema, tablePrefix)}
}

func (s *resetPostgres) Insert(ctx context.Context, r *Reset) error {
	t := s.resets
	stmt := t.INSERT(t.AllColumns).VALUES(
		r.ID, r.PublicID, r.UserID, r.TokenHash, r.ExpiresAt.UTC(), entity.NullTimestampz(), r.CreatedAt.UTC(),
	)
	if _, err := stmt.ExecContext(ctx, db.Writer(ctx, s.pool)); err != nil {
		return fmt.Errorf("auth.resets: Insert: %w", err)
	}
	return nil
}

func (s *resetPostgres) DeleteUnused(ctx context.Context, userID uuid.UUID) error {
	t := s.resets
	stmt := t.DELETE().WHERE(t.UserID.EQ(postgres.UUID(userID)).AND(t.UsedAt.IS_NULL()))
	if _, err := stmt.ExecContext(ctx, db.Writer(ctx, s.pool)); err != nil {
		return fmt.Errorf("auth.resets: DeleteUnused: %w", err)
	}
	return nil
}

func (s *resetPostgres) HasRecentUnused(ctx context.Context, userID uuid.UUID, since time.Time) (bool, error) {
	t := s.resets
	stmt := postgres.SELECT(t.ID).FROM(t).WHERE(
		t.UserID.EQ(postgres.UUID(userID)).AND(t.UsedAt.IS_NULL()).AND(t.CreatedAt.GT(postgres.TimestampzT(since.UTC()))),
	).LIMIT(1)
	var row struct {
		ID uuid.UUID `alias:"password_resets.id"`
	}
	if err := stmt.QueryContext(ctx, db.Writer(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return false, nil
		}
		return false, fmt.Errorf("auth.resets: HasRecentUnused: %w", err)
	}
	return true, nil
}

// NOTE: reads use the writer; a lagging replica could show a used reset as usable.
func (s *resetPostgres) ByTokenHash(ctx context.Context, hash string) (*Reset, error) {
	t := s.resets
	return s.one(ctx, "ByTokenHash", postgres.SELECT(t.AllColumns).FROM(t).WHERE(t.TokenHash.EQ(postgres.String(hash))))
}

func (s *resetPostgres) ByPublicID(ctx context.Context, publicID string) (*Reset, error) {
	t := s.resets
	return s.one(ctx, "ByPublicID", postgres.SELECT(t.AllColumns).FROM(t).WHERE(t.PublicID.EQ(postgres.String(publicID))))
}

// SECURITY: one conditional UPDATE, so two submits of one link cannot both use it.
func (s *resetPostgres) MarkUsed(ctx context.Context, publicID string, now time.Time) (*Reset, error) {
	t := s.resets
	at := postgres.TimestampzT(now.UTC())
	stmt := t.UPDATE().SET(t.UsedAt.SET(at)).
		WHERE(t.PublicID.EQ(postgres.String(publicID)).AND(t.UsedAt.IS_NULL()).AND(t.ExpiresAt.GT(at))).
		RETURNING(t.AllColumns)
	var row resetRow
	if err := stmt.QueryContext(ctx, db.Writer(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &ResetNotUsableError{Reason: ResetNotFound}
		}
		return nil, fmt.Errorf("auth.resets: MarkUsed: %w", err)
	}
	return row.toReset(), nil
}

func (s *resetPostgres) one(ctx context.Context, op string, stmt postgres.SelectStatement) (*Reset, error) {
	var row resetRow
	if err := stmt.QueryContext(ctx, db.Writer(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &ResetNotUsableError{Reason: ResetNotFound}
		}
		return nil, fmt.Errorf("auth.resets: %s: %w", op, err)
	}
	return row.toReset(), nil
}
