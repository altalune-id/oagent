package outbox

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/schema/entity"
)

func entryColumns() []string {
	return []string{
		"outbox_entries.id", "outbox_entries.event_id", "outbox_entries.org_id", "outbox_entries.project_id",
		"outbox_entries.target", "outbox_entries.payload", "outbox_entries.attempt", "outbox_entries.next_attempt_at",
		"outbox_entries.status", "outbox_entries.last_error", "outbox_entries.created_at", "outbox_entries.delivered_at",
	}
}

func newTestStore(t *testing.T) (*postgresStore, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	return &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, entries: entity.NewOutboxEntries("public", "")}, mock, sqlDB
}

func tenantedCtx(org, project uuid.UUID) context.Context {
	return tenant.Into(context.Background(), tenant.Context{OrgID: org, ProjectID: project})
}

func TestPgEntryRow_ToEntry(t *testing.T) {
	in := entryRow{
		ID: uuid.New(), EventID: uuid.New(), OrgID: uuid.New(), ProjectID: uuid.New(),
		Target: "ep", Payload: []byte("{}"), Attempt: 3,
		NextAttemptAt: time.Unix(10, 0), Status: string(StatusPending), LastError: "",
		CreatedAt: time.Unix(1, 0),
	}
	got := in.toEntry()
	assert.Equal(t, in.ID, got.ID)
	assert.Equal(t, in.EventID, got.EventID)
	assert.Equal(t, in.OrgID, got.OrgID)
	assert.Equal(t, in.ProjectID, got.ProjectID)
	assert.Equal(t, "ep", got.Target)
	assert.Equal(t, 3, got.Attempt)
	assert.Equal(t, StatusPending, got.Status)
	assert.Equal(t, in.CreatedAt.UTC(), got.CreatedAt)
}

func TestNewStoreNotNil(t *testing.T) {
	assert.NotNil(t, NewStore(db.Pool{}, "public", ""))
}

func TestWritesRequireATenant(t *testing.T) {
	s, _, _ := newTestStore(t)
	err := s.Requeue(context.Background(), uuid.New(), "t")
	assert.True(t, tenant.IsMissingError(err))
}

func lockedRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"outbox_entries.id"}).AddRow(uuid.New())
}

var errBoom = errors.New("boom")

func TestEnqueue(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	ctx := tenantedCtx(org, project)

	t.Run("invalid entry", func(t *testing.T) {
		s, _, _ := newTestStore(t)
		assert.True(t, IsInvalidEntryError(s.Enqueue(ctx, Entry{})))
	})

	t.Run("org mismatch", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		mock.ExpectBegin()
		mock.ExpectRollback()
		e := Entry{ID: uuid.New(), EventID: uuid.New(), OrgID: uuid.New(), ProjectID: project, Target: "t"}
		assert.True(t, IsInvalidEntryError(s.Enqueue(ctx, e)))
	})

	t.Run("project mismatch", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		mock.ExpectBegin()
		mock.ExpectRollback()
		e := Entry{ID: uuid.New(), EventID: uuid.New(), OrgID: org, ProjectID: uuid.New(), Target: "t"}
		assert.True(t, IsInvalidEntryError(s.Enqueue(ctx, e)))
	})

	t.Run("inserts", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		e := Entry{ID: uuid.New(), PublicID: "d", EventID: uuid.New(), EventPublicID: "e", OrgID: org, ProjectID: project, Target: "t", Payload: []byte("{}")}
		assert.NoError(t, s.Enqueue(ctx, e))
	})
}

func TestClaimDue(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	ctx := tenantedCtx(org, project)

	t.Run("claims rows", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		rows := sqlmock.NewRows(entryColumns()).
			AddRow(uuid.New(), uuid.New(), org, project, "ep", []byte("{}"), 1, time.Now(), string(StatusPending), "", time.Now(), nil)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 0)) // reap
		mock.ExpectQuery("SELECT").WillReturnRows(lockedRows())             // lock
		mock.ExpectQuery("UPDATE").WillReturnRows(rows)                     // claim
		mock.ExpectCommit()

		got, err := s.ClaimDue(ctx, time.Now(), 10)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "ep", got[0].Target)
	})

	t.Run("claim error rolls back", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT").WillReturnRows(lockedRows())
		mock.ExpectQuery("UPDATE").WillReturnError(errBoom)
		mock.ExpectRollback()
		_, err := s.ClaimDue(ctx, time.Now(), 10)
		assert.Error(t, err)
	})

	t.Run("lock error rolls back", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT").WillReturnError(errBoom)
		mock.ExpectRollback()
		_, err := s.ClaimDue(ctx, time.Now(), 10)
		assert.Error(t, err)
	})

	t.Run("nothing due claims nothing", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"outbox_entries.id"}))
		mock.ExpectCommit()
		got, err := s.ClaimDue(ctx, time.Now(), 10)
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("reap error rolls back", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnError(errBoom)
		mock.ExpectRollback()
		_, err := s.ClaimDue(ctx, time.Now(), 10)
		assert.Error(t, err)
	})
}

