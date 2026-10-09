package webhook

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/kernel/events"
)

func TestSave(t *testing.T) {
	org, project := uuid.New(), uuid.New()

	t.Run("upsert", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, URL: "u", Active: true, EventTypes: []events.Type{events.AgentCreated}}
		assert.NoError(t, s.Save(storeCtx(org, project), e))
	})

	t.Run("org mismatch", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectRollback()
		e := &Endpoint{ID: uuid.New(), OrgID: uuid.New(), ProjectID: project}
		assert.True(t, IsNotFoundError(s.Save(storeCtx(org, project), e)))
	})

	t.Run("no rows", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, EventTypes: []events.Type{events.AgentCreated}}
		assert.True(t, IsNotFoundError(s.Save(storeCtx(org, project), e)))
	})
}

func TestSaveSecrets(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	id := uuid.New()
	next := SealedSecrets{Primary: []byte("n")}

	t.Run("updated", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		assert.NoError(t, s.SaveSecrets(storeCtx(org, project), id, SealedSecrets{Primary: []byte("e")}, next))
	})

	t.Run("conflict", func(t *testing.T) {
		s, mock := newStoreTest(t)
		rows := sqlmock.NewRows([]string{"webhook_endpoints.id"}).AddRow(id)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT").WillReturnRows(rows)
		mock.ExpectCommit()
		assert.True(t, IsSecretConflictError(s.SaveSecrets(storeCtx(org, project), id, SealedSecrets{}, next)))
	})

	t.Run("not found", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"webhook_endpoints.id"}))
		mock.ExpectCommit()
		assert.True(t, IsNotFoundError(s.SaveSecrets(storeCtx(org, project), id, SealedSecrets{}, next)))
	})
}

func TestDelete(t *testing.T) {
	org, project := uuid.New(), uuid.New()

	t.Run("deleted", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		assert.NoError(t, s.Delete(storeCtx(org, project), uuid.New()))
	})

	t.Run("not found", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		assert.True(t, IsNotFoundError(s.Delete(storeCtx(org, project), uuid.New())))
	})
}

func TestSaveAttempt(t *testing.T) {
	org, project := uuid.New(), uuid.New()

	t.Run("inserts", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		a := Attempt{OrgID: org, ProjectID: project, EndpointID: uuid.New(), EventType: events.AgentCreated, ResponseHeaders: nil}
		assert.NoError(t, s.SaveAttempt(storeCtx(org, project), a))
	})

	t.Run("foreign key", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT").WillReturnError(&pgconn.PgError{Code: "23503"})
		mock.ExpectRollback()
		a := Attempt{OrgID: org, ProjectID: project, EndpointID: uuid.New()}
		assert.True(t, IsNotFoundError(s.SaveAttempt(storeCtx(org, project), a)))
	})

	t.Run("scope mismatch", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectRollback()
		a := Attempt{OrgID: uuid.New(), ProjectID: project}
		assert.True(t, IsInvalidAttemptError(s.SaveAttempt(storeCtx(org, project), a)))
	})
}

func TestPgSecondary(t *testing.T) {
	assert.NotNil(t, pgSecondary(nil))
	assert.NotNil(t, pgSecondary([]byte("x")))
}

func TestWriterErrorBranches(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	ctx := storeCtx(org, project)

	t.Run("save exec error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT").WillReturnError(errBoom)
		mock.ExpectRollback()
		e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, EventTypes: []events.Type{events.AgentCreated}}
		assert.Error(t, s.Save(ctx, e))
	})

	t.Run("saveSecrets exec error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnError(errBoom)
		mock.ExpectRollback()
		assert.Error(t, s.SaveSecrets(ctx, uuid.New(), SealedSecrets{}, SealedSecrets{Primary: []byte("n")}))
	})

	t.Run("delete exec error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("DELETE").WillReturnError(errBoom)
		mock.ExpectRollback()
		assert.Error(t, s.Delete(ctx, uuid.New()))
	})

	t.Run("saveAttempt exec error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT").WillReturnError(errBoom)
		mock.ExpectRollback()
		assert.Error(t, s.SaveAttempt(ctx, Attempt{OrgID: org, ProjectID: project, EndpointID: uuid.New()}))
	})

	t.Run("begin error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin().WillReturnError(errBoom)
		assert.Error(t, s.Delete(ctx, uuid.New()))
	})

	t.Run("commit error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit().WillReturnError(errBoom)
		assert.Error(t, s.Delete(ctx, uuid.New()))
	})
}
