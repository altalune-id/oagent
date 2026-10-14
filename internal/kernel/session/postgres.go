package session

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
	"altalune.id/oagent/sealer"
)

const deleteExpiredBatch = 500

// PostgresStore keeps sealed sessions in the sessions table, always on the primary.
type PostgresStore struct {
	db         *sql.DB
	table      *entity.Sessions
	sealer     sealer.Sealer
	unexpected apperror.UnexpectedFunc
	batch      int
}

// NewPostgresStore builds a PostgresStore; conn must be the writer pool so a fresh sign-in is never missed on a lagging replica.
func NewPostgresStore(conn *sql.DB, schema, tablePrefix string, s sealer.Sealer, unexpected apperror.UnexpectedFunc) *PostgresStore {
	return &PostgresStore{
		db: conn, table: entity.NewSessions(schema, tablePrefix), sealer: s, unexpected: unexpected, batch: deleteExpiredBatch,
	}
}

// Save upserts the sealed principal under sid.
func (s *PostgresStore) Save(ctx context.Context, sid string, p Principal, expiresAt time.Time) error {
	blob, err := sealPrincipal(s.sealer, sid, p.withoutAuthority())
	if err != nil {
		return err
	}
	t := s.table
	stmt := t.INSERT(t.SID, t.UserID, t.Payload, t.ExpiresAt, t.CreatedAt).
		VALUES(sid, p.UserID, blob, expiresAt.UTC(), time.Now().UTC()).
		ON_CONFLICT(t.SID).
		DO_UPDATE(postgres.SET(
			t.UserID.SET(postgres.UUID(p.UserID)),
			t.Payload.SET(postgres.Bytea(blob)),
			t.ExpiresAt.SET(postgres.TimestampzT(expiresAt.UTC())),
		))
	if _, err := stmt.ExecContext(ctx, s.db); err != nil {
		return fmt.Errorf("session.postgres: Save: %w", err)
	}
	return nil
}

// Load returns the live session for sid; an expired, missing or unopenable row reads as signed out.
func (s *PostgresStore) Load(ctx context.Context, sid string) (Principal, bool, error) {
	t := s.table
	stmt := postgres.SELECT(t.Payload).FROM(t).
		WHERE(t.SID.EQ(postgres.String(sid)).AND(t.ExpiresAt.GT(postgres.NOW()))).
		LIMIT(1)
	var row struct {
		Payload []byte `alias:"sessions.payload"`
	}
	if err := stmt.QueryContext(ctx, s.db, &row); err != nil {
		if db.IsNoRows(err) {
			return Principal{}, false, nil
		}
		return Principal{}, false, fmt.Errorf("session.postgres: Load: %w", err)
	}
	p, err := openPrincipal(s.sealer, sid, row.Payload)
	if err != nil {
		// SECURITY: a rotated key or a changed Principal shape signs the holder out instead of failing the request.
		_ = s.unexpected(ctx, "session: open principal", err)
		return Principal{}, false, nil //nolint:nilerr // an unopenable session means "not signed in".
	}
	return p, true, nil
}

// Delete removes the session for sid.
func (s *PostgresStore) Delete(ctx context.Context, sid string) error {
	if _, err := s.table.DELETE().WHERE(s.table.SID.EQ(postgres.String(sid))).ExecContext(ctx, s.db); err != nil {
		return fmt.Errorf("session.postgres: Delete: %w", err)
	}
	return nil
}

// DeleteByUser removes every session of userID; it joins the transaction on ctx when there is one.
func (s *PostgresStore) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	var conn qrm.DB = s.db
	if tx, ok := db.CurrentTx(ctx); ok {
		conn = tx
	}
	if _, err := s.table.DELETE().WHERE(s.table.UserID.EQ(postgres.UUID(userID))).ExecContext(ctx, conn); err != nil {
		return fmt.Errorf("session.postgres: DeleteByUser: %w", err)
	}
	return nil
}

// DeleteByUserExcept removes every session of userID but keepSID; it joins the transaction on ctx when there is one.
func (s *PostgresStore) DeleteByUserExcept(ctx context.Context, userID uuid.UUID, keepSID string) error {
	var conn qrm.DB = s.db
	if tx, ok := db.CurrentTx(ctx); ok {
		conn = tx
	}
	where := s.table.UserID.EQ(postgres.UUID(userID)).AND(s.table.SID.NOT_EQ(postgres.String(keepSID)))
	if _, err := s.table.DELETE().WHERE(where).ExecContext(ctx, conn); err != nil {
		return fmt.Errorf("session.postgres: DeleteByUserExcept: %w", err)
	}
	return nil
}

// DeleteExpired removes expired sessions in batches and reports how many went.
func (s *PostgresStore) DeleteExpired(ctx context.Context) (int, error) {
	t := s.table
	total := 0
	for {
		expired := postgres.SELECT(t.SID).FROM(t).
			WHERE(t.ExpiresAt.LT_EQ(postgres.NOW())).
			LIMIT(int64(s.batch))
		res, err := t.DELETE().WHERE(t.SID.IN(expired)).ExecContext(ctx, s.db)
		if err != nil {
			return total, fmt.Errorf("session.postgres: DeleteExpired: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("session.postgres: DeleteExpired: rows affected: %w", err)
		}
		total += int(n)
		if int(n) < s.batch {
			return total, nil
		}
	}
}
