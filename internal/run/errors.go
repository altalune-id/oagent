package run

import (
	"errors"
	"strconv"
	"time"

	"altalune.id/oagent/apperror"
)

// IllegalMoveError reports a status move the run's life does not allow.
type IllegalMoveError struct{ From, To Status }

func (e *IllegalMoveError) Error() string {
	return "run: illegal move " + string(e.From) + " -> " + string(e.To)
}

// IsIllegalMoveError reports whether err's tree contains an *IllegalMoveError.
func IsIllegalMoveError(err error) bool {
	_, ok := errors.AsType[*IllegalMoveError](err)
	return ok
}

// LeaseLostError reports that the thread's lease is no longer held by the token, so the write was refused.
type LeaseLostError struct{ ThreadID string }

func (e *LeaseLostError) Error() string { return "run: lease lost: thread=" + e.ThreadID }

// IsLeaseLostError reports whether err's tree contains a *LeaseLostError.
func IsLeaseLostError(err error) bool {
	_, ok := errors.AsType[*LeaseLostError](err)
	return ok
}

// ThreadNotFoundError reports that no thread of the org on ctx matched.
type ThreadNotFoundError struct{ ID string }

func (e *ThreadNotFoundError) Error() string { return "run: thread not found: id=" + e.ID }

// IsThreadNotFoundError reports whether err's tree contains a *ThreadNotFoundError.
func IsThreadNotFoundError(err error) bool {
	_, ok := errors.AsType[*ThreadNotFoundError](err)
	return ok
}

// UnsettledError reports a Settle of a run that is neither terminal nor interrupted.
type UnsettledError struct{ Status Status }

func (e *UnsettledError) Error() string {
	return "run: cannot settle a run in status " + string(e.Status)
}

// NotFoundError reports that no run of the org on ctx matched.
type NotFoundError struct{ ID string }

func (e *NotFoundError) Error() string { return "run: run not found: id=" + e.ID }

// IsNotFoundError reports whether err's tree contains a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// RateLimitedError reports that the project started too many runs in the last minute.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string {
	return "run: project rate limited; retry after " + e.RetryAfter.String()
}

// IsRateLimitedError reports whether err's tree contains a *RateLimitedError.
func IsRateLimitedError(err error) bool {
	_, ok := errors.AsType[*RateLimitedError](err)
	return ok
}

// RuntimeUnavailableError reports that the agent runtime could not take the turn.
type RuntimeUnavailableError struct{ Cause error }

func (e *RuntimeUnavailableError) Error() string {
	if e.Cause == nil {
		return "run: runtime unavailable"
	}
	return "run: runtime unavailable: " + e.Cause.Error()
}

func (e *RuntimeUnavailableError) Unwrap() error { return e.Cause }

// TimedOutError reports that the turn passed its deadline.
type TimedOutError struct{}

func (*TimedOutError) Error() string { return "run: turn deadline passed" }

// AttemptsExhaustedError reports that the run failed on every attempt it was allowed.
type AttemptsExhaustedError struct{ Attempts int32 }

func (e *AttemptsExhaustedError) Error() string {
	return "run: attempts exhausted after " + strconv.Itoa(int(e.Attempts))
}

// RuntimeFailedError reports that the runtime ended the turn as failed; Reason is its error code.
type RuntimeFailedError struct{ Reason string }

func (e *RuntimeFailedError) Error() string { return "run: runtime failed: " + e.Reason }

// IsExpectedError reports whether err is a typed domain error a service returns as-is.
func IsExpectedError(err error) bool {
	for _, is := range []func(error) bool{
		IsNotFoundError, IsRateLimitedError, IsThreadNotFoundError, IsIllegalMoveError, IsInProgressError, IsIdempotencyConflictError,
		IsInvalidPageTokenError,
		func(err error) bool { _, ok := errors.AsType[*RuntimeUnavailableError](err); return ok },
		func(err error) bool { _, ok := errors.AsType[*TimedOutError](err); return ok },
		func(err error) bool { _, ok := errors.AsType[*AttemptsExhaustedError](err); return ok },
		func(err error) bool { _, ok := errors.AsType[*RuntimeFailedError](err); return ok },
		func(err error) bool { return errors.Is(err, ErrDraftRefused) },
	} {
		if is(err) {
			return true
		}
	}
	return false
}

// ToAppError maps NotFoundError to its own code.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeRunNotFound, "Run not found").WithCause(e)
}

// ToAppError maps RateLimitedError to its own code.
func (e *RateLimitedError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeRunRateLimited, "Too many runs in this project").WithCause(e)
}

// ToAppError maps RuntimeUnavailableError to its own code; the cause stays server-side.
func (e *RuntimeUnavailableError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeRunRuntimeUnavailable, "The agent runtime is unavailable").WithCause(e)
}

// ToAppError maps TimedOutError to its own code.
func (e *TimedOutError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeRunTimedOut, "The run passed its deadline").WithCause(e)
}

