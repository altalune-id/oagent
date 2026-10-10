package member

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/apperror"
)

func TestErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		err interface {
			error
			ToAppError() *apperror.AppError
		}
		is   func(error) bool
		code apperror.Code
	}{
		"not found":      {&NotFoundError{}, IsNotFoundError, apperror.CodeMemberNotFound},
		"already member": {&AlreadyMemberError{OrgID: uuid.New(), UserID: uuid.New()}, IsAlreadyMemberError, apperror.CodeMemberAlreadyExists},
		"invalid role":   {&InvalidRoleError{Role: "root", Reason: "unknown role"}, IsInvalidRoleError, apperror.CodeMemberInvalidRole},
	} {
		t.Run(name, func(t *testing.T) {
			assert.True(t, tc.is(tc.err))
			assert.True(t, IsExpectedError(tc.err))
			assert.NotEmpty(t, tc.err.Error())
			assert.Equal(t, tc.code, tc.err.ToAppError().Code())
		})
	}
	assert.False(t, IsExpectedError(errors.New("boom")))
	assert.True(t, IsUniqueViolation(&pgconn.PgError{Code: "23505"}))
	assert.False(t, IsUniqueViolation(errors.New("x")))
}
