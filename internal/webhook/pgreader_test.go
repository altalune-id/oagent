package webhook

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestByID(t *testing.T) {
	org := uuid.New()
	rows := sqlmock.NewRows(endpointColumns()).AddRow(uuid.New(), "pub", org, uuid.New(), "u", "d", "[]", []byte("p"), nil, true, time.Now(), time.Now())

	t.Run("found", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectQuery("SELECT").WillReturnRows(rows)
		e, err := s.ByID(storeCtx(org, uuid.New()), uuid.New())
		require.NoError(t, err)
		assert.Equal(t, "u", e.URL)
	})

	t.Run("not found", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(endpointColumns()))
		_, err := s.ByID(storeCtx(org, uuid.New()), uuid.New())
		assert.True(t, IsNotFoundError(err))
	})
}

func TestList(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	rows := sqlmock.NewRows(endpointColumns()).AddRow(uuid.New(), "pub", org, project, "u", "", "[]", []byte("p"), nil, true, time.Now(), time.Now())
	s, mock := newStoreTest(t)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)
	got, err := s.List(storeCtx(org, project), org, project)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestListAttempts(t *testing.T) {
	org := uuid.New()
	rows := sqlmock.NewRows(attemptColumns()).AddRow(uuid.New(), org, uuid.New(), uuid.New(), uuid.New(), uuid.New(), "x", 1, 200, "", "", false, "[]", 3, time.Now())
	s, mock := newStoreTest(t)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)
	got, err := s.ListAttempts(storeCtx(org, uuid.New()), uuid.New(), uuid.New())
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestReaderErrorBranches(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	ctx := storeCtx(org, project)

	t.Run("byID query error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectQuery("SELECT").WillReturnError(errBoom)
		_, err := s.ByID(ctx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("list query error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectQuery("SELECT").WillReturnError(errBoom)
		_, err := s.List(ctx, org, project)
		assert.Error(t, err)
	})

	t.Run("list attempts query error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectQuery("SELECT").WillReturnError(errBoom)
		_, err := s.ListAttempts(ctx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})
}