func TestSucceed(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	ctx := tenantedCtx(org, project)
	e := Entry{ID: uuid.New(), OrgID: org, ProjectID: project, Target: "ep", Attempt: 1}

	t.Run("settles", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		assert.NoError(t, s.Succeed(ctx, e, time.Now()))
	})

	t.Run("stale claim", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		rows := sqlmock.NewRows([]string{"outbox_entries.status", "outbox_entries.attempt"}).AddRow(string(StatusPending), 2)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT").WillReturnRows(rows)
		mock.ExpectCommit()
		assert.True(t, IsStaleClaimError(s.Succeed(ctx, e, time.Now())))
	})

	t.Run("terminal", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		rows := sqlmock.NewRows([]string{"outbox_entries.status", "outbox_entries.attempt"}).AddRow(string(StatusDelivered), 1)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT").WillReturnRows(rows)
		mock.ExpectCommit()
		assert.True(t, IsTerminalStateError(s.Succeed(ctx, e, time.Now())))
	})

	t.Run("missing", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		rows := sqlmock.NewRows([]string{"outbox_entries.status", "outbox_entries.attempt"})
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT").WillReturnRows(rows)
		mock.ExpectCommit()
		assert.True(t, IsNotFoundError(s.Succeed(ctx, e, time.Now())))
	})

	t.Run("settle error", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnError(errBoom)
		mock.ExpectRollback()
		assert.Error(t, s.Succeed(ctx, e, time.Now()))
	})

	t.Run("refuse read error", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT").WillReturnError(errBoom)
		mock.ExpectCommit()
		assert.Error(t, s.Succeed(ctx, e, time.Now()))
	})
}

func TestFail(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	s, mock, _ := newTestStore(t)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	assert.NoError(t, s.Fail(tenantedCtx(org, project), Entry{ID: uuid.New(), OrgID: org, ProjectID: project, Target: "ep", Attempt: 1}, time.Now(), "boom"))
}

func TestRequeue(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	ctx := tenantedCtx(org, project)
	id := uuid.New()

	t.Run("requeues failed", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		assert.NoError(t, s.Requeue(ctx, id, "ep"))
	})

	t.Run("not failed", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		rows := sqlmock.NewRows([]string{"outbox_entries.status"}).AddRow(string(StatusPending))
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT").WillReturnRows(rows)
		mock.ExpectCommit()
		assert.True(t, IsNotFailedError(s.Requeue(ctx, id, "ep")))
	})

	t.Run("missing", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		rows := sqlmock.NewRows([]string{"outbox_entries.status"})
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT").WillReturnRows(rows)
		mock.ExpectCommit()
		assert.True(t, IsNotFoundError(s.Requeue(ctx, id, "ep")))
	})
}

func TestRequeueFailed(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	s, mock, _ := newTestStore(t)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()
	n, err := s.RequeueFailed(tenantedCtx(org, project), "ep")
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestByID(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	ctx := tenantedCtx(org, project)

	t.Run("found", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		id := uuid.New()
		rows := sqlmock.NewRows(entryColumns()).
			AddRow(id, uuid.New(), org, project, "ep", []byte("{}"), 0, time.Now(), string(StatusPending), "", time.Now(), nil)
		mock.ExpectQuery("SELECT").WillReturnRows(rows)
		got, err := s.ByID(ctx, id, "ep")
		require.NoError(t, err)
		assert.Equal(t, id, got.ID)
	})

	t.Run("not found", func(t *testing.T) {
		s, mock, _ := newTestStore(t)
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(entryColumns()))
		_, err := s.ByID(ctx, uuid.New(), "ep")
		assert.True(t, IsNotFoundError(err))
	})
}

func TestListByTarget(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	s, mock, _ := newTestStore(t)
	rows := sqlmock.NewRows(entryColumns()).
		AddRow(uuid.New(), uuid.New(), org, project, "ep", []byte("{}"), 0, time.Now(), string(StatusPending), "", time.Now(), nil)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)
	got, err := s.ListByTarget(tenantedCtx(org, project), "ep", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestStoreQueryErrors(t *testing.T) {
	s, mock, sqlDB := newTestStore(t)
	mock.ExpectClose()
	require.NoError(t, sqlDB.Close())
	ctx := tenantedCtx(uuid.New(), uuid.New())
	e := Entry{ID: uuid.New(), EventID: uuid.New(), OrgID: uuid.New(), ProjectID: uuid.New(), Target: "ep"}

	assert.Error(t, s.Enqueue(ctx, e))
	_, err := s.ClaimDue(ctx, time.Now(), 10)
	assert.Error(t, err)
	_, err = s.ByID(ctx, uuid.New(), "ep")
	assert.Error(t, err)
	_, err = s.ListByTarget(ctx, "ep", 10)
	assert.Error(t, err)
}
