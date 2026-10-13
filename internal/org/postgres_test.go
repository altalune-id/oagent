package org

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

func TestOrgRow_ToOrg(t *testing.T) {
	createdBy := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	in := orgRow{
		ID:        uuid.New(),
		PublicID:  "p",
		Slug:      "s",
		Name:      "n",
		CreatedBy: createdBy,
		CreatedAt: time.Unix(1, 0),
		UpdatedAt: time.Unix(2, 0),
	}

	o := in.toOrg()
	assert.Equal(t, in.ID, o.ID)
	assert.Equal(t, "p", o.PublicID)
	assert.Equal(t, "s", o.Slug)
	assert.Equal(t, "n", o.Name)
	assert.Equal(t, createdBy, o.CreatedBy)
	assert.Equal(t, in.CreatedAt, o.CreatedAt)
	assert.Equal(t, in.UpdatedAt, o.UpdatedAt)
}

func TestNewStore(t *testing.T) {
	assert.NotNil(t, NewStore(db.Pool{}, "public", ""))
}

func orgRowColumns() []string {
	return []string{"orgs.id", "orgs.public_id", "orgs.slug", "orgs.name", "orgs.created_by", "orgs.created_at", "orgs.updated_at", "orgs.is_system"}
}

func TestList(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	id := uuid.New()
	rows := sqlmock.NewRows(orgRowColumns()).
		AddRow(id, "p", "acme", "Acme", nil, time.Now(), time.Now(), false)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, orgs: entity.NewOrgs("public", "")}
	got, err := s.List(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, id, got[0].ID)
	assert.Equal(t, "acme", got[0].Slug)
}

func TestByID_NotFound(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(orgRowColumns()))

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, orgs: entity.NewOrgs("public", "")}
	_, err = s.ByID(context.Background(), uuid.New())
	assert.True(t, IsNotFoundError(err))
}

func TestBySlug(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	rows := sqlmock.NewRows(orgRowColumns()).
		AddRow(uuid.New(), "p", "acme", "Acme", nil, time.Now(), time.Now(), false)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, orgs: entity.NewOrgs("public", "")}
	got, err := s.BySlug(context.Background(), "acme")
	require.NoError(t, err)
	assert.Equal(t, "acme", got.Slug)
}

func TestSave(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(0, 1))

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, orgs: entity.NewOrgs("public", "")}
	o, err := NewOrg("acme", "Acme", uuid.Nil) // genesis: exercises the NULL created_by branch
	require.NoError(t, err)
	assert.NoError(t, s.Save(context.Background(), o))
}

func TestSave_AlreadyExists(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectExec("INSERT").WillReturnError(&pgconn.PgError{Code: "23505"})

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, orgs: entity.NewOrgs("public", "")}
	o, err := NewOrg("acme", "Acme", uuid.New())
	require.NoError(t, err)
	assert.True(t, IsAlreadyExistsError(s.Save(context.Background(), o)))
}

func TestStore_QueryErrors(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose()
	require.NoError(t, sqlDB.Close())

	s := &postgresStore{pool: db.Pool{W: sqlDB, R: sqlDB}, orgs: entity.NewOrgs("public", "")}
	ctx := context.Background()

	_, err = s.ByID(ctx, uuid.New())
	assert.Error(t, err)
	_, err = s.BySlug(ctx, "x")
	assert.Error(t, err)
	_, err = s.ByPublicID(ctx, "x")
	assert.Error(t, err)
	_, err = s.List(ctx)
	assert.Error(t, err)

	o, err := NewOrg("acme", "Acme", uuid.New())
	require.NoError(t, err)
	assert.Error(t, s.Save(ctx, o))
}

func TestSystem(t *testing.T) {
	writer, wMock, err := sqlmock.New()
	require.NoError(t, err)
	reader, rMock, err := sqlmock.New()
	require.NoError(t, err)
	s := &postgresStore{pool: db.Pool{W: writer, R: reader}, orgs: entity.NewOrgs("public", "")}

	wMock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(orgRowColumns()).
		AddRow(uuid.New(), "p", "acme", "Acme", nil, time.Now(), time.Now(), true))
	got, err := s.System(context.Background())
	require.NoError(t, err)
	assert.True(t, got.IsSystem)
	assert.NoError(t, rMock.ExpectationsWereMet(), "the installed marker is read from the primary")

	wMock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(orgRowColumns()))
	_, err = s.System(context.Background())
	assert.True(t, IsNotFoundError(err))
}
