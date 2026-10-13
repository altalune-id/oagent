package agent

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

func TestAgentRow_ToAgent(t *testing.T) {
	createdBy := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	in := agentRow{
		ID:          uuid.New(),
		PublicID:    "p",
		OrgID:       uuid.New(),
		ProjectID:   uuid.New(),
		Slug:        "s",
		Name:        "n",
		Description: "d",
		CreatedBy:   createdBy,
		CreatedAt:   time.Unix(1, 0),
		UpdatedAt:   time.Unix(2, 0),
	}

	a := in.toAgent()
	assert.Equal(t, in.ID, a.ID)
	assert.Equal(t, in.OrgID, a.OrgID)
	assert.Equal(t, in.ProjectID, a.ProjectID)
	assert.Equal(t, "s", a.Slug)
	assert.Equal(t, "n", a.Name)
	assert.Equal(t, "d", a.Description)
	assert.Equal(t, createdBy, a.CreatedBy)
}

func TestNewStore(t *testing.T) {
	assert.NotNil(t, NewStore(db.Pool{}, "public", ""))
}

func agentRowColumns() []string {
	return []string{"agents.id", "agents.public_id", "agents.org_id", "agents.project_id", "agents.slug", "agents.name", "agents.description", "agents.created_by", "agents.created_at", "agents.updated_at"}
}

func TestList(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	id := uuid.New()
	rows := sqlmock.NewRows(agentRowColumns()).
		AddRow(id, "p", uuid.New(), uuid.New(), "acme", "Acme", "d", nil, time.Now(), time.Now())
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, agents: entity.NewAgents("public", "")}
	got, err := s.List(context.Background(), uuid.New())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, id, got[0].ID)
}

func TestByID_NotFound(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(agentRowColumns()))

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, agents: entity.NewAgents("public", "")}
	_, err = s.ByID(context.Background(), uuid.New())
	assert.True(t, IsNotFoundError(err))
}

func TestSave(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(0, 1))

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, agents: entity.NewAgents("public", "")}
	a, err := New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.Nil) // genesis: exercises the NULL created_by branch
	require.NoError(t, err)
	assert.NoError(t, s.Save(context.Background(), a))
}

func TestSave_AlreadyExists(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectExec("INSERT").WillReturnError(&pgconn.PgError{Code: "23505"})

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, agents: entity.NewAgents("public", "")}
	a, err := New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.New())
	require.NoError(t, err)
	assert.True(t, IsAlreadyExistsError(s.Save(context.Background(), a)))
}

func TestDelete(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(0, 1))

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, agents: entity.NewAgents("public", "")}
	assert.NoError(t, s.Delete(context.Background(), uuid.New()))
}

func TestDelete_NotFound(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(0, 0))

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, agents: entity.NewAgents("public", "")}
	assert.True(t, IsNotFoundError(s.Delete(context.Background(), uuid.New())))
}

func TestStore_QueryErrors(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose()
	require.NoError(t, sqlDB.Close())

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, agents: entity.NewAgents("public", "")}
	ctx := context.Background()

	_, err = s.ByID(ctx, uuid.New())
	assert.Error(t, err)
	_, err = s.List(ctx, uuid.New())
	assert.Error(t, err)
	assert.Error(t, s.Delete(ctx, uuid.New()))

	a, err := New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.New())
	require.NoError(t, err)
	assert.Error(t, s.Save(ctx, a))
}
