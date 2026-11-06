package runtime

import (
	"errors"
	"strconv"
)

// ContractError reports that the runtime refused the request or sent a stream the contract does not allow.
type ContractError struct {
	Status int
	Code   string
	Cause  error
}

func (e *ContractError) Error() string {
	msg := "runtime: contract broken: status " + strconv.Itoa(e.Status)
	if e.Code != "" {
		msg += " " + e.Code
	}
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

func (e *ContractError) Unwrap() error { return e.Cause }

// IsContractError reports whether err's tree contains a *ContractError.
func IsContractError(err error) bool {
	_, ok := errors.AsType[*ContractError](err)
	return ok
}

// SnapshotUnloadableError reports that the runtime could not load the snapshot; the caller retries with history.
type SnapshotUnloadableError struct{}

func (*SnapshotUnloadableError) Error() string { return "runtime: snapshot unloadable" }

// IsSnapshotUnloadableError reports whether err's tree contains a *SnapshotUnloadableError.
func IsSnapshotUnloadableError(err error) bool {
	_, ok := errors.AsType[*SnapshotUnloadableError](err)
	return ok
}

// BusyError reports that the runtime is at its concurrency cap; the turn may be retried.
type BusyError struct{}

func (*BusyError) Error() string { return "runtime: busy" }

// IsBusyError reports whether err's tree contains a *BusyError.
func IsBusyError(err error) bool {
	_, ok := errors.AsType[*BusyError](err)
	return ok
}

// StatusError reports an HTTP status the contract does not name.
type StatusError struct{ Status int }

func (e *StatusError) Error() string { return "runtime: unexpected status " + strconv.Itoa(e.Status) }

// IsStatusError reports whether err's tree contains a *StatusError.
func IsStatusError(err error) bool {
	_, ok := errors.AsType[*StatusError](err)
	return ok
}

// NoTerminalError reports a stream that ended without its terminal event: the runtime crashed mid-turn.
type NoTerminalError struct{ Cause error }

func (e *NoTerminalError) Error() string {
	if e.Cause == nil {
		return "runtime: stream ended without a terminal event"
	}
	return "runtime: stream ended without a terminal event: " + e.Cause.Error()
}

func (e *NoTerminalError) Unwrap() error { return e.Cause }

// IsNoTerminalError reports whether err's tree contains a *NoTerminalError.
func IsNoTerminalError(err error) bool {
	_, ok := errors.AsType[*NoTerminalError](err)
	return ok
}
