package member

import (
	"context"
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

func membershipColumns() []string {
	return []string{
		"memberships.id", "memberships.public_id", "memberships.org_id",
		"memberships.user_id", "memberships.role", "memberships.created_at",
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

func TestAdd(t *testing.T) {
	s, wMock, _ := newMockStore(t)
	wMock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(0, 1))
	m, err := New(uuid.New(), uuid.New(), RoleOwner)
	require.NoError(t, err)
	require.NoError(t, s.Add(context.Background(), m))
	assert.NoError(t, wMock.ExpectationsWereMet())
}

func TestAdd_AlreadyMember(t *testing.T) {
	s, wMock, _ := newMockStore(t)
	wMock.ExpectExec("INSERT").WillReturnError(&pgconn.PgError{Code: "23505"})
	m, err := New(uuid.New(), uuid.New(), RoleMember)
	require.NoError(t, err)
	err = s.Add(context.Background(), m)
	require.True(t, IsAlreadyMemberError(err))
	ae, _ := errors.AsType[*AlreadyMemberError](err)
	assert.Equal(t, m.OrgID, ae.OrgID)
	assert.Equal(t, m.UserID, ae.UserID)
}

func TestFind_UsesWriter(t *testing.T) {
	s, wMock, rMock := newMockStore(t)
	org, usr := uuid.New(), uuid.New()
	wMock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(membershipColumns()).
		AddRow(uuid.New(), "p", org, usr, "admin", time.Now()))
	m, err := s.Find(context.Background(), org, usr)
	require.NoError(t, err)
	assert.Equal(t, RoleAdmin, m.Role)
	assert.Equal(t, org, m.OrgID)
	assert.NoError(t, rMock.ExpectationsWereMet(), "authorization reads never go to a replica")
}

func TestFind_NotFound(t *testing.T) {
	s, wMock, _ := newMockStore(t)
	wMock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(membershipColumns()))
	_, err := s.Find(context.Background(), uuid.New(), uuid.New())
	assert.True(t, IsNotFoundError(err))
}

func TestListByUser(t *testing.T) {
	s, wMock, _ := newMockStore(t)
	usr := uuid.New()
	wMock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(membershipColumns()).
		AddRow(uuid.New(), "a", uuid.New(), usr, "owner", time.Unix(1, 0)).
		AddRow(uuid.New(), "b", uuid.New(), usr, "member", time.Unix(2, 0)))
	got, err := s.ListByUser(context.Background(), usr)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, RoleOwner, got[0].Role)
	assert.Equal(t, RoleMember, got[1].Role)
}

func TestCountOwners(t *testing.T) {
	s, wMock, _ := newMockStore(t)
	wMock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	n, err := s.CountOwners(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestStore_QueryErrors(t *testing.T) {
	s, wMock, _ := newMockStore(t)
	boom := errors.New("boom")
	wMock.ExpectExec("INSERT").WillReturnError(boom)
	wMock.ExpectQuery("SELECT").WillReturnError(boom)
	wMock.ExpectQuery("SELECT").WillReturnError(boom)
	wMock.ExpectQuery("SELECT").WillReturnError(boom)

	m, _ := New(uuid.New(), uuid.New(), RoleMember)
	assert.ErrorIs(t, s.Add(context.Background(), m), boom)
	_, err := s.Find(context.Background(), uuid.New(), uuid.New())
	assert.ErrorIs(t, err, boom)
	_, err = s.ListByUser(context.Background(), uuid.New())
	assert.ErrorIs(t, err, boom)
	_, err = s.CountOwners(context.Background(), uuid.New())
	assert.ErrorIs(t, err, boom)
}
