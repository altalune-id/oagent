//go:build integration

package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/storage"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/nanoid"
)

type fixture struct {
	db    *sql.DB
	pool  db.Pool
	store storage.Store
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool, sqlDB, schemaName := pgtest.MigratedRLS(t)
	return &fixture{db: sqlDB, pool: pool, store: storage.NewStore(pool, schemaName, "")}
}

type seeded struct {
	ctx     context.Context
	org     uuid.UUID
	project uuid.UUID
}

func (f *fixture) seed(t *testing.T) seeded {
	t.Helper()
	org, project := uuid.New(), uuid.New()
	now := time.Now().UTC()
	f.exec(t, `INSERT INTO orgs (id, public_id, slug, name, created_at, updated_at) VALUES ($1, $2, $2, 'Org', $3, $3)`, org, org.String(), now)
	f.exec(t, `INSERT INTO projects (id, public_id, org_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, $2, 'P', $4, $4)`,
		project, project.String(), org, now)
	return seeded{ctx: tenant.Into(t.Context(), tenant.Context{OrgID: org}), org: org, project: project}
}

func (f *fixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	_, err := f.db.ExecContext(t.Context(), q, args...)
	require.NoError(t, err)
}

func newFile(s seeded, status storage.Status, expires time.Time) *storage.File {
	pub := nanoid.MustNew(21)
	return &storage.File{
		ID: uuid.New(), PublicID: pub, OrgID: s.org, ProjectID: s.project,
		Purpose: "knowledge", Visibility: storage.Private, ObjectKey: "private/oagent/projects/" + s.project.String() + "/knowledge/" + pub + ".pdf",
		ContentType: "application/pdf", SizeBytes: 100, Status: status,
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond), ExpiresAt: expires.UTC().Truncate(time.Microsecond),
	}
}

func TestStore_InsertAndReadBack(t *testing.T) {
	f := newFixture(t)
	s := f.seed(t)
	in := newFile(s, storage.StatusPending, time.Now().Add(time.Hour))
	require.NoError(t, f.store.Insert(s.ctx, in))

	got, err := f.store.ByID(s.ctx, in.ID)
	require.NoError(t, err)
	assert.Equal(t, in, got)

	_, err = f.store.ByID(s.ctx, uuid.New())
	assert.True(t, storage.IsUploadNotFoundError(err))
}

// SECURITY: a file of one org is invisible and untouchable from another org's scope.
func TestStore_IsTenantScoped(t *testing.T) {
	f := newFixture(t)
	a, b := f.seed(t), f.seed(t)
	file := newFile(a, storage.StatusPending, time.Now().Add(time.Hour))
	require.NoError(t, f.store.Insert(a.ctx, file))

	_, err := f.store.ByID(b.ctx, file.ID)
	assert.True(t, storage.IsUploadNotFoundError(err))
	assert.True(t, storage.IsUploadNotFoundError(f.store.MarkReady(b.ctx, file.ID, time.Now())))
	assert.True(t, storage.IsUploadNotFoundError(f.store.MarkDeleting(b.ctx, file.ID)))
	require.NoError(t, f.store.DeleteDeleting(b.ctx, file.ID))
	got, err := f.store.ByID(a.ctx, file.ID)
	require.NoError(t, err)
	assert.Equal(t, storage.StatusPending, got.Status)

	foreign := newFile(a, storage.StatusPending, time.Now().Add(time.Hour))
	assert.Error(t, f.store.Insert(b.ctx, foreign), "a row naming another org is refused by the policy")
}

