package enduser

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

// Save inserts a new end user of the org on ctx.
func (s *postgresStore) Save(ctx context.Context, e *EndUser) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	if e.OrgID != tc.OrgID {
		// SECURITY: without RLS in force, a row naming another org would otherwise land in it.
		return &NotFoundError{ID: e.ID.String()}
	}
	t := s.users
	stmt := t.INSERT(t.AllColumns).
		VALUES(e.ID, e.PublicID, e.OrgID, e.ProjectID, e.CreatedAt.UTC(), e.UpdatedAt.UTC())
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error {
		_, err := stmt.ExecContext(ctx, q)
		return err
	}); err != nil {
		return fmt.Errorf("enduser.postgres: Save: %w", err)
	}
	return nil
}

// AddIdentity links id to e; added is false when the project already knows id, whoever owns it.
func (s *postgresStore) AddIdentity(ctx context.Context, e *EndUser, id Identity) (bool, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return false, err
	}
	if e.OrgID != tc.OrgID {
		// SECURITY: without RLS in force, a row naming another org would otherwise land in it.
		return false, &NotFoundError{ID: e.ID.String()}
	}
	t := s.identities
	stmt := t.INSERT(t.AllColumns).
		VALUES(uuid.Must(uuid.NewV7()), e.OrgID, e.ProjectID, e.ID, string(id.Kind), id.Value, time.Now().UTC()).
		ON_CONFLICT(t.ProjectID, t.Kind, t.Value).
		DO_NOTHING()
	var res sql.Result
	if err := db.Write(ctx, s.pool, func(q qrm.DB) (err error) {
		res, err = stmt.ExecContext(ctx, q)
		return err
	}); err != nil {
		return false, fmt.Errorf("enduser.postgres: AddIdentity: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("enduser.postgres: AddIdentity: %w", err)
	}
	return n == 1, nil
}