// ToAppError maps AttemptsExhaustedError to its own code.
func (e *AttemptsExhaustedError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeRunAttemptsExhausted, "The run failed on every attempt").WithCause(e)
}

// ToAppError maps RuntimeFailedError to its own code.
func (e *RuntimeFailedError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeRunRuntimeFailed, "The agent runtime failed the run").WithCause(e)
}

// ErrNothingPending reports a new run with no pending message to take; nothing was stored.
var ErrNothingPending = errors.New("run: no pending message to start a run from")

// ErrDraftRefused reports that the agent's draft is refused like a publish, so a Try it run can't pin it.
var ErrDraftRefused = errors.New("run: the agent's draft can't run as it is")

// ErrResourceGone reports that a run's pinned agent version or a granted MCP server no longer exists.
var ErrResourceGone = errors.New("run: the pinned version or a granted MCP server is gone")

// SettleError reports that a turn's end could not be written; the lease is kept so it expires and the sweeper reclaims the run.
type SettleError struct{ Cause error }

func (e *SettleError) Error() string { return "run: settle failed: " + e.Cause.Error() }

func (e *SettleError) Unwrap() error { return e.Cause }

// IsSettleError reports whether err's tree contains a *SettleError.
func IsSettleError(err error) bool {
	_, ok := errors.AsType[*SettleError](err)
	return ok
}

// InProgressError reports that the thread already has a live turn; RunPublicID is empty while no run exists yet.
type InProgressError struct{ RunPublicID string }

func (e *InProgressError) Error() string {
	if e.RunPublicID == "" {
		return "run: a turn is in progress on this thread"
	}
	return "run: run in progress: " + e.RunPublicID
}

// IsInProgressError reports whether err's tree contains an *InProgressError.
func IsInProgressError(err error) bool {
	_, ok := errors.AsType[*InProgressError](err)
	return ok
}

// ToAppError maps InProgressError to its own code, naming the run when there is one.
func (e *InProgressError) ToAppError() *apperror.AppError {
	ae := apperror.New(apperror.CodeRunInProgress, "A run is already in progress on this thread").WithCause(e)
	if e.RunPublicID != "" {
		ae = ae.WithMeta(map[string]string{"run": e.RunPublicID})
	}
	return ae
}

// IdempotencyConflictError reports an Idempotency-Key already used with a different request body.
type IdempotencyConflictError struct{}

func (*IdempotencyConflictError) Error() string {
	return "run: idempotency key reused with another body"
}

// IsIdempotencyConflictError reports whether err's tree contains an *IdempotencyConflictError.
func IsIdempotencyConflictError(err error) bool {
	_, ok := errors.AsType[*IdempotencyConflictError](err)
	return ok
}

// ToAppError maps IdempotencyConflictError to its own code.
func (e *IdempotencyConflictError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeRunIdempotencyConflict, "This Idempotency-Key was already used with a different request").WithCause(e)
}

const threadNotFoundMessage = "Thread not found"

// ToAppError maps ThreadNotFoundError to its own code.
func (e *ThreadNotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeThreadNotFound, threadNotFoundMessage).WithCause(e)
}

// ThreadActorMismatchError reports a thread bound to another actor; it reads as not found so a stranger learns nothing.
type ThreadActorMismatchError struct{ ThreadID string }

func (e *ThreadActorMismatchError) Error() string {
	return "run: thread bound to another actor: " + e.ThreadID
}

// Unwrap makes the mismatch a ThreadNotFoundError for every caller that checks for one.
func (e *ThreadActorMismatchError) Unwrap() error { return &ThreadNotFoundError{ID: e.ThreadID} }

// ToAppError maps ThreadActorMismatchError to THR001. SECURITY: the wire must not tell a bound thread from a missing one.
func (e *ThreadActorMismatchError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeThreadNotFound, threadNotFoundMessage).WithCause(e)
}

// DuplicateMessageError reports a message whose external id the thread already holds; Seq is the stored one when known.
type DuplicateMessageError struct{ Seq int64 }

func (e *DuplicateMessageError) Error() string {
	return "run: the thread already holds this message: seq " + strconv.FormatInt(e.Seq, 10)
}

var errNoActor = errors.New("run: an API thread needs an actor")

// InvalidPageTokenError reports a page token this server did not issue.
type InvalidPageTokenError struct{}

func (e *InvalidPageTokenError) Error() string { return "run: invalid page token" }

// IsInvalidPageTokenError reports whether err's tree contains an *InvalidPageTokenError.
func IsInvalidPageTokenError(err error) bool {
	_, ok := errors.AsType[*InvalidPageTokenError](err)
	return ok
}

// ToAppError maps InvalidPageTokenError to a validation error on page_token.
func (e *InvalidPageTokenError) ToAppError() *apperror.AppError {
	return apperror.Validation([]apperror.Violation{{Field: "page_token", Reason: apperror.FieldInvalid}})
}
