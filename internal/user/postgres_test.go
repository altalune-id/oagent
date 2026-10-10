package user

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
)

func userRowColumns() []string {
	return []string{
		"users.id", "users.public_id", "users.email", "users.name", "users.avatar_url", "users.password_hash",
		"users.idp_issuer", "users.idp_subject", "users.locale", "users.terms_accepted_at", "users.created_at", "users.updated_at",
	}
}

func newMockStore(t *testing.T) (*postgresStore, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()
	w, wMock, err := sqlmock.New()
	require.NoError(t, err)
	r, rMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close(); _ = r.Close() })
	return NewStore(db.Pool{W: w, R: r}, "public", ""), wMock, rMock
}

func TestUserRow_ToUser(t *testing.T) {
	at := time.Unix(3, 0).UTC()
	in := userRow{
		ID: uuid.New(), PublicID: "p", Email: "a@example.com", Name: "A", AvatarURL: "https://x/a.png", PasswordHash: "h",
		IDPIssuer: sql.NullString{String: "https://idp", Valid: true}, IDPSubject: sql.NullString{String: "sub", Valid: true},
		Locale: "id", TermsAcceptedAt: &at, CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(2, 0),
	}
	u := in.toUser()
	assert.Equal(t, User{
		ID: in.ID, PublicID: "p", Email: "a@example.com", Name: "A", AvatarURL: "https://x/a.png", PasswordHash: "h",
		IDPIssuer: "https://idp", IDPSubject: "sub", Locale: "id", TermsAcceptedAt: &at, CreatedAt: in.CreatedAt, UpdatedAt: in.UpdatedAt,
	}, *u)
}

func TestByID_UsesReader(t *testing.T) {
	s, _, rMock := newMockStore(t)
	id := uuid.New()
	rMock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(userRowColumns()).
		AddRow(id, "p", "a@example.com", "", "", "", nil, nil, "", nil, time.Now(), time.Now()))
	got, err := s.ByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, id, got.ID)
	assert.Empty(t, got.IDPIssuer)
	assert.Nil(t, got.TermsAcceptedAt)
	assert.NoError(t, rMock.ExpectationsWereMet())
}

func TestByID_NotFound(t *testing.T) {
	s, _, rMock := newMockStore(t)
	rMock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(userRowColumns()))
	_, err := s.ByID(context.Background(), uuid.New())
	assert.True(t, IsNotFoundError(err))
}

func TestByEmail_UsesWriter(t *testing.T) {
	s, wMock, rMock := newMockStore(t)
	wMock.ExpectQuery("SELECT").WithArgs("alice@example.com", sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows(userRowColumns()).
		AddRow(uuid.New(), "p", "alice@example.com", "", "", "", nil, nil, "", nil, time.Now(), time.Now()))
	got, err := s.ByEmail(context.Background(), " ALICE@example.com ")
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", got.Email)
	assert.NoError(t, wMock.ExpectationsWereMet())
	assert.NoError(t, rMock.ExpectationsWereMet(), "a replica may lag right after install, so no read goes there")
}

func TestByEmail_InvalidIsNotFound(t *testing.T) {
	s, wMock, rMock := newMockStore(t)
	_, err := s.ByEmail(context.Background(), "not an email")
	assert.True(t, IsNotFoundError(err))
	assert.NoError(t, wMock.ExpectationsWereMet())
	assert.NoError(t, rMock.ExpectationsWereMet())
}

func TestSave(t *testing.T) {
	s, wMock, _ := newMockStore(t)
	wMock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(0, 1))
	u, err := New("a@example.com", "A")
	require.NoError(t, err)
	at := time.Now()
	u.IDPIssuer, u.IDPSubject, u.TermsAcceptedAt = "https://idp", "sub", &at
	before := u.UpdatedAt
	require.NoError(t, s.Save(context.Background(), u))
	assert.False(t, u.UpdatedAt.Before(before))
}

func TestSave_AlreadyExists(t *testing.T) {
	s, wMock, _ := newMockStore(t)
	wMock.ExpectExec("INSERT").WillReturnError(&pgconn.PgError{Code: "23505"})
	u, err := New("a@example.com", "A")
	require.NoError(t, err)
	err = s.Save(context.Background(), u)
	assert.True(t, IsAlreadyExistsError(err))
}

func TestStore_QueryErrors(t *testing.T) {
	s, wMock, rMock := newMockStore(t)
	boom := errors.New("boom")
	rMock.ExpectQuery("SELECT").WillReturnError(boom)
	wMock.ExpectQuery("SELECT").WillReturnError(boom)
	wMock.ExpectExec("INSERT").WillReturnError(boom)

	_, err := s.ByID(context.Background(), uuid.New())
	assert.ErrorIs(t, err, boom)
	_, err = s.ByEmail(context.Background(), "a@example.com")
	assert.ErrorIs(t, err, boom)
	u, _ := New("a@example.com", "")
	assert.ErrorIs(t, s.Save(context.Background(), u), boom)
}
