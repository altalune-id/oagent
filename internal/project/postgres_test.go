package project

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

func TestProjectRow_ToProject(t *testing.T) {
	createdBy := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	in := projectRow{
		ID:        uuid.New(),
		PublicID:  "p",
		OrgID:     uuid.New(),
		Slug:      "s",
		Name:      "n",
		CreatedBy: createdBy,
		CreatedAt: time.Unix(1, 0),
		UpdatedAt: time.Unix(2, 0),
	}

	p := in.toProject()
	assert.Equal(t, in.ID, p.ID)
	assert.Equal(t, "p", p.PublicID)
	assert.Equal(t, in.OrgID, p.OrgID)
	assert.Equal(t, "s", p.Slug)
	assert.Equal(t, "n", p.Name)
	assert.Equal(t, createdBy, p.CreatedBy)
	assert.Equal(t, in.CreatedAt, p.CreatedAt)
	assert.Equal(t, in.UpdatedAt, p.UpdatedAt)
}

func TestNewStore(t *testing.T) {
	assert.NotNil(t, NewStore(db.Pool{}, "public", ""))
}

func projectRowColumns() []string {
	return []string{"projects.id", "projects.public_id", "projects.org_id", "projects.slug", "projects.name", "projects.created_by", "projects.created_at", "projects.updated_at"}
}

func TestList(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	id := uuid.New()
	rows := sqlmock.NewRows(projectRowColumns()).
		AddRow(id, "p", uuid.New(), "acme", "Acme", nil, time.Now(), time.Now())
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, projects: entity.NewProjects("public", "")}
	got, err := s.List(context.Background(), uuid.New())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, id, got[0].ID)
	assert.Equal(t, "acme", got[0].Slug)
}

func TestByID_NotFound(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(projectRowColumns()))

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, projects: entity.NewProjects("public", "")}
	_, err = s.ByID(context.Background(), uuid.New())
	assert.True(t, IsNotFoundError(err))
}

func TestBySlug(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	rows := sqlmock.NewRows(projectRowColumns()).
		AddRow(uuid.New(), "p", uuid.New(), "acme", "Acme", nil, time.Now(), time.Now())
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, projects: entity.NewProjects("public", "")}
	got, err := s.BySlug(context.Background(), uuid.New(), "acme")
	require.NoError(t, err)
	assert.Equal(t, "acme", got.Slug)
}

func TestSave(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(0, 1))

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, projects: entity.NewProjects("public", "")}
	p, err := NewProject(uuid.New(), "acme", "Acme", uuid.Nil) // genesis: exercises the NULL created_by branch
	require.NoError(t, err)
	assert.NoError(t, s.Save(context.Background(), p))
}

func TestSave_AlreadyExists(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectExec("INSERT").WillReturnError(&pgconn.PgError{Code: "23505"})

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, projects: entity.NewProjects("public", "")}
	p, err := NewProject(uuid.New(), "acme", "Acme", uuid.New())
	require.NoError(t, err)
	assert.True(t, IsAlreadyExistsError(s.Save(context.Background(), p)))
}

func TestStore_QueryErrors(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose()
	require.NoError(t, sqlDB.Close())

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, projects: entity.NewProjects("public", "")}
	ctx := context.Background()

	_, err = s.ByID(ctx, uuid.New())
	assert.Error(t, err)
	_, err = s.BySlug(ctx, uuid.New(), "x")
	assert.Error(t, err)
	_, err = s.List(ctx, uuid.New())
	assert.Error(t, err)

	p, err := NewProject(uuid.New(), "acme", "Acme", uuid.New())
	require.NoError(t, err)
	assert.Error(t, s.Save(ctx, p))
}
