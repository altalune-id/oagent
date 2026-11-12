package approval_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/approval"
)

func TestErrors_MapToTheirCodes(t *testing.T) {
	for _, tc := range []struct {
		err  interface{ ToAppError() *apperror.AppError }
		is   func(error) bool
		code apperror.Code
		meta map[string]string
	}{
		{&approval.PendingError{Approval: "Vb3Kq8Rt2Wy5Zp9Hm7Nc4"}, approval.IsPendingError, apperror.CodeApprovalPending,
			map[string]string{"approval": "Vb3Kq8Rt2Wy5Zp9Hm7Nc4"}},
		{&approval.NotFoundError{}, approval.IsNotFoundError, apperror.CodeApprovalNotFound, nil},
		{&approval.AlreadyDecidedError{Status: "expired"}, approval.IsAlreadyDecidedError, apperror.CodeApprovalAlreadyDecided,
			map[string]string{"status": "expired"}},
		{&approval.ResumeIncompleteError{Missing: []string{"call_1", "call_2"}}, approval.IsResumeIncompleteError, apperror.CodeResumeIncomplete,
			map[string]string{"missing": "call_1,call_2"}},
		{&approval.EditedArgsRefusedError{}, approval.IsEditedArgsRefusedError, apperror.CodeEditedArgsRefused, nil},
		{&approval.AnsweredByAppError{}, approval.IsAnsweredByAppError, apperror.CodeApprovalAnsweredByApp, nil},
	} {
		err := tc.err.(error)
		ae := tc.err.ToAppError()
		assert.Equal(t, tc.code, ae.Code(), "%T", err)
		assert.NotEmpty(t, err.Error())
		assert.NotEmpty(t, ae.Message())
		for k, v := range tc.meta {
			assert.Equal(t, v, ae.Meta()[k], "%T meta %s", err, k)
		}
		assert.True(t, tc.is(fmt.Errorf("wrapped: %w", err)), "%T", err)
		assert.False(t, tc.is(errors.New("other")), "%T", err)
	}
}
