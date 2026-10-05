package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPool_CloseEmpty(t *testing.T) {
	assert.NoError(t, Pool{}.Close())
}

func TestPool_CloseAliased(t *testing.T) {
	sqlDB, err := openPostgres(DBConfig{DSN: okDSN})
	require.NoError(t, err)

	assert.NoError(t, Pool{W: sqlDB, R: sqlDB}.Close())
}

func TestOpenPool_BadDSN(t *testing.T) {
	_, err := OpenPool(context.Background(), DBConfig{DSN: "postgres://u:p@h:notaport/db"}, nil)
	assert.Error(t, err)
}

func TestOpenPool_WithReader(t *testing.T) {
	writer, writerMock, err := sqlmock.New()
	require.NoError(t, err)
	writerMock.ExpectClose()
	reader, readerMock, err := sqlmock.New()
	require.NoError(t, err)
	readerMock.ExpectClose()

	orig := openFunc
	t.Cleanup(func() { openFunc = orig })
	queue := []*sql.DB{writer, reader}
	openFunc = func(DBConfig) (*sql.DB, error) {
		d := queue[0]
		queue = queue[1:]
		return d, nil
	}

	p, err := OpenPool(context.Background(), DBConfig{DSN: okDSN, Reader: ReaderConfig{DSN: "postgres://reader"}}, nil)
	require.NoError(t, err)
	assert.NotSame(t, p.W, p.R)
	assert.NoError(t, p.Close())
}

func TestOpenPool_ReaderError(t *testing.T) {
	writer, writerMock, err := sqlmock.New()
	require.NoError(t, err)
	writerMock.ExpectClose()

	orig := openFunc
	t.Cleanup(func() { openFunc = orig })
	calls := 0
	openFunc = func(DBConfig) (*sql.DB, error) {
		calls++
		if calls == 1 {
			return writer, nil
		}
		return nil, errors.New("reader down")
	}

	_, err = OpenPool(context.Background(), DBConfig{DSN: okDSN, Reader: ReaderConfig{DSN: "postgres://reader"}}, nil)
	assert.Error(t, err)
}
