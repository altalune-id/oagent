package invite

import (
	"errors"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/password"
)

// Reason is why an invite can't be used, sent to clients as meta.reason.
type Reason string

// The reasons of a NotUsableError.
const (
	ReasonExpired  Reason = "expired"
	ReasonRevoked  Reason = "revoked"
	ReasonUsed     Reason = "used"
	ReasonNotFound Reason = "not_found"
)

// NotUsableError reports an invite link that no longer works.
type NotUsableError struct{ Reason Reason }

func (e *NotUsableError) Error() string { return "invite: not usable: " + string(e.Reason) }

// ToAppError maps NotUsableError to its own code with the reason.
func (e *NotUsableError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeInviteNotUsable, "This invite can no longer be used").
		WithMeta(map[string]string{"reason": string(e.Reason)}).WithCause(e)
}

// IsNotUsableError reports whether err's tree contains a *NotUsableError.
func IsNotUsableError(err error) bool {
	_, ok := errors.AsType[*NotUsableError](err)
	return ok
}

// ReasonOf returns the reason of the *NotUsableError in err's tree, or "".
func ReasonOf(err error) Reason {
	if e, ok := errors.AsType[*NotUsableError](err); ok {
		return e.Reason
	}
	return ""
}

// EmailMismatchError reports a person accepting an invite sent to another email.
type EmailMismatchError struct{}

func (*EmailMismatchError) Error() string { return "invite: email mismatch" }

// ToAppError maps EmailMismatchError to its own code.
func (e *EmailMismatchError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeInviteEmailMismatch, "This invite is for a different email address").WithCause(e)
}

// IsEmailMismatchError reports whether err's tree contains an *EmailMismatchError.
func IsEmailMismatchError(err error) bool {
	_, ok := errors.AsType[*EmailMismatchError](err)
	return ok
}

// DraftExpiredError reports a draft confirmed after DraftTTL.
type DraftExpiredError struct{}

func (*DraftExpiredError) Error() string { return "invite: draft expired" }

// ToAppError maps DraftExpiredError to its own code.
func (e *DraftExpiredError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeInviteDraftExpired, "This draft expired; ask the assistant again").WithCause(e)
}

// IsDraftExpiredError reports whether err's tree contains a *DraftExpiredError.
func IsDraftExpiredError(err error) bool {
	_, ok := errors.AsType[*DraftExpiredError](err)
	return ok
}

// NotFoundError reports an invite id that is unknown, outside the org, or not in the state the call needs.
type NotFoundError struct{}

func (*NotFoundError) Error() string { return "invite: not found" }

// ToAppError maps NotFoundError to the not-found envelope.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeNotFound, "Invite not found").WithCause(e)
}

// IsNotFoundError reports whether err's tree contains a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// PendingError reports a draft for an email that already has a pending invite.
type PendingError struct{}

func (*PendingError) Error() string { return "invite: already pending" }

// ToAppError maps PendingError to the already-exists envelope.
func (e *PendingError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAlreadyExists, "This email already has a pending invite").WithCause(e)
}

// IsPendingError reports whether err's tree contains a *PendingError.
func IsPendingError(err error) bool {
	_, ok := errors.AsType[*PendingError](err)
	return ok
}

// IsExpectedError reports whether err is a domain outcome callers handle, not an incident to report.
func IsExpectedError(err error) bool {
	return IsNotUsableError(err) || IsEmailMismatchError(err) || IsDraftExpiredError(err) ||
		IsNotFoundError(err) || IsPendingError(err) ||
		member.IsExpectedError(err) || user.IsExpectedError(err) || password.IsPolicyError(err) || authz.IsForbiddenError(err)
}
