// Package approval holds a run's tool calls for a person's decision before they run.
package approval

import (
	"errors"
	"strings"

	"altalune.id/oagent/apperror"
)

// PendingError reports a thread whose run waits on an approval; Approval is its public id.
type PendingError struct{ Approval string }

func (e *PendingError) Error() string { return "approval: pending " + e.Approval }

// IsPendingError reports whether err's tree contains a *PendingError.
func IsPendingError(err error) bool {
	_, ok := errors.AsType[*PendingError](err)
	return ok
}

// ToAppError maps PendingError to its own code, naming the approval.
func (e *PendingError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeApprovalPending, "This thread is waiting for an approval decision").
		WithMeta(map[string]string{"approval": e.Approval}).WithCause(e)
}

// NotFoundError reports an approval that does not exist or that the caller does not reach.
type NotFoundError struct{}

func (e *NotFoundError) Error() string { return "approval: not found" }

// IsNotFoundError reports whether err's tree contains a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// ToAppError maps NotFoundError to its own code.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeApprovalNotFound, "Approval not found").WithCause(e)
}

// AlreadyDecidedError reports an approval decided or expired before this decision; Status is approved, rejected or expired.
type AlreadyDecidedError struct{ Status string }

func (e *AlreadyDecidedError) Error() string { return "approval: already " + e.Status }

// IsAlreadyDecidedError reports whether err's tree contains an *AlreadyDecidedError.
func IsAlreadyDecidedError(err error) bool {
	_, ok := errors.AsType[*AlreadyDecidedError](err)
	return ok
}

// ToAppError maps AlreadyDecidedError to its own code.
func (e *AlreadyDecidedError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeApprovalAlreadyDecided, "This approval was already decided or has expired").
		WithMeta(map[string]string{"status": e.Status}).WithCause(e)
}

// ResumeIncompleteError reports a resume that leaves open interrupts unanswered; Missing lists their ids.
type ResumeIncompleteError struct{ Missing []string }

func (e *ResumeIncompleteError) Error() string {
	return "approval: resume misses " + strings.Join(e.Missing, ",")
}

// IsResumeIncompleteError reports whether err's tree contains a *ResumeIncompleteError.
func IsResumeIncompleteError(err error) bool {
	_, ok := errors.AsType[*ResumeIncompleteError](err)
	return ok
}

// ToAppError maps ResumeIncompleteError to its own code, listing the unanswered interrupt ids.
func (e *ResumeIncompleteError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeResumeIncomplete, "The resume must answer every open interrupt").
		WithMeta(map[string]string{"missing": strings.Join(e.Missing, ",")}).WithCause(e)
}

// EditedArgsRefusedError reports a resume that tries to change a call's arguments.
type EditedArgsRefusedError struct{}

func (e *EditedArgsRefusedError) Error() string { return "approval: edited arguments refused" }

// IsEditedArgsRefusedError reports whether err's tree contains an *EditedArgsRefusedError.
func IsEditedArgsRefusedError(err error) bool {
	_, ok := errors.AsType[*EditedArgsRefusedError](err)
	return ok
}

// ToAppError maps EditedArgsRefusedError to its own code.
func (e *EditedArgsRefusedError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeEditedArgsRefused, "An approval can't change the call's arguments").WithCause(e)
}

// AnsweredByAppError reports an approval raised by an API conversation; only that app answers it, through its resume.
type AnsweredByAppError struct{}

func (e *AnsweredByAppError) Error() string { return "approval: answered by the app" }

// IsAnsweredByAppError reports whether err's tree contains an *AnsweredByAppError.
func IsAnsweredByAppError(err error) bool {
	_, ok := errors.AsType[*AnsweredByAppError](err)
	return ok
}

// ToAppError maps AnsweredByAppError to its own code.
func (e *AnsweredByAppError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeApprovalAnsweredByApp, "This approval is answered by the app that started the conversation").WithCause(e)
}

// InvalidPageTokenError reports a page token this server did not issue.
type InvalidPageTokenError struct{}

func (e *InvalidPageTokenError) Error() string { return "approval: invalid page token" }

// ToAppError maps InvalidPageTokenError to a validation error on page_token.
func (e *InvalidPageTokenError) ToAppError() *apperror.AppError {
	return apperror.Validation([]apperror.Violation{{Field: "page_token", Reason: apperror.FieldInvalid}})
}
