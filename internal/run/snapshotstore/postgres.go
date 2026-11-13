// Package snapshotstore holds the run.SnapshotStore drivers.
package snapshotstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/schema/entity"
)

// Postgres keeps snapshots in the thread_snapshots table.
type Postgres struct {
	pool db.Pool
	t    *entity.ThreadSnapshots
}

var _ run.SnapshotStore = (*Postgres)(nil)

// NewPostgres builds the Postgres driver over pool.
func NewPostgres(pool db.Pool, schema, tablePrefix string) *Postgres {
	if schema == "" {
		schema = "public"
	}
	return &Postgres{pool: pool, t: entity.NewThreadSnapshots(schema, tablePrefix)}
}

type snapshotRow struct {
	ThreadID       uuid.UUID `alias:"thread_snapshots.thread_id"`
	OrgID          uuid.UUID `alias:"thread_snapshots.org_id"`
	ProjectID      uuid.UUID `alias:"thread_snapshots.project_id"`
	AgentVersionID uuid.UUID `alias:"thread_snapshots.agent_version_id"`
	SDKVersion     string    `alias:"thread_snapshots.sdk_version"`
	Blob           []byte    `alias:"thread_snapshots.blob"`
	UpdatedAt      time.Time `alias:"thread_snapshots.updated_at"`
}

// Load returns the thread's snapshot, or nil when it has none.
func (s *Postgres) Load(ctx context.Context, threadID uuid.UUID) (*run.Snapshot, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.t
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.ThreadID.EQ(postgres.UUID(threadID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	var row snapshotRow
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("snapshotstore.postgres: Load: %w", err)
	}
	return &run.Snapshot{
		ThreadID: row.ThreadID, OrgID: row.OrgID, ProjectID: row.ProjectID, AgentVersionID: row.AgentVersionID,
		SDKVersion: row.SDKVersion, Blob: row.Blob, UpdatedAt: row.UpdatedAt.UTC(),
	}, nil
}

// Save stores snap as its thread's snapshot, replacing any earlier one.
func (s *Postgres) Save(ctx context.Context, snap *run.Snapshot) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	if snap.OrgID != tc.OrgID {
		// SECURITY: without RLS in force, a row naming another org would otherwise land in it.
		return &run.ThreadNotFoundError{ID: snap.ThreadID.String()}
	}
	now := time.Now().UTC()
	t := s.t
	stmt := t.INSERT(t.AllColumns).
		VALUES(snap.ThreadID, snap.OrgID, snap.ProjectID, snap.AgentVersionID, snap.SDKVersion, postgres.Bytea(snap.Blob), now).
		ON_CONFLICT(t.ThreadID).
		// SECURITY: the conflict clause is guarded by org, or a snapshot naming another tenant's thread would rewrite it.
		DO_UPDATE(postgres.SET(
			t.AgentVersionID.SET(postgres.UUID(snap.AgentVersionID)),
			t.SDKVersion.SET(postgres.String(snap.SDKVersion)),
			t.Blob.SET(postgres.Bytea(snap.Blob)),
			t.UpdatedAt.SET(postgres.TimestampzT(now)),
		).WHERE(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	var res sql.Result
	err = db.Write(ctx, s.pool, func(q qrm.DB) (err error) { res, err = stmt.ExecContext(ctx, q); return err })
	if db.IsRowSecurityViolation(err) {
		return &run.ThreadNotFoundError{ID: snap.ThreadID.String()}
	}
	if err != nil {
		return fmt.Errorf("snapshotstore.postgres: Save: %w", err)
	}
	// NOTE: zero rows means the org guard refused; Postgres 17.12 and later refuse with a row-security error before this.
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
		return &run.ThreadNotFoundError{ID: snap.ThreadID.String()}
	}
	snap.UpdatedAt = now
	return nil
}

// Delete removes the thread's snapshot; a thread without one is not an error.
func (s *Postgres) Delete(ctx context.Context, threadID uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.t
	stmt := t.DELETE().WHERE(t.ThreadID.EQ(postgres.UUID(threadID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		return fmt.Errorf("snapshotstore.postgres: Delete: %w", err)
	}
	return nil
}
