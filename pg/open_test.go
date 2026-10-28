package pg

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const okDSN = "postgres://user:pass@127.0.0.1:5432/db"

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestOpen_ValidateError(t *testing.T) {
	_, err := Open(context.Background(), Config{}, nil)
	assert.Error(t, err)
}

func TestOpen_BadDSN(t *testing.T) {
	_, err := Open(context.Background(), Config{DSN: "postgres://u:p@h:notaport/db"}, nil)
	assert.Error(t, err)
}

func TestOpen_PingFails(t *testing.T) {
	cfg := Config{
		DSN:            "postgres://u:p@127.0.0.1:1/db",
		ConnectTimeout: 20 * time.Millisecond,
		ConnectBackoff: time.Millisecond,
	}
	_, err := Open(context.Background(), cfg, discardLogger())
	assert.Error(t, err)
}

func TestOpen_Success(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose()

	orig := openFunc
	t.Cleanup(func() { openFunc = orig })
	openFunc = func(Config) (*sql.DB, error) { return sqlDB, nil }

	cfg := Config{
		DSN:             okDSN,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: time.Minute,
	}
	db, err := Open(context.Background(), cfg, discardLogger())
	require.NoError(t, err)
	assert.Equal(t, 5, db.Stats().MaxOpenConnections)
	assert.NoError(t, db.Close())
}

func TestOpenPostgres_NoRole(t *testing.T) {
	sqlDB, err := openPostgres(Config{DSN: okDSN})
	require.NoError(t, err)
	require.NotNil(t, sqlDB)
	assert.NoError(t, sqlDB.Close())
}

func TestOpenPostgres_BadDSN(t *testing.T) {
	_, err := openPostgres(Config{DSN: "postgres://u:p@h:notaport/db"})
	assert.Error(t, err)
}

func TestOpen_NeverLeaksThePassword(t *testing.T) {
	const password = "s3cret-pw-do-not-print"
	for name, dsn := range map[string]string{
		"unreachable": "postgres://app:" + password + "@127.0.0.1:1/db?sslmode=disable&connect_timeout=1",
		"unparsable":  "postgres://app:" + password + "@h:notaport/db",
		"bad option":  "postgres://app:" + password + "@127.0.0.1:1/db?sslmode=sideways",
		"keyword dsn": "host=127.0.0.1 port=1 user=app password=" + password + " connect_timeout=1 sslmode=disable",
	} {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			log := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			_, err := Open(context.Background(), Config{
				DSN: dsn, ConnectTimeout: 300 * time.Millisecond, ConnectBackoff: 50 * time.Millisecond,
			}, log)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), password)
			assert.NotContains(t, logs.String(), password)
		})
	}
}

func openWithMock(t *testing.T) (sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	orig := openFunc
	openFunc = func(Config) (*sql.DB, error) { return sqlDB, nil }
	return mock, func() { openFunc = orig }
}

func TestOpen_AcceptsTheExpectedLoginRole(t *testing.T) {
	mock, restore := openWithMock(t)
	t.Cleanup(restore)
	mock.ExpectQuery(`SELECT session_user`).WillReturnRows(sqlmock.NewRows([]string{"session_user"}).AddRow("oagent_service"))

	sqlDB, err := Open(context.Background(), Config{DSN: okDSN, Role: "oagent_service"}, nil)
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet(), "Open must never SET ROLE")
	_ = sqlDB.Close()
}

func TestOpen_RefusesALoginOtherThanTheRole(t *testing.T) {
	mock, restore := openWithMock(t)
	t.Cleanup(restore)
	mock.ExpectQuery(`SELECT session_user`).WillReturnRows(sqlmock.NewRows([]string{"session_user"}).AddRow("oagent_owner"))
	mock.ExpectClose()

	_, err := Open(context.Background(), Config{DSN: okDSN, Role: "oagent_service"}, nil)
	require.Error(t, err)
	assert.True(t, IsLoginRoleError(err), "got %v", err)
	assert.Contains(t, err.Error(), `logged in as "oagent_owner"`)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOpen_LoginRoleQueryFails(t *testing.T) {
	mock, restore := openWithMock(t)
	t.Cleanup(restore)
	mock.ExpectQuery(`SELECT session_user`).WillReturnError(errors.New("gone"))
	mock.ExpectClose()

	_, err := Open(context.Background(), Config{DSN: okDSN, Role: "oagent_service"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pg: login role")
}
