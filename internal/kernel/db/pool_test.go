package db

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPool_CloseEmpty(t *testing.T) {
	assert.NoError(t, Pool{}.Close())
}

func TestPool_CloseAliased(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose()

	assert.NoError(t, Pool{W: sqlDB, R: sqlDB}.Close())
	assert.NoError(t, mock.ExpectationsWereMet(), "an aliased reader is closed once")
}

func TestPool_CloseBoth(t *testing.T) {
	writer, writerMock, err := sqlmock.New()
	require.NoError(t, err)
	writerMock.ExpectClose()
	reader, readerMock, err := sqlmock.New()
	require.NoError(t, err)
	readerMock.ExpectClose()

	assert.NoError(t, Pool{W: writer, R: reader}.Close())
	assert.NoError(t, writerMock.ExpectationsWereMet())
	assert.NoError(t, readerMock.ExpectationsWereMet())
}
