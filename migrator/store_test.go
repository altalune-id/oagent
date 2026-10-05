package migrator

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/pressly/goose/v3/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	applied map[int64]bool
	listErr error
	getErr  error
}

func (f *fakeStore) Tablename() string { return "goose_db_version" }

func (f *fakeStore) TableExists(context.Context, database.DBTxConn) (bool, error) {
	return true, nil
}

func (f *fakeStore) CreateVersionTable(context.Context, database.DBTxConn) error { return nil }

func (f *fakeStore) Insert(context.Context, database.DBTxConn, database.InsertRequest) error {
	return nil
}

func (f *fakeStore) Delete(context.Context, database.DBTxConn, int64) error { return nil }

func (f *fakeStore) GetMigration(_ context.Context, _ database.DBTxConn, version int64) (*database.GetMigrationResult, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.applied[version] {
		return &database.GetMigrationResult{IsApplied: true, Timestamp: time.Unix(0, 0)}, nil
	}
	return nil, database.ErrVersionNotFound
}

func (f *fakeStore) GetLatestVersion(context.Context, database.DBTxConn) (int64, error) {
	return 1, nil
}

func (f *fakeStore) ListMigrations(context.Context, database.DBTxConn) ([]*database.ListMigrationsResult, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return []*database.ListMigrationsResult{{Version: 1, IsApplied: true}, {Version: 0, IsApplied: true}}, nil
}

const testSQL = "-- +goose Up\nCREATE TABLE t (id INT);\n-- +goose Down\nDROP TABLE t;\n"

var testFS = fstest.MapFS{
	"VERSION":      &fstest.MapFile{Data: []byte("1\n")},
	"001_init.sql": &fstest.MapFile{Data: []byte(testSQL)},
}

func appliedStore() *fakeStore {
	return &fakeStore{applied: map[int64]bool{0: true, 1: true}}
}

func newMigrator(t *testing.T, fsys fs.FS, store database.Store) (*Migrator, *sql.DB) {
	t.Helper()
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	m := New(fsys, Config{})
	m.store = store
	return m, db
}

func TestStatus(t *testing.T) {
	m, db := newMigrator(t, testFS, appliedStore())

	rows, err := m.Status(context.Background(), db)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(1), rows[0].Version)
	assert.True(t, rows[0].Applied)
}

func TestStatus_StoreError(t *testing.T) {
	m, db := newMigrator(t, testFS, &fakeStore{getErr: errors.New("boom")})

	_, err := m.Status(context.Background(), db)
	assert.Error(t, err)
}

func TestIsUpToDate(t *testing.T) {
	m, db := newMigrator(t, testFS, appliedStore())

	ok, err := m.IsUpToDate(context.Background(), db)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestIsUpToDate_Pending(t *testing.T) {
	m, db := newMigrator(t, testFS, &fakeStore{applied: map[int64]bool{0: true}})

	ok, err := m.IsUpToDate(context.Background(), db)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestUp(t *testing.T) {
	m, db := newMigrator(t, testFS, appliedStore())
	assert.NoError(t, m.Up(context.Background(), db))
}

func TestUp_ListError(t *testing.T) {
	m, db := newMigrator(t, testFS, &fakeStore{listErr: errors.New("boom")})
	assert.Error(t, m.Up(context.Background(), db))
}

func TestUp_MissingVersionFile(t *testing.T) {
	fsys := fstest.MapFS{"001_init.sql": &fstest.MapFile{Data: []byte(testSQL)}}
	m, db := newMigrator(t, fsys, appliedStore())
	assert.Error(t, m.Up(context.Background(), db))
}

func TestUp_WithUpgrade(t *testing.T) {
	m, db := newMigrator(t, testFS, appliedStore())
	called := false
	m.WithUpgrade("external", func(context.Context, *sql.DB) error {
		called = true
		return nil
	})

	require.NoError(t, m.Up(context.Background(), db))
	assert.True(t, called)
}

func TestUp_UpgradeError(t *testing.T) {
	m, db := newMigrator(t, testFS, appliedStore())
	m.WithUpgrade("external", func(context.Context, *sql.DB) error {
		return errors.New("boom")
	})

	assert.Error(t, m.Up(context.Background(), db))
}

func TestDownTo(t *testing.T) {
	m, db := newMigrator(t, testFS, appliedStore())
	assert.NoError(t, m.DownTo(context.Background(), db, 1))
}

func TestDownTo_ListError(t *testing.T) {
	m, db := newMigrator(t, testFS, &fakeStore{listErr: errors.New("boom")})
	assert.Error(t, m.DownTo(context.Background(), db, 1))
}

func TestIsUpToDate_StatusError(t *testing.T) {
	m, db := newMigrator(t, testFS, &fakeStore{getErr: errors.New("boom")})

	_, err := m.IsUpToDate(context.Background(), db)
	assert.Error(t, err)
}

func TestIsUpToDate_VersionError(t *testing.T) {
	fsys := fstest.MapFS{"001_init.sql": &fstest.MapFile{Data: []byte(testSQL)}}
	m, db := newMigrator(t, fsys, appliedStore())

	_, err := m.IsUpToDate(context.Background(), db)
	assert.Error(t, err)
}

func TestProvider_DefaultStore(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	p, err := New(testFS, Config{}).provider(db)
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestGooseTableName(t *testing.T) {
	table, err := gooseTableName(Config{Schema: "app", TablePrefix: "t_"})
	require.NoError(t, err)
	assert.Equal(t, "app.t_goose_db_version", table)

	table, err = gooseTableName(Config{})
	require.NoError(t, err)
	assert.Equal(t, "public.goose_db_version", table)

	_, err = gooseTableName(Config{Schema: strings.Repeat("a", 64)})
	assert.Error(t, err)

	_, err = gooseTableName(Config{TablePrefix: strings.Repeat("a", 64)})
	assert.Error(t, err)
}

func TestStatus_ProviderError(t *testing.T) {
	m, _ := newMigrator(t, testFS, appliedStore())
	_, err := m.Status(context.Background(), nil)
	assert.Error(t, err)
}

func TestUp_ProviderError(t *testing.T) {
	m, _ := newMigrator(t, testFS, appliedStore())
	assert.Error(t, m.Up(context.Background(), nil))
}

func TestDownTo_ProviderError(t *testing.T) {
	m, _ := newMigrator(t, testFS, appliedStore())
	assert.Error(t, m.DownTo(context.Background(), nil, 0))
}
