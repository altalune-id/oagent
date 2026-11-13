package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/schema/entity"
)

// Insert records a new file.
func (s *postgresStore) Insert(ctx context.Context, f *File) error {
	t := s.files
	completed := entity.NullTimestampz()
	if f.CompletedAt != nil {
		completed = postgres.TimestampzT(f.CompletedAt.UTC())
	}
	stmt := t.INSERT(t.AllColumns).VALUES(
		f.ID, f.PublicID, f.OrgID, f.ProjectID, f.Purpose, string(f.Visibility), f.ObjectKey,
		f.ContentType, f.SizeBytes, string(f.Status), f.CreatedAt.UTC(), completed, f.ExpiresAt.UTC(),
	)
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		return fmt.Errorf("storage.postgres: Insert: %w", err)
	}
	return nil
}

// MarkReady moves a pending file not yet expired at at to ready; a file already ready is left as is.
func (s *postgresStore) MarkReady(ctx context.Context, id uuid.UUID, at time.Time) error {
	org, err := orgOf(ctx)
	if err != nil {
		return err
	}
	t := s.files
	stmt := t.UPDATE(t.Status, t.CompletedAt).
		SET(postgres.String(string(StatusReady)), postgres.TimestampzT(at.UTC())).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(org)).AND(t.Status.EQ(postgres.String(string(StatusPending)))).
			AND(t.ExpiresAt.GT(postgres.TimestampzT(at.UTC()))))
	n, err := s.exec(ctx, stmt)
	if err != nil {
		return fmt.Errorf("storage.postgres: MarkReady: %w", err)
	}
	if n == 0 {
		return s.expectStatus(ctx, id, StatusReady)
	}
	return nil
}

// MarkDeleting hands a file to the sweep.
func (s *postgresStore) MarkDeleting(ctx context.Context, id uuid.UUID) error {
	org, err := orgOf(ctx)
	if err != nil {
		return err
	}
	t := s.files
	stmt := t.UPDATE(t.Status).SET(postgres.String(string(StatusDeleting))).WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(org)))
	n, err := s.exec(ctx, stmt)
	if err != nil {
		return fmt.Errorf("storage.postgres: MarkDeleting: %w", err)
	}
	if n == 0 {
		return &UploadNotFoundError{PublicID: id.String()}
	}
	return nil
}

// DeleteDeleting removes the file row id once it is marked deleting.
func (s *postgresStore) DeleteDeleting(ctx context.Context, id uuid.UUID) error {
	org, err := orgOf(ctx)
	if err != nil {
		return err
	}
	t := s.files
	stmt := t.DELETE().WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(org)).AND(t.Status.EQ(postgres.String(string(StatusDeleting)))))
	if _, err := s.exec(ctx, stmt); err != nil {
		return fmt.Errorf("storage.postgres: DeleteDeleting: %w", err)
	}
	return nil
}

// LockSweepable locks the file FOR UPDATE when it is still deleting, or pending past its expiry at now; otherwise nil.
func (s *postgresStore) LockSweepable(ctx context.Context, id uuid.UUID, now time.Time) (*File, error) {
	org, err := orgOf(ctx)
	if err != nil {
		return nil, err
	}
	tx, ok := db.CurrentTx(ctx)
	if !ok {
		return nil, errors.New("storage.postgres: LockSweepable needs a unit of work")
	}
	t := s.files
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(org)).AND(sweepable(t, now))).
		FOR(postgres.UPDATE())
	var row fileRow
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		if db.IsNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("storage.postgres: LockSweepable: %w", err)
	}
	return row.toFile(), nil
}

func orgOf(ctx context.Context) (postgres.StringExpression, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	return postgres.UUID(tc.OrgID), nil
}

func (s *postgresStore) exec(ctx context.Context, stmt postgres.Statement) (int64, error) {
	var n int64
	err := db.Write(ctx, s.pool, func(q qrm.DB) error {
		res, err := stmt.ExecContext(ctx, q)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	})
	return n, err
}

func (s *postgresStore) expectStatus(ctx context.Context, id uuid.UUID, want Status) error {
	t := s.files
	stmt := postgres.SELECT(t.Status).FROM(t).WHERE(t.ID.EQ(postgres.UUID(id)))
	var row struct {
		Status string `alias:"files.status"`
	}
	err := db.Write(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) })
	switch {
	case db.IsNoRows(err):
		return &UploadNotFoundError{PublicID: id.String()}
	case err != nil:
		return fmt.Errorf("storage.postgres: status: %w", err)
	case Status(row.Status) != want:
		return &UploadNotFoundError{PublicID: id.String()}
	}
	return nil
}
