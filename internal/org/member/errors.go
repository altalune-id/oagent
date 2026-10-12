package member

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"altalune.id/oagent/apperror"
)

// NotFoundError reports that the user is not a member of the org.
type NotFoundError struct{}

func (*NotFoundError) Error() string { return "member: not found" }

// ToAppError maps NotFoundError to the not-found envelope.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMemberNotFound, "Membership not found").WithCause(e)
}

// IsNotFoundError reports whether err's tree contains a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// AlreadyMemberError reports a user who already belongs to the org.
type AlreadyMemberError struct{ OrgID, UserID uuid.UUID }

func (*AlreadyMemberError) Error() string { return "member: already a member" }

// ToAppError maps AlreadyMemberError to the conflict envelope.
func (e *AlreadyMemberError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMemberAlreadyExists, "User is already a member").WithCause(e)
}

// IsAlreadyMemberError reports whether err's tree contains an *AlreadyMemberError.
func IsAlreadyMemberError(err error) bool {
	_, ok := errors.AsType[*AlreadyMemberError](err)
	return ok
}

// InvalidRoleError reports a membership with an unknown role.
type InvalidRoleError struct{ Role string }

func (e *InvalidRoleError) Error() string {
	return fmt.Sprintf("member: unknown role %q", apperror.Echo(e.Role))
}

// ToAppError maps InvalidRoleError to its own code with the role.
func (e *InvalidRoleError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMemberInvalidRole, fmt.Sprintf("Unknown membership role %q", apperror.Echo(e.Role))).
		WithMeta(map[string]string{"role": apperror.Echo(e.Role)}).WithCause(e)
}

// IsInvalidRoleError reports whether err's tree contains an *InvalidRoleError.
func IsInvalidRoleError(err error) bool {
	_, ok := errors.AsType[*InvalidRoleError](err)
	return ok
}

// IsExpectedError reports whether err is a domain outcome callers handle, not an incident to report.
func IsExpectedError(err error) bool {
	return IsNotFoundError(err) || IsAlreadyMemberError(err) || IsInvalidRoleError(err)
}

// IsUniqueViolation reports whether err is a Postgres unique-violation (SQLSTATE 23505).
func IsUniqueViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "23505"
}
