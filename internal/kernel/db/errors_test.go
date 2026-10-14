package db

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestIsNoRows(t *testing.T) {
	assert.True(t, IsNoRows(qrm.ErrNoRows))
	assert.True(t, IsNoRows(sql.ErrNoRows))
	assert.True(t, IsNoRows(fmt.Errorf("load: %w", qrm.ErrNoRows)))
	assert.False(t, IsNoRows(errors.New("boom")))
	assert.False(t, IsNoRows(nil))
}

func TestIsUniqueViolation(t *testing.T) {
	assert.True(t, IsUniqueViolation(&pgconn.PgError{Code: "23505"}))
	assert.True(t, IsUniqueViolation(fmt.Errorf("insert: %w", &pgconn.PgError{Code: "23505"})))
	assert.False(t, IsUniqueViolation(&pgconn.PgError{Code: "23503"}))
	assert.False(t, IsUniqueViolation(errors.New("boom")))
	assert.False(t, IsUniqueViolation(nil))
}

func TestIsForeignKeyViolation(t *testing.T) {
	assert.True(t, IsForeignKeyViolation(&pgconn.PgError{Code: "23503"}))
	assert.True(t, IsForeignKeyViolation(fmt.Errorf("insert: %w", &pgconn.PgError{Code: "23503"})))
	assert.False(t, IsForeignKeyViolation(&pgconn.PgError{Code: "23505"}))
	assert.False(t, IsForeignKeyViolation(errors.New("boom")))
	assert.False(t, IsForeignKeyViolation(nil))
}

func TestViolatedConstraint(t *testing.T) {
	name, ok := ViolatedConstraint(fmt.Errorf("insert: %w", &pgconn.PgError{Code: "23505", ConstraintName: "t_api_keys_hint_idx"}))
	assert.True(t, ok)
	assert.Equal(t, "t_api_keys_hint_idx", name)

	_, ok = ViolatedConstraint(&pgconn.PgError{Code: "23505"})
	assert.False(t, ok, "a violation without a constraint name names none")
	_, ok = ViolatedConstraint(errors.New("boom"))
	assert.False(t, ok)
	_, ok = ViolatedConstraint(nil)
	assert.False(t, ok)
}