func TestStore_StatusTransitions(t *testing.T) {
	f := newFixture(t)
	s := f.seed(t)
	file := newFile(s, storage.StatusPending, time.Now().Add(time.Hour))
	require.NoError(t, f.store.Insert(s.ctx, file))

	at := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, f.store.MarkReady(s.ctx, file.ID, at))
	require.NoError(t, f.store.MarkReady(s.ctx, file.ID, at.Add(time.Hour)), "ready twice is fine")
	got, err := f.store.ByID(s.ctx, file.ID)
	require.NoError(t, err)
	assert.Equal(t, storage.StatusReady, got.Status)
	require.NotNil(t, got.CompletedAt)
	assert.Equal(t, at, *got.CompletedAt, "the first completion time stays")

	require.NoError(t, f.store.MarkDeleting(s.ctx, file.ID))
	assert.True(t, storage.IsUploadNotFoundError(f.store.MarkReady(s.ctx, file.ID, at)), "a deleting file is never attached")
	assert.True(t, storage.IsUploadNotFoundError(f.store.MarkReady(s.ctx, uuid.New(), at)))
	assert.True(t, storage.IsUploadNotFoundError(f.store.MarkDeleting(s.ctx, uuid.New())))
}

func TestStore_MarkReadyJoinsTheCallersUnitOfWork(t *testing.T) {
	f := newFixture(t)
	s := f.seed(t)
	file := newFile(s, storage.StatusPending, time.Now().Add(time.Hour))
	require.NoError(t, f.store.Insert(s.ctx, file))

	rollback := errors.New("rollback")
	err := db.RunInTx(s.ctx, f.pool, func(ctx context.Context) error {
		require.NoError(t, f.store.MarkReady(ctx, file.ID, time.Now()))
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	got, err := f.store.ByID(s.ctx, file.ID)
	require.NoError(t, err)
	assert.Equal(t, storage.StatusPending, got.Status, "the domain's rollback undoes the attach")
}

func TestStore_SweepableAndDelete(t *testing.T) {
	f := newFixture(t)
	s := f.seed(t)
	now := time.Now().UTC()
	expired := newFile(s, storage.StatusPending, now.Add(-time.Minute))
	fresh := newFile(s, storage.StatusPending, now.Add(time.Hour))
	deleting := newFile(s, storage.StatusDeleting, now.Add(time.Hour))
	ready := newFile(s, storage.StatusReady, now.Add(-time.Hour))
	for _, file := range []*storage.File{expired, fresh, deleting, ready} {
		require.NoError(t, f.store.Insert(s.ctx, file))
	}

	got, err := f.store.Sweepable(s.ctx, now, 10)
	require.NoError(t, err)
	ids := []uuid.UUID{}
	for _, g := range got {
		ids = append(ids, g.ID)
	}
	assert.ElementsMatch(t, []uuid.UUID{expired.ID, deleting.ID}, ids)

	got, err = f.store.Sweepable(s.ctx, now, 1)
	require.NoError(t, err)
	assert.Len(t, got, 1)

	require.NoError(t, f.store.DeleteDeleting(s.ctx, expired.ID))
	_, err = f.store.ByID(s.ctx, expired.ID)
	require.NoError(t, err, "a row not yet claimed for deletion stays")
	require.NoError(t, f.store.DeleteDeleting(s.ctx, deleting.ID))
	_, err = f.store.ByID(s.ctx, deleting.ID)
	assert.True(t, storage.IsUploadNotFoundError(err))
}

func TestSchema_FileRules(t *testing.T) {
	f := newFixture(t)
	a, b := f.seed(t), f.seed(t)
	file := newFile(a, storage.StatusReady, time.Now().Add(time.Hour))
	require.NoError(t, f.store.Insert(a.ctx, file))

	dup := newFile(a, storage.StatusPending, time.Now().Add(time.Hour))
	dup.ObjectKey = file.ObjectKey
	err := f.store.Insert(a.ctx, dup)
	require.Error(t, err)
	assert.True(t, db.IsUniqueViolation(err), "one row per object key")

	crossed := newFile(a, storage.StatusPending, time.Now().Add(time.Hour))
	crossed.ProjectID = b.project
	assert.Error(t, f.store.Insert(a.ctx, crossed), "a file belongs to a project of its own org")

	bad := newFile(a, "done", time.Now())
	assert.Error(t, f.store.Insert(a.ctx, bad), "status is checked")

	f.exec(t, `DELETE FROM projects WHERE id = $1`, a.project)
	var n int
	require.NoError(t, f.db.QueryRowContext(t.Context(), `SELECT count(*) FROM files WHERE org_id = $1`, a.org).Scan(&n))
	assert.Zero(t, n, "files go with their project")
}
