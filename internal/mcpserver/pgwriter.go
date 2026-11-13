package mcpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/schema/entity"
)

var errLockOutsideTx = errors.New("mcpserver.postgres: a row lock needs a transaction")

// Save inserts a server or updates everything but its name; a name taken in the project is an *AlreadyExistsError.
func (s *postgresStore) Save(ctx context.Context, sv *Server) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	if sv.OrgID != tc.OrgID {
		// SECURITY: without RLS in force, a fresh row naming another org would otherwise land in it.
		return &NotFoundError{ID: sv.ID.String()}
	}
	header, sealed := nullableText(sv.AuthHeader), nullableBytea(sv.AuthSealed)
	t := s.servers
	stmt := t.INSERT(t.AllColumns).
		VALUES(
			sv.ID, sv.PublicID, sv.OrgID, sv.ProjectID, sv.Name, sv.URL, string(sv.AuthKind), header, sealed,
			sv.TimeoutMS, sv.Trusted, sv.CreatedAt.UTC(), sv.UpdatedAt.UTC(),
		).
		ON_CONFLICT(t.ID).
		// SECURITY: the conflict clause is guarded by org, or an upsert carrying another tenant's row id would rewrite that row.
		DO_UPDATE(
			postgres.SET(
				t.URL.SET(postgres.String(sv.URL)),
				t.AuthKind.SET(postgres.String(string(sv.AuthKind))),
				t.AuthHeader.SET(header),
				t.AuthSealed.SET(sealed),
				t.TimeoutMS.SET(postgres.Int32(sv.TimeoutMS)),
				t.Trusted.SET(postgres.Bool(sv.Trusted)),
				t.UpdatedAt.SET(postgres.TimestampzT(sv.UpdatedAt.UTC())),
			).WHERE(t.OrgID.EQ(postgres.UUID(tc.OrgID))),
		)
	var res sql.Result
	err = db.Write(ctx, s.pool, func(q qrm.DB) (err error) {
		res, err = stmt.ExecContext(ctx, q)
		return err
	})
	if err != nil {
		if db.IsUniqueViolation(err) {
			return &AlreadyExistsError{Name: sv.Name}
		}
		if db.IsRowSecurityViolation(err) {
			return &NotFoundError{ID: sv.ID.String()}
		}
		return fmt.Errorf("mcpserver.postgres: Save: %w", err)
	}
	// NOTE: zero rows means the conflict clause's org guard refused.
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
		return &NotFoundError{ID: sv.ID.String()}
	}
	return nil
}

func nullableText(v string) postgres.StringExpression {
	if v == "" {
		return entity.NullText()
	}
	return postgres.String(v)
}

func nullableBytea(b []byte) postgres.ByteaExpression {
	if len(b) == 0 {
		return entity.NullBytea()
	}
	return postgres.Bytea(b)
}

// Delete removes a server of the org on ctx by id.
func (s *postgresStore) Delete(ctx context.Context, id uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.servers
	stmt := t.DELETE().WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	var res sql.Result
	err = db.Write(ctx, s.pool, func(q qrm.DB) (err error) {
		res, err = stmt.ExecContext(ctx, q)
		return err
	})
	if err != nil {
		return fmt.Errorf("mcpserver.postgres: Delete: %w", err)
	}
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
		return &NotFoundError{ID: id.String()}
	}
	return nil
}

// LockForUpdate loads a server FOR UPDATE, so a concurrent LockShared or LockForUpdate on it waits for the unit of work on ctx.
func (s *postgresStore) LockForUpdate(ctx context.Context, id uuid.UUID) (*Server, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	tx, ok := db.CurrentTx(ctx)
	if !ok {
		return nil, errLockOutsideTx
	}
	t := s.servers
	stmt := postgres.SELECT(t.AllColumns).
		FROM(t).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		FOR(postgres.UPDATE())
	var row serverRow
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{ID: id.String()}
		}
		return nil, fmt.Errorf("mcpserver.postgres: LockForUpdate: %w", err)
	}
	return row.toServer(), nil
}

// LockShared locks the servers FOR SHARE until the unit of work on ctx ends; a missing one is a *NotFoundError.
func (s *postgresStore) LockShared(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	tx, ok := db.CurrentTx(ctx)
	if !ok {
		return errLockOutsideTx
	}
	t := s.servers
	in := make([]postgres.Expression, len(ids))
	for i, id := range ids {
		in[i] = postgres.UUID(id)
	}
	stmt := postgres.SELECT(t.ID).
		FROM(t).
		WHERE(t.ID.IN(in...).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		ORDER_BY(t.ID.ASC()).
		FOR(postgres.SHARE())
	var found []idRow
	if err := stmt.QueryContext(ctx, tx, &found); err != nil {
		return fmt.Errorf("mcpserver.postgres: LockShared: %w", err)
	}
	for _, id := range ids {
		if !slices.Contains(found, idRow{ID: id}) {
			return &NotFoundError{ID: id.String()}
		}
	}
	return nil
}

type idRow struct {
	ID uuid.UUID `alias:"mcp_servers.id"`
}
