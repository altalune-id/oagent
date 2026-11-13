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

func TestIsRowSecurityViolation(t *testing.T) {
	using := `new row violates row-level security policy (USING expression) for table "webhook_endpoints"`
	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"the conflict refusal, as Postgres 17.12 reports it": {&pgconn.PgError{Code: "42501", Routine: "ExecWithCheckOptions", Message: using}, true},
		"wrapped": {fmt.Errorf("save: %w", &pgconn.PgError{Code: "42501", Routine: "ExecWithCheckOptions", Message: using}), true},
		"a translated message, matched by its routine":         {&pgconn.PgError{Code: "42501", Routine: "ExecWithCheckOptions", Message: "la nueva fila viola la política de seguridad de registros"}, true},
		"no routine: the English conflict text":                {&pgconn.PgError{Code: "42501", Message: using}, true},
		"no routine: a WITH CHECK refusal is not the conflict": {&pgconn.PgError{Code: "42501", Message: `new row violates row-level security policy for table "webhook_endpoints"`}, false},
		"another insufficient-privilege error":                 {&pgconn.PgError{Code: "42501", Routine: "aclcheck_error", Message: "permission denied for table webhook_endpoints"}, false},
		"another code":                                         {&pgconn.PgError{Code: "23505", Routine: "ExecWithCheckOptions", Message: using}, false},
		"not a Postgres error":                                 {errors.New(using), false},
		"nil":                                                  {nil, false},
	} {
		assert.Equal(t, tc.want, IsRowSecurityViolation(tc.err), name)
	}
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
